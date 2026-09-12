package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/anacrolix/torrent"

	"cloudpan/internal/fscore"
	"cloudpan/internal/model"
)

// btProps BT/磁力链任务属性（写入 Task.Props，进度时回写展示字段）
type btProps struct {
	URL      string `json:"url"`
	PolicyID uint   `json:"policyId"`
	Dest     string `json:"dest"`
	Name     string `json:"name"`
	Kind     string `json:"kind"` // bt
	RTName   string `json:"rtName,omitempty"`
	Seeds    int    `json:"seeds,omitempty"`
	Peers    int    `json:"peers,omitempty"`
}

// runBT BT/磁力链离线下载：纯 Go torrent 客户端（DHT+tracker），完成后导入网盘
func (p *TaskPool) runBT(t *model.Task) error {
	var props btProps
	if err := json.Unmarshal([]byte(t.Props), &props); err != nil {
		return err
	}
	var policy model.Policy
	if err := model.DB.First(&policy, props.PolicyID).Error; err != nil {
		return fmt.Errorf("存储策略不存在")
	}
	d, err := p.Svc.DriverFor(&policy, userOfID(t.UserID)) // 任务结果落回属主隔离目录
	if err != nil {
		return err
	}

	dataDir := filepath.Join(p.BtDir, fmt.Sprint(t.ID))
	_ = os.MkdirAll(dataDir, 0o755)
	defer os.RemoveAll(dataDir)

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = dataDir
	cfg.NoUpload = true
	client, err := torrent.NewClient(cfg)
	if err != nil {
		return fmt.Errorf("BT 客户端启动失败: %w", err)
	}
	defer client.Close()

	var tr *torrent.Torrent
	if strings.HasPrefix(props.URL, "magnet:") {
		tr, err = client.AddMagnet(props.URL)
	} else {
		// http(s) .torrent 种子文件（SSRF 防护客户端）；.torrent 正常为 KB 级，100MB 上限兜底
		resp, herr := ssrfHTTP.Get(props.URL)
		if herr != nil {
			return fmt.Errorf("种子下载失败: %w", herr)
		}
		const maxTorrent = 100 << 20
		body, rerr := io.ReadAll(io.LimitReader(resp.Body, maxTorrent))
		resp.Body.Close()
		if rerr != nil {
			return rerr
		}
		if len(body) >= maxTorrent {
			return fmt.Errorf("种子文件过大（超过 100MB），已拒绝")
		}
		if resp.StatusCode >= 400 {
			return fmt.Errorf("种子下载失败 HTTP %d", resp.StatusCode)
		}
		tf := filepath.Join(dataDir, "meta.torrent")
		if werr := os.WriteFile(tf, body, 0o644); werr != nil {
			return werr
		}
		tr, err = client.AddTorrentFromFile(tf)
	}
	if err != nil {
		return fmt.Errorf("添加任务失败: %w", err)
	}
	// 补一批公共 tracker。磁力链里自带的 tr= 往往早已失效，只靠 DHT 时内网环境
	// 极容易一直连不上节点；补上公共 tracker 是提高"能拿到种子信息"概率最直接的手段。
	// 每个元素是一个 tracker tier（这里是单元素 tier），失败的 tracker 会被库自行忽略。
	if tiers := publicTrackerTiers(); len(tiers) > 0 {
		tr.AddTrackers(tiers)
	}

	// 等待元数据（磁力链需 DHT/PEX/Tracker 发现）；期间持续回报节点数，别让人干等
	p.setTaskMsg(t.ID, "正在获取种子信息（DHT/Tracker 发现中）...")
	tick := time.NewTicker(10 * time.Second)
	defer tick.Stop()
	deadline := time.NewTimer(3 * time.Minute)
	defer deadline.Stop()
waitInfo:
	for {
		select {
		case <-tr.GotInfo():
			break waitInfo
		case <-tick.C:
			st := tr.Stats()
			p.setTaskMsg(t.ID, fmt.Sprintf("正在获取种子信息：已连接 peer %d / seed %d（若长期为 0，多半是出站 UDP 被限制）", st.ActivePeers, st.ConnectedSeeders))
		case <-deadline.C:
			st := tr.Stats()
			return fmt.Errorf("获取种子信息超时（3 分钟）：始终没有连上有效节点（peer %d / seed %d）。"+
				"常见原因：① 本机或所在内网限制了出站 UDP（DHT 依赖 UDP，很多企业网/校园网会封），"+
				"② 该资源长期无人做种，③ 磁力链本身已失效", st.ActivePeers, st.ConnectedSeeders)
		}
	}
	info := tr.Info()
	props.RTName = info.BestName()
	p.saveProps(t.ID, props)

	total := info.TotalLength()
	// 硬性总量上限（20GB）：恶意种子可虚报/携带超大内容，先卡死再下载
	if total > 20<<30 {
		return fmt.Errorf("种子总大小 %dMB 超过 20GB 上限，已拒绝", total>>20)
	}
	// 配额预检：已知总大小且将超限时不开始下载
	if u := userOfID(t.UserID); u != nil {
		if limit, limited := effectiveQuotaBytes(u, model.AdminPerms()); limited && u.UsedBytes+total > limit {
			return fmt.Errorf("超出配额（已用 %dMB / 上限 %dMB），未开始下载", u.UsedBytes>>20, limit>>20)
		}
	}
	tr.DownloadAll()

	// 进度循环
	for {
		if p.cancelled(t.ID) {
			return fmt.Errorf("已取消")
		}
		done := tr.BytesCompleted()
		if total > 0 {
			p.setProgress(t.ID, int(done*100/total))
		}
		props.Seeds = tr.Stats().ConnectedSeeders
		props.Peers = tr.Stats().ActivePeers
		p.saveProps(t.ID, props)
		if total > 0 && done >= total {
			break
		}
		time.Sleep(2 * time.Second)
	}

	// 导入网盘：单文件种子 → dest/<种子名>；多文件种子 → dest/<种子名>/<目录结构>
	// 注意 anacrolix API：f.Path() = "种子名/相对路径"（含前缀），DisplayPath() 才是纯相对路径；
	// 文件存储物理布局 = dataDir/<info.Name>/<相对路径>（单文件时 = dataDir/<info.Name>）
	files := tr.Files()
	var imported int64 // 已导入字节数（配额记账）
	defer p.accountQuota(t, &imported)
	importOne := func(vp string, phys string) error {
		src, oerr := os.Open(phys)
		if oerr != nil {
			return fmt.Errorf("导入失败: %w", oerr)
		}
		defer src.Close()
		if fi, serr := os.Stat(phys); serr == nil {
			imported += fi.Size()
		}
		return d.CreateFile(vp, src)
	}
	if len(files) == 1 {
		singleFile := files[0]
		// 单文件种子：尝试多种物理路径（兼容不同种子制作工具）
		candidates := []string{
			filepath.Join(dataDir, singleFile.DisplayPath()),
			filepath.Join(dataDir, info.Name),
		}
		if singleFile.DisplayPath() != info.Name {
			candidates = append(candidates, filepath.Join(dataDir, info.Name, singleFile.DisplayPath()))
		}
		var phys string
		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				phys = c
				break
			}
		}
		if phys == "" {
			return fmt.Errorf("单文件种子物理路径未找到: 搜索 %v", candidates)
		}
		vp, jerr := fscore.Join(props.Dest, singleFile.DisplayPath())
		if jerr != nil {
			return jerr
		}
		if ierr := importOne(vp, phys); ierr != nil {
			return ierr
		}
		return nil
	}
	rootVP, err := fscore.Join(props.Dest, props.RTName)
	if err != nil {
		return err
	}
	if merr := p.mkdirChain(d, rootVP); merr != nil {
		return merr
	}
	for _, f := range files {
		if p.cancelled(t.ID) {
			return fmt.Errorf("已取消")
		}
		rel := f.DisplayPath()
		vp, jerr := fscore.Join(rootVP, rel)
		if jerr != nil {
			continue
		}
		if derr := p.mkdirChain(d, path.Dir(vp)); derr != nil {
			return derr
		}
		if ierr := importOne(vp, filepath.Join(dataDir, info.Name, filepath.FromSlash(rel))); ierr != nil {
			return ierr
		}
	}
	return nil
}

// mkdirChain 逐级创建虚拟目录（已存在则忽略）
func (p *TaskPool) mkdirChain(d interface {
	Mkdir(dir string) error
}, vp string) error {
	clean, err := fscore.Clean(vp)
	if err != nil {
		return err
	}
	if clean == "/" {
		return nil
	}
	cur := ""
	for _, seg := range strings.Split(strings.Trim(clean, "/"), "/") {
		cur += "/" + seg
		_ = d.Mkdir(cur) // 已存在会报错，忽略
	}
	return nil
}

// setTaskMsg 写运行中的阶段提示（Task.Msg 列）。
//
// 注意不要再把阶段消息写进 error 列：任务结束后 error 必须为空，否则
// 已完成的任务会一直显示最后那句阶段提示（"正在合并分片..."）。
func (p *TaskPool) setTaskMsg(id uint, msg string) {
	model.DB.Model(&model.Task{}).Where("id = ?", id).UpdateColumn("msg", msg)
}

func (p *TaskPool) saveProps(id uint, props btProps) {
	b, _ := json.Marshal(props)
	model.DB.Model(&model.Task{}).Where("id = ?", id).UpdateColumn("props", string(b))
}

// publicTrackerList 内置公共 tracker。
//
// 为什么要有：磁力链自带的 tracker 大量已下线，只依赖 DHT 时，内网/受限网络下
// 经常 3 分钟都拿不到种子信息。补一批社区维护的公共 tracker 能显著提高连通率；
// 库会自行忽略不可达的 tracker，所以多写几个没有副作用。
// 只用 UDP 与 HTTPS：UDP 不走 HTTP 请求，HTTPS 由 BT 库自行发起（不经过离线下载的 SSRF 客户端）。
var publicTrackerList = []string{
	"udp://tracker.opentrackr.org:1337/announce",
	"udp://open.tracker.cl:1337/announce",
	"udp://tracker.openbittorrent.com:6969/announce",
	"udp://exodus.desync.com:6969/announce",
	"udp://tracker.torrent.eu.org:451/announce",
	"udp://open.stealth.si:80/announce",
	"udp://opentracker.i2p.rocks:6969/announce",
	"udp://explodie.org:6969/announce",
	"udp://tracker.dler.org:6969/announce",
	"https://tracker.tamersunion.org:443/announce",
}

// publicTrackerTiers 转成 AddTrackers 需要的 [][]string（每个 tracker 单独一个 tier）
func publicTrackerTiers() [][]string {
	out := make([][]string, 0, len(publicTrackerList))
	for _, t := range publicTrackerList {
		out = append(out, []string{t})
	}
	return out
}

// sniffOfflineKind 按 URL 特征判定离线任务类型：bt / m3u8 / http
//
// 注意 m3u8 必须在 http 之前判定，否则 .m3u8 会被当成普通文件下载
// —— 那样只会落下一个几百字节的清单文本，等于"下不了"。
func sniffOfflineKind(url string) string {
	u := strings.TrimSpace(url)
	if strings.HasPrefix(u, "magnet:") {
		return "bt"
	}
	if strings.HasPrefix(u, "http://") || strings.HasPrefix(u, "https://") {
		if isM3U8URL(u) {
			return "m3u8"
		}
		if strings.HasSuffix(strings.ToLower(u), ".torrent") {
			return "bt"
		}
	}
	return "http"
}
