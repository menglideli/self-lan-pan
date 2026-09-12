package handler

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"

	"cloudpan/internal/dto"
	"cloudpan/internal/fscore"
	"cloudpan/internal/middleware"
	"cloudpan/internal/model"
)

// ---- 简易任务队列（DB 状态机 + 信号量 worker，重启恢复） ----

type TaskPool struct {
	Svc   *fscore.Service
	Zips  string
	BtDir string
	meds  chan struct{}
	mu    sync.Mutex
	opts  map[uint]bool // 取消标记 taskID
}

var pool *TaskPool

func InitTaskPool(svc *fscore.Service, btDir string) {
	pool = &TaskPool{Svc: svc, BtDir: btDir, meds: make(chan struct{}, 3), opts: map[uint]bool{}}
	pool.resume()
	go pool.sweepLoop()
}

// sweepLoop 定期清扫：孤儿上传分片 + 回收站超期项（每 6 小时）
//
// 注意：这里曾经有一个 sweepGuestWorkspace（游客 24h 临时工作区清理），它会按 mtime
// 物理删除策略根下的文件且不进回收站。单用户私有部署下挂载根就是真实目录，该任务
// 会直接删用户的数据，故已彻底移除，不要重新引入任何"按时间自动物理删除挂载目录"
// 的清理逻辑。
func (p *TaskPool) sweepLoop() {
	p.sweepUploads()
	fscore.PruneAll()
	p.sweepRecycle()
	t := time.NewTicker(6 * time.Hour)
	for range t.C {
		p.sweepUploads()
		fscore.PruneAll()
		p.sweepRecycle()
	}
}

// sweepRecycle 按权限档案的回收站保留天数永久删除超期项，并回补配额。
//
// 单用户私有部署下档案为 AdminPerms，RecycleRetentionDays = 0（永久保留），
// 因此本函数默认直接返回、不删除任何东西 —— 这是刻意的：不做"按时间自动删用户文件"。
// 若将来要通过回收站保留策略清理，改 model.AdminPerms() 里的天数即可。
func (p *TaskPool) sweepRecycle() {
	days := model.AdminPerms().RecycleRetentionDays
	if days <= 0 {
		return
	}
	cutoff := time.Now().Add(-time.Duration(days) * 24 * time.Hour)
	var users []model.User
	model.DB.Select("id").Find(&users)
	for _, u := range users {
		var items []model.RecycleItem
		model.DB.Where("user_id = ? AND deleted_at < ?", u.ID, cutoff).Find(&items)
		for _, item := range items {
			trashPhys := filepath.Join(p.Svc.RecycleDir, fmt.Sprint(u.ID), item.TrashPath)
			// 超期自动清空 = 永久物理删除：移除副本记录并对账
			fscore.HashPathGone(trashPhys)
			_ = os.RemoveAll(trashPhys)
			// 目录项的 Size 为删除前统计的递归总大小，与文件同样需要退款
			if item.Size > 0 {
				model.DB.Model(&model.User{}).Where("id = ?", u.ID).
					UpdateColumn("used_bytes", gorm.Expr("MAX(used_bytes - ?, 0)", item.Size))
			}
			model.DB.Delete(&item)
		}
	}
}

func (p *TaskPool) sweepUploads() {
	cutoff := time.Now().Add(-48 * time.Hour)
	var dead []model.UploadSession
	model.DB.Where("status != 'uploading' OR updated_at < ?", cutoff).Find(&dead)
	for _, s := range dead {
		_ = os.RemoveAll(filepath.Join(p.Svc.TmpDir, s.ID))
		model.DB.Delete(&s)
	}
	// 磁盘上有但数据库无记录的孤儿目录
	if entries, err := os.ReadDir(p.Svc.TmpDir); err == nil {
		for _, e := range entries {
			var n int64
			model.DB.Model(&model.UploadSession{}).Where("id = ?", e.Name()).Count(&n)
			if n == 0 {
				_ = os.RemoveAll(filepath.Join(p.Svc.TmpDir, e.Name()))
			}
		}
	}
}

func (p *TaskPool) resume() {
	var tasks []model.Task
	model.DB.Where("status IN ?", []string{"queued", "processing"}).Find(&tasks)
	for i := range tasks {
		t := tasks[i]
		model.DB.Model(&t).UpdateColumn("status", "queued")
		go p.run(&t)
	}
}

func (p *TaskPool) submit(t *model.Task) error {
	if err := model.DB.Create(t).Error; err != nil {
		return err
	}
	go p.run(t)
	return nil
}

func (p *TaskPool) run(t *model.Task) {
	p.meds <- struct{}{}
	defer func() { <-p.meds }()
	model.DB.Model(t).UpdateColumn("status", "processing")
	var err error
	switch t.Type {
	case "offline":
		err = p.runOffline(t)
	case "m3u8":
		err = p.runM3U8(t)
	case "bt":
		err = p.runBT(t)
	case "compress":
		err = p.runCompress(t)
	case "decompress":
		err = p.runDecompress(t)
	default:
		err = fmt.Errorf("未知任务类型 %s", t.Type)
	}
	if p.cancelled(t.ID) {
		// 取消后阶段提示必须清掉，否则界面会留着"正在下载分片 x/y"这类已经失效的文案
		model.DB.Model(t).Updates(map[string]interface{}{"status": "canceled", "msg": ""})
		return
	}
	if err != nil {
		// 失败：error 写原因；msg 里最后一条阶段提示保留（详情里能看到停在哪一步）
		model.DB.Model(t).Updates(map[string]interface{}{"status": "error", "error": err.Error()})
		Notify(t.UserID, "task", "任务失败", taskTypeName(t.Type)+"任务失败："+truncateRunes(err.Error(), 200), t.ID)
		return
	}
	// 成功：error/msg 都必须为空 —— 这里曾漏清 msg，导致任务已完成但界面仍显示"正在合并分片"
	model.DB.Model(t).Updates(map[string]interface{}{"status": "finished", "progress": 100, "error": "", "msg": ""})
	Notify(t.UserID, "task", "任务完成", taskTypeName(t.Type)+"任务完成："+taskTargetName(t), t.ID)
}

func taskTypeName(typ string) string {
	switch typ {
	case "offline":
		return "HTTP 离线下载"
	case "m3u8":
		return "m3u8 视频下载"
	case "bt":
		return "BT/磁力链下载"
	case "compress":
		return "压缩"
	case "decompress":
		return "解压"
	case "transfer":
		return "转存"
	}
	return typ
}

// taskTargetName 从任务 Props 里提取可读的目标文件名/路径
func taskTargetName(t *model.Task) string {
	var props struct {
		Name   string `json:"name"`
		RTName string `json:"rtName"` // 实际落盘名（m3u8 默认名/BT 种子名），比用户填的 name 更可信
		Path   string `json:"path"`
		URL    string `json:"url"`
	}
	if err := json.Unmarshal([]byte(t.Props), &props); err == nil {
		if props.RTName != "" {
			return truncateRunes(props.RTName, 80)
		}
		if props.Name != "" {
			return truncateRunes(props.Name, 80)
		}
		if props.Path != "" {
			return truncateRunes(props.Path, 80)
		}
		if props.URL != "" {
			return truncateRunes(props.URL, 80)
		}
	}
	return fmt.Sprintf("任务 #%d", t.ID)
}

func (p *TaskPool) cancelled(id uint) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.opts[id]
}

func (p *TaskPool) cancel(id uint) {
	p.mu.Lock()
	p.opts[id] = true
	p.mu.Unlock()
}

// uncancel 清除取消标记。重试时必须调用：否则任务刚起跑就撞上上一轮留下的标记，
// 直接被判成 canceled（状态"排队中"却永远不动）。
func (p *TaskPool) uncancel(id uint) {
	p.mu.Lock()
	delete(p.opts, id)
	p.mu.Unlock()
}

func (p *TaskPool) setProgress(id uint, pct int) {
	model.DB.Model(&model.Task{}).Where("id = ?", id).UpdateColumn("progress", pct)
}

// runOffline HTTP(S) URL 服务器代下载入网盘
func (p *TaskPool) runOffline(t *model.Task) error {
	var props struct {
		URL      string `json:"url"`
		PolicyID uint   `json:"policyId"`
		Dest     string `json:"dest"`
		Name     string `json:"name"`
		Referer  string `json:"referer"`
		UA       string `json:"ua"`
	}
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
	req, err := http.NewRequest("GET", props.URL, nil)
	if err != nil {
		return err
	}
	// 尽量模拟浏览器请求头，降低多数 CDN 基于 UA/Referer 的防盗链 403
	ua := strings.TrimSpace(props.UA)
	if ua == "" {
		ua = defaultOfflineUA
	}
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Language", "zh-CN,zh;q=0.9,en;q=0.8")
	req.Header.Set("Connection", "keep-alive")
	if ref := strings.TrimSpace(props.Referer); ref != "" {
		req.Header.Set("Referer", ref)
	}
	resp, err := ssrfHTTP.Do(req) // SSRF 防护客户端（重定向复检 + 拨号层 IP 复检）
	if err != nil {
		if !SSRFAllowPrivate() {
			return fmt.Errorf("%v（若目标在你的内网，可在管理台开启「允许离线下载访问内网地址」）", err)
		}
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("下载失败 HTTP %d：该链接可能禁止服务器端下载（防盗链/需登录/需签名），请改用可直接访问的直链，或填写该站点所需的 Referer", resp.StatusCode)
	}

	// HLS 兜底：地址没带 .m3u8 后缀、但返回的其实是清单时，转 m3u8 流程。
	// 不少站点把清单挂在 "/play?id=xxx" 这种无后缀地址上，只按后缀判断会把它当普通文件存下来
	// —— 落到网盘里的就是一个几百字节的文本，看起来就是"m3u8 下不了"。
	br := bufio.NewReaderSize(resp.Body, 1<<20)
	if head, _ := br.Peek(1024); looksLikeM3U8Body(head, resp.Header.Get("Content-Type")) {
		// 注意：Peek 不消费缓冲，下面的 ReadAll 读到的是「完整正文」（含刚 peek 的那段）。
		// 早先这里又把它拼了一次 → 清单被当成两份解析，分片数直接翻倍（实测抓到）。
		body, rerr := io.ReadAll(io.LimitReader(br, 8<<20))
		if rerr != nil {
			return fmt.Errorf("读取 m3u8 清单失败: %w", rerr)
		}
		finalURL := props.URL
		if resp.Request != nil && resp.Request.URL != nil {
			finalURL = resp.Request.URL.String()
		}
		mp := m3u8Props{
			URL: props.URL, PolicyID: props.PolicyID, Dest: props.Dest, Name: props.Name,
			Kind: "m3u8", Referer: props.Referer, UA: props.UA,
		}
		model.DB.Model(t).UpdateColumn("type", "m3u8")
		p.saveAnyProps(t.ID, mp)
		return p.runM3U8Task(t, mp, string(body), finalURL)
	}

	name := props.Name
	if name == "" {
		if u, perr := url.Parse(props.URL); perr == nil && path.Base(u.Path) != "/" && path.Base(u.Path) != "." {
			name = path.Base(u.Path)
		} else {
			name = fmt.Sprintf("download_%d.bin", t.ID)
		}
	}
	// 已有同名文件就改名（name_1、name_2…），不覆盖用户盘里已有的东西
	name, err = uniqueFileName(d, props.Dest, name)
	if err != nil {
		return err
	}
	target, err := fscore.Join(props.Dest, name)
	if err != nil {
		return err
	}
	// 先落临时文件，边下边报进度
	tmpPath := path.Join(os.TempDir(), "cp_offline_"+fmt.Sprint(t.ID)+".tmp")
	f, err := os.Create(tmpPath)
	if err != nil {
		return err
	}
	total := resp.ContentLength
	// 硬性总量上限（20GB）：即使未设配额，也防止恶意链接拖爆磁盘
	const offlineHardCap int64 = 20 << 30
	if total > offlineHardCap {
		return fmt.Errorf("文件大小 %dMB 超过 20GB 上限，已拒绝", total>>20)
	}
	// 配额预检（用户个人覆盖优先）：已知总大小且将超限时直接失败，避免白下
	if total > 0 {
		var u model.User
		if model.DB.First(&u, t.UserID).Error == nil {
			if limit, limited := effectiveQuotaBytes(&u, model.AdminPerms()); limited && u.UsedBytes+total > limit {
				return fmt.Errorf("超出配额（已用 %dMB / 上限 %dMB），未开始下载", u.UsedBytes>>20, limit>>20)
			}
		}
	}
	var written int64
	buf := make([]byte, 256<<10)
	lastPct := -1
	for {
		if p.cancelled(t.ID) {
			f.Close()
			os.Remove(tmpPath)
			return fmt.Errorf("已取消")
		}
		n, rerr := br.Read(buf)
		if n > 0 {
			written += int64(n)
			if written > offlineHardCap {
				// Content-Length 缺失/谎报时按实际流入字节兜底
				f.Close()
				os.Remove(tmpPath)
				return fmt.Errorf("下载量超过 20GB 上限，已中止")
			}
			if _, werr := f.Write(buf[:n]); werr != nil {
				f.Close()
				return werr
			}
			if total > 0 {
				pct := int(written * 100 / total)
				if pct != lastPct {
					p.setProgress(t.ID, pct)
					lastPct = pct
				}
			}
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			f.Close()
			os.Remove(tmpPath)
			return rerr
		}
	}
	f.Close()
	tf, err := os.Open(tmpPath)
	if err != nil {
		return err
	}
	defer tf.Close()
	defer os.Remove(tmpPath)
	if err := d.CreateFile(target, tf); err != nil {
		return err
	}
	// 配额记账：离线下载落盘内容计入用户用量
	model.DB.Model(&model.User{}).Where("id = ?", t.UserID).
		UpdateColumn("used_bytes", gorm.Expr("MAX(used_bytes + ?, 0)", written))
	return nil
}

// runCompress 异步压缩：把若干路径打包为 zip 写入网盘，按文件数报进度
func (p *TaskPool) runCompress(t *model.Task) error {
	var props struct {
		PolicyID uint     `json:"policyId"`
		Paths    []string `json:"paths"`
		Name     string   `json:"name"`
	}
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
	if len(props.Paths) == 0 {
		return fmt.Errorf("没有要压缩的路径")
	}
	name := props.Name
	if name == "" {
		name = fmt.Sprintf("archive_%s.zip", time.Now().Format("0102150405"))
	}
	// 按扩展名决定归档格式：.zip / .tar / .tar.gz(.tgz) / .tar.bz2(.tbz2)
	kind, tarCompress, name := archiveFormatOf(name)
	if kind == "tar" && tarCompress == "bzip2" {
		return fmt.Errorf("暂不支持打包为 .tar.bz2（Go 标准库无 bzip2 压缩；.tar.bz2 可以解压）")
	}
	// 目标目录 = 首个路径的父目录；与已有同名时自动加序号，避免覆盖
	parent := props.Paths[0]
	if i := strings.LastIndex(parent, "/"); i > 0 {
		parent = parent[:i]
	} else {
		parent = "/"
	}
	vp, err := p.uniqueArchiveName(d, parent, name)
	if err != nil {
		return err
	}
	tmpZip := filepath.Join(p.Zips, "mk_"+genID16())
	defer os.Remove(tmpZip)
	// 同一父目录下的选择用纯文件名做条目名（解压不嵌套）；跨目录时保留相对路径
	sameParent := true
	for _, pt := range props.Paths[1:] {
		if parentOf(pt) != parent {
			sameParent = false
			break
		}
	}
	items := make([]fscore.ZipItem, len(props.Paths))
	for i, pt := range props.Paths {
		it := fscore.ZipItem{Path: pt}
		if sameParent {
			it.Name = baseOf(pt)
		}
		items[i] = it
	}
	var lastPct int
	onFile := func(done, total int64) error {
		if p.cancelled(t.ID) {
			return fmt.Errorf("已取消")
		}
		if total > 0 {
			pct := int(done * 100 / total)
			if pct > lastPct && pct < 100 {
				lastPct = pct
				p.setProgress(t.ID, pct)
			}
		}
		return nil
	}
	var archSize int64
	var aerr error
	if kind == "tar" {
		archSize, aerr = p.Svc.BuildTar(d, items, tmpZip, tarCompress, onFile)
	} else {
		archSize, aerr = p.Svc.BuildZip(d, items, tmpZip, onFile)
	}
	if aerr != nil {
		return aerr
	}
	if p.cancelled(t.ID) {
		return fmt.Errorf("已取消")
	}
	// 配额预检（用户个人覆盖优先）：超限则失败，不落盘
	var u model.User
	if model.DB.First(&u, t.UserID).Error == nil {
		if limit, limited := effectiveQuotaBytes(&u, model.AdminPerms()); limited && u.UsedBytes+archSize > limit {
			return fmt.Errorf("超出配额（已用 %dMB / 上限 %dMB），压缩结果未保存", u.UsedBytes>>20, limit>>20)
		}
	}
	f, err := os.Open(tmpZip)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := d.CreateFile(vp, f); err != nil {
		return fmt.Errorf("写入失败: %w", err)
	}
	addQuota(t.UserID, archSize)
	return nil
}

// runDecompress 异步解压：把网盘里的 zip 解压到其所在目录，按条目数报进度
func (p *TaskPool) runDecompress(t *model.Task) error {
	var props struct {
		PolicyID uint   `json:"policyId"`
		Path     string `json:"path"`
	}
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
	if props.Path == "" {
		return fmt.Errorf("没有要解压的文件")
	}
	var lastPct int
	onFile := func(done, total int) error {
		if p.cancelled(t.ID) {
			return fmt.Errorf("已取消")
		}
		if total > 0 {
			pct := done * 100 / total
			if pct > lastPct && pct < 100 {
				lastPct = pct
				p.setProgress(t.ID, pct)
			}
		}
		return nil
	}
	// 配额预检：解压产物（未压缩总大小）将超限时整体中止、不落盘
	precheck := func(total int64) error {
		var u model.User
		if model.DB.First(&u, t.UserID).Error != nil {
			return nil
		}
		if limit, limited := effectiveQuotaBytes(&u, model.AdminPerms()); limited && u.UsedBytes+total > limit {
			return fmt.Errorf("超出配额（已用 %dMB / 上限 %dMB），解压未开始", u.UsedBytes>>20, limit>>20)
		}
		return nil
	}
	// 解压写入的字节计入用户配额（与删除/清空回收站的冲销配对，避免账目漂移）
	// zip 有中央目录可先预检；tar 无目录表，解压后再记账
	var written int64
	var err2 error
	low := strings.ToLower(props.Path)
	if strings.HasSuffix(low, ".tar") || strings.HasSuffix(low, ".tar.gz") ||
		strings.HasSuffix(low, ".tgz") || strings.HasSuffix(low, ".tar.bz2") || strings.HasSuffix(low, ".tbz2") {
		written, err2 = p.Svc.ExtractTar(d, props.Path, onFile)
	} else {
		written, err2 = p.Svc.ExtractZip(d, props.Path, onFile, precheck)
	}
	if err2 != nil {
		return err2
	}
	addQuota(t.UserID, written)
	return nil
}

// archiveFormatOf 按文件名判定归档格式，返回 (kind, compress, canonicalName)。
// kind: "zip"|"tar"；compress: ""|"gzip"|"bzip2"（仅 tar）；canonicalName 归一化后缀
func archiveFormatOf(name string) (kind, compress, canonical string) {
	low := strings.ToLower(name)
	switch {
	case strings.HasSuffix(low, ".tar.gz") || strings.HasSuffix(low, ".tgz"):
		if strings.HasSuffix(low, ".tgz") {
			name = strings.TrimSuffix(name, ".tgz") + ".tar.gz"
		}
		return "tar", "gzip", name
	case strings.HasSuffix(low, ".tar.bz2") || strings.HasSuffix(low, ".tbz2"):
		if strings.HasSuffix(low, ".tbz2") {
			name = strings.TrimSuffix(name, ".tbz2") + ".tar.bz2"
		}
		return "tar", "bzip2", name
	case strings.HasSuffix(low, ".tar"):
		return "tar", "", name
	default:
		if !strings.HasSuffix(low, ".zip") {
			name += ".zip"
		}
		return "zip", "", name
	}
}

// archiveSuffixOf 返回归档文件的完整后缀（用于冲突改名时保留扩展名）
func archiveSuffixOf(name string) string {
	low := strings.ToLower(name)
	for _, s := range []string{".tar.gz", ".tar.bz2", ".tar", ".zip"} {
		if strings.HasSuffix(low, s) {
			return s
		}
	}
	return ""
}

// uniqueArchiveName 在 dir 下为 name 找一个不与现有文件冲突的虚拟路径（保留归档扩展名）
func (p *TaskPool) uniqueArchiveName(d fscore.Driver, dir, name string) (string, error) {
	suf := archiveSuffixOf(name)
	base := strings.TrimSuffix(name, suf)
	cand, err := fscore.Join(dir, name)
	if err != nil {
		return "", err
	}
	for n := 1; ; n++ {
		if _, serr := d.Stat(cand); serr != nil {
			return cand, nil // 不存在 → 可用
		}
		cand, err = fscore.Join(dir, fmt.Sprintf("%s_%d%s", base, n, suf))
		if err != nil {
			return "", err
		}
	}
}

// accountQuota 任务下载内容计入用户配额（BT 多文件导入用，defer 调用）
func (p *TaskPool) accountQuota(t *model.Task, bytes *int64) {
	if *bytes > 0 {
		model.DB.Model(&model.User{}).Where("id = ?", t.UserID).
			UpdateColumn("used_bytes", gorm.Expr("MAX(used_bytes + ?, 0)", *bytes))
	}
}

// ---- 任务产物命名 ----

// mediaDefaultExt 视频任务的默认扩展名。
//
// HLS 分片合并出来的其实是 MPEG-TS 流，这里按用户要求统一用 .mp4 命名
//（VLC / PotPlayer / mpv / 手机播放器都按内容识别，能正常播；
// Windows 自带「电影和电视」可能不认 —— 要改回 .ts 只需改这一个常量）。
const mediaDefaultExt = ".mp4"

// resolveOutName 生成任务产物文件名：
//   - 用户填了名字 → 用他的（没有扩展名时补 defaultExt）
//   - 没填 → "YYYYMMDD_NN" + defaultExt，NN = 目标目录里当天已有同款文件的最大编号 +1（两位）
//
// 非法名称直接报错，不静默改写：否则用户指定的名字落盘成别的样子，只会更让人困惑。
func resolveOutName(d fscore.Driver, dir, userName, defaultExt string) (string, error) {
	if n := strings.TrimSpace(userName); n != "" {
		if strings.ContainsAny(n, `/\`) {
			return "", fmt.Errorf("文件名不能包含路径分隔符（/ 或 \\）")
		}
		n = strings.TrimRight(n, " .") // Windows 会静默吃掉结尾空格与点
		if n == "" {
			return "", fmt.Errorf("文件名不能为空")
		}
		if path.Ext(n) == "" {
			n += defaultExt
		}
		safe, err := fscore.SanitizeName(n)
		if err != nil {
			return "", err
		}
		return safe, nil
	}
	return nextDailyName(d, dir, defaultExt), nil
}

// nextDailyName "YYYYMMDD_NN<ext>"：NN 取目标目录里今天已有的最大编号 + 1（两位，超过 99 自然进位）
func nextDailyName(d fscore.Driver, dir, ext string) string {
	day := time.Now().Format("20060102")
	maxN := 0
	if ents, err := d.List(dir); err == nil {
		for _, e := range ents {
			if e.IsDir || !strings.HasPrefix(e.Name, day+"_") || !strings.EqualFold(path.Ext(e.Name), ext) {
				continue
			}
			rest := strings.TrimSuffix(strings.TrimPrefix(e.Name, day+"_"), path.Ext(e.Name))
			if n, err := strconv.Atoi(rest); err == nil && n > maxN {
				maxN = n
			}
		}
	}
	return fmt.Sprintf("%s_%02d%s", day, maxN+1, ext)
}

// uniqueFileName 目录下已有同名文件时追加 _1/_2…（绝不覆盖既有文件）
func uniqueFileName(d fscore.Driver, dir, name string) (string, error) {
	ext := path.Ext(name)
	base := strings.TrimSuffix(name, ext)
	cand := name
	for i := 1; ; i++ {
		vp, err := fscore.Join(dir, cand)
		if err != nil {
			return "", err
		}
		if _, serr := d.Stat(vp); serr != nil {
			return cand, nil // 不存在 → 可用
		}
		cand = fmt.Sprintf("%s_%d%s", base, i, ext)
	}
}

// resetTaskProps 重试前清掉上一轮运行写入的字段（分片进度/码率/产物名/节点数），
// 保留 url / dest / name / policyId 等真正的入参。
func resetTaskProps(raw string) string {
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &m); err != nil || len(m) == 0 {
		return raw
	}
	for _, k := range []string{"segDone", "segTotal", "variant", "rtName", "note", "seeds", "peers", "live"} {
		delete(m, k)
	}
	if b, err := json.Marshal(m); err == nil {
		return string(b)
	}
	return raw
}

// ---- Offline API ----

type OfflineHandler struct{}

func (h *OfflineHandler) Create(c *gin.Context) {
	x := ctxOf(c)
	if !x.perm.AllowOffline {
		dto.Fail(c, 403, "当前用户组未启用离线下载")
		return
	}
	var in struct {
		PolicyID uint   `json:"policyId" binding:"required"`
		Dest     string `json:"dest"`
		URL      string `json:"url" binding:"required"`
		Name     string `json:"name"`
		// 可选请求头覆盖：相当多站点（尤其 m3u8）是按 Referer 判防盗链的
		Referer string `json:"referer"`
		UA      string `json:"ua"`
		Threads int    `json:"threads"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	dest, err := fscore.Clean(in.Dest)
	if err != nil || dest == "" {
		dest = "/"
	}
	// 文件名（可选）：这里只做校验，真正补扩展名/生成默认名在下发任务时按类型处理
	// （m3u8 视频：没填 → YYYYMMDD_NN.mp4，没扩展名 → 补 .mp4；见 resolveOutName）
	if in.Name = strings.TrimSpace(in.Name); in.Name != "" {
		if strings.ContainsAny(in.Name, `/\`) {
			dto.Fail(c, 400, "文件名不能包含路径分隔符（/ 或 \\）")
			return
		}
		if _, jerr := fscore.Join(dest, in.Name); jerr != nil {
			dto.Fail(c, 400, "文件名不合法："+jerr.Error())
			return
		}
	}
	if !x.perm.CanUsePolicy(in.PolicyID) && x.user.Role != "admin" {
		dto.Fail(c, 403, "无权使用该存储")
		return
	}
	kind := sniffOfflineKind(in.URL)
	trimmed := strings.TrimSpace(in.URL)
	// SSRF 防护：非磁力链（http 直链 / .torrent / m3u8）都按 URL 校验，禁止内网/保留地址；
	// 管理台开启「允许离线下载访问内网地址」后该判定整体放行
	if !strings.HasPrefix(trimmed, "magnet:") {
		if err := validateFetchURL(in.URL); err != nil {
			msg := err.Error()
			if !SSRFAllowPrivate() {
				msg += "（若目标就在你的内网，可在管理台开启「允许离线下载访问内网地址」）"
			}
			dto.Fail(c, 400, msg)
			return
		}
	}
	if kind == "bt" {
		// BT/磁力要求「BT/磁力」功能独立开启（offline_http 只覆盖 HTTP 直链，
		// 避免仅有直链权限的用户借磁力链走 P2P 下载通道）
		if !model.AppAllowed("bt", x.user) {
			dto.Fail(c, 403, "BT/磁力功能未启用")
			return
		}
		if strings.HasPrefix(trimmed, "magnet:") {
			if err := validateMagnetTrackers(trimmed); err != nil {
				dto.Fail(c, 400, err.Error())
				return
			}
		}
	}
	props, _ := json.Marshal(map[string]interface{}{
		"url": in.URL, "policyId": in.PolicyID, "dest": dest, "name": in.Name, "kind": kind,
		"referer": strings.TrimSpace(in.Referer), "ua": strings.TrimSpace(in.UA), "threads": in.Threads,
	})
	taskType := "offline"
	switch kind {
	case "bt":
		taskType = "bt"
	case "m3u8":
		taskType = "m3u8"
	}
	t := &model.Task{UserID: x.user.ID, Type: taskType, Props: string(props)}
	if err := pool.submit(t); err != nil {
		dto.Fail(c, 500, err.Error())
		return
	}
	middleware.Audit(c, "offline", in.URL)
	dto.OK(c, t)
}

func (h *OfflineHandler) List(c *gin.Context) {
	x := ctxOf(c)
	var items []model.Task
	model.DB.Where("user_id = ? AND type IN ?", x.user.ID, []string{"offline", "bt", "m3u8"}).Order("id DESC").Limit(50).Find(&items)
	dto.OK(c, items)
}

// Cancel 取消进行中的任务（执行体会在下一次检查点退出）
func (h *OfflineHandler) Cancel(c *gin.Context) {
	x := ctxOf(c)
	id := parseUintParam(c, "id")
	var t model.Task
	if err := model.DB.Where("id = ? AND user_id = ?", id, x.user.ID).First(&t).Error; err != nil {
		dto.Fail(c, 404, "任务不存在")
		return
	}
	pool.cancel(id)
	// msg 一并清掉：取消后界面不该再挂着"正在下载分片 x/y"这类已失效的阶段文案
	model.DB.Model(&t).Updates(map[string]interface{}{"status": "canceled", "msg": ""})
	dto.OK(c, nil)
}

// Retry 重试失败 / 已取消的任务：就地重置为排队并重新执行（不产生重复记录）。
//
// 必须显式 uncancel —— 上一轮若被取消过，取消标记还留在任务池里，
// 不清掉的话新起的执行体第一次检查就自我了断，任务会永远停在"排队中"。
func (h *OfflineHandler) Retry(c *gin.Context) {
	x := ctxOf(c)
	id := parseUintParam(c, "id")
	var t model.Task
	if err := model.DB.Where("id = ? AND user_id = ?", id, x.user.ID).First(&t).Error; err != nil {
		dto.Fail(c, 404, "任务不存在")
		return
	}
	if t.Status == "queued" || t.Status == "processing" {
		dto.Fail(c, 400, "任务正在执行中，无需重试")
		return
	}
	model.DB.Model(&t).Updates(map[string]interface{}{
		"status": "queued", "progress": 0, "error": "", "msg": "",
		"props": resetTaskProps(t.Props),
	})
	pool.uncancel(id)
	// 重新取一次（Updates 不回写结构体），执行体需要带上最新的 Props/Status
	if err := model.DB.First(&t, id).Error; err != nil {
		dto.Fail(c, 500, "任务重试失败")
		return
	}
	go pool.run(&t)
	middleware.Audit(c, "offline", fmt.Sprintf("重试任务 #%d", t.ID))
	dto.OK(c, t)
}

// Delete 删除任务记录（仅限已结束的任务）。
//
// 进行中的任务不给删：执行体还在跑，删了记录它照样会把文件写进网盘
//（用户以为删了却冒出个文件）。要先取消再删除。
func (h *OfflineHandler) Delete(c *gin.Context) {
	x := ctxOf(c)
	id := parseUintParam(c, "id")
	var t model.Task
	if err := model.DB.Where("id = ? AND user_id = ?", id, x.user.ID).First(&t).Error; err != nil {
		dto.Fail(c, 404, "任务不存在")
		return
	}
	if t.Status == "queued" || t.Status == "processing" {
		dto.Fail(c, 400, "任务正在执行中：请先「取消」，再删除")
		return
	}
	pool.uncancel(id)
	if err := model.DB.Delete(&model.Task{}, t.ID).Error; err != nil {
		dto.Fail(c, 500, "删除失败："+err.Error())
		return
	}
	middleware.Audit(c, "offline", fmt.Sprintf("删除任务 #%d (%s)", t.ID, t.Type))
	dto.OK(c, nil)
}

func parseUintParam(c *gin.Context, key string) uint {
	var v uint64
	fmt.Sscanf(c.Param(key), "%d", &v)
	return uint(v)
}

var _ = context.Background
var _ = time.Now
