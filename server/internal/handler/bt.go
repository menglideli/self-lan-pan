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

// btListenPortOffset BT 监听端口的分配基数。
//
// anacrolix 客户端的默认 ListenPort 是 42069，而每个 BT 任务都会新建一个客户端 ——
// 于是第二个并发 BT 任务必然撞端口，直接报
// "first listen: listen tcp4 :42069: bind: Only one usage of each socket address"。
// 实测抓到（见 docs 单用户私有化改造计划 §7.11）。
// 这里按「任务 ID 取模」散列到一个端口段里，把并发任务之间错开；
// 真撞上了也只是那个任务失败并在 error 里说明，不会影响别的任务。
const (
	btListenPortBase = 42300
	btListenPortSpan = 500
)

// btListenPortOf 为某个任务算一个监听端口
func btListenPortOf(taskID uint) int {
	return btListenPortBase + int(taskID%btListenPortSpan)
}

// btDataDir BT 任务的数据目录（与 cache.go 的 btTaskDir 同一规则）
func (p *TaskPool) btDataDir(taskID uint) string {
	return p.btTaskDir(taskID)
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

	dataDir := p.btDataDir(t.ID)
	_ = os.MkdirAll(dataDir, 0o755)
	defer os.RemoveAll(dataDir)

	cfg := torrent.NewDefaultClientConfig()
	cfg.DataDir = dataDir
	cfg.NoUpload = true
	// 每个任务用不同端口，避免并发 BT 任务撞 42069（默认值）
	cfg.ListenPort = btListenPortOf(t.ID)
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

	// 进度循环。
	//
	// 这里必须带「停滞超时」：早先只有 `if total > 0 && done >= total { break }`，
	// 意味着只要下载始终不完成（拿不到 peer、对方不做种、tracker 全挂），
	// 这个 goroutine 就会永远 2 秒一轮地空转，任务在界面上永远停在"下载中 0%"，
	// 而 bt_tmp/<id> 一直占着磁盘 —— 用户看到的就是"BT 下不动、缓存还不删"。
	// 实测复现：本机做种 + 下载端连不上 peer 时，任务 90 秒后仍在 processing（见 §7.11）。
	//
	// 规则：连续 btStallTimeout 长时间（a）进度没有任何增长，就判失败并说明原因。
	// 进度有增长则重新计时，正常下载再慢也不会被误杀。
	const btStallTimeout = 5 * time.Minute
	lastDone := int64(-1)
	lastProgressAt := time.Now()
	for {
		if p.cancelled(t.ID) {
			return fmt.Errorf("已取消")
		}
		done := tr.BytesCompleted()
		if total > 0 {
			p.setProgress(t.ID, int(done*100/total))
		}
		st := tr.Stats()
		props.Seeds = st.ConnectedSeeders
		props.Peers = st.ActivePeers
		p.saveProps(t.ID, props)
		if total > 0 && done >= total {
			break
		}
		if done != lastDone {
			lastDone = done
			lastProgressAt = time.Now()
		} else if time.Since(lastProgressAt) > btStallTimeout {
			return fmt.Errorf("下载停滞超过 %d 分钟：已连接 peer %d / seed %d，累计下载 %dMB / %dMB。"+
				"常见原因：① 没有可用做种者（该资源已无人做种），② 出站 TCP/UDP 被网络策略限制，"+
				"③ tracker 均不可达且 DHT 被禁用。",
				int(btStallTimeout.Minutes()), st.ActivePeers, st.ConnectedSeeders, done>>20, total>>20)
		}
		time.Sleep(2 * time.Second)
	}

	// 导入网盘：单文件种子 → dest/<种子名>；多文件种子 → dest/<种子名>/<目录结构>
	// 注意 anacrolix API：f.Path() = "种子名/相对路径"（含前缀），DisplayPath() 才是纯相对路径；
	// 文件存储物理布局 = dataDir/<info.Name>/<相对路径>（单文件时 = dataDir/<info.Name>）。
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
	// resolvePhys 在 dataDir 下按几种常见物理布局找文件。
	//
	// 为什么要试多种：不同种子制作工具/不同版本写出的目录层级并不统一。
	// 早先只按 info.Name 拼一次，找不到就直接报错返回 —— 而 run() 收到 error 后
	// 任务判失败，紧接着 defer os.RemoveAll(dataDir) 会把**刚刚下好的数据全删掉**。
	// 用户看到的现象正是"明明下好了，却没进网盘"，同时缓存也"消失"了（被 defer 删的）。
	resolvePhys := func(rel string) string {
		rel = filepath.FromSlash(rel)
		cands := []string{
			filepath.Join(dataDir, info.Name, rel), // 标准：dataDir/<种子名>/<相对路径>
			filepath.Join(dataDir, rel),            // 扁平：dataDir/<相对路径>
			filepath.Join(dataDir, filepath.Base(rel)),
		}
		for _, c := range cands {
			if fi, err := os.Stat(c); err == nil && !fi.IsDir() {
				return c
			}
		}
		return ""
	}

	if len(files) == 1 {
		singleFile := files[0]
		// 单文件种子：DisplayPath() 在单文件种子下就等于种子名（见 anacrolix file.go 注释），
		// 物理布局 = dataDir/<DisplayPath>。
		rel := singleFile.DisplayPath()
		phys := resolvePhys(rel)
		if phys == "" {
			return fmt.Errorf("单文件种子物理路径未找到（种子名 %q，相对路径 %q，数据目录 %s）", info.Name, rel, dataDir)
		}
		vp, jerr := fscore.Join(props.Dest, rel)
		if jerr != nil {
			return jerr
		}
		return importOne(vp, phys)
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
		phys := resolvePhys(rel)
		if phys == "" {
			return fmt.Errorf("种子内文件物理路径未找到（相对路径 %q，数据目录 %s）", rel, dataDir)
		}
		if derr := p.mkdirChain(d, path.Dir(vp)); derr != nil {
			return derr
		}
		if ierr := importOne(vp, phys); ierr != nil {
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
