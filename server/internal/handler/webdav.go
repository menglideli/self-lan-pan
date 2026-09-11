package handler

import (
	"context"
	"errors"
	"fmt"
	"html"
	"io"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"golang.org/x/net/webdav"

	"cloudpan/internal/fscore"
	"cloudpan/internal/model"
)

// ---- WebDAV ----
//
// 两种入口，同一套文件系统：
//
//	/dav/             统一根：列出全部挂载项。挂一次就能看到全部（推荐）
//	/dav/<挂载名>/     单个挂载：只想单独挂某一个时用
//
// 路径段用「挂载名」：不适合出现在 URL 里的字符替换为 `_`，重名自动加 `-盘符` 后缀；
// 历史盘符形式（/dav/C/…）保留为兼容别名。
//
// 只覆盖「本机文件夹挂载」：云盘驱动在这里没有可用的写语义，且本机无法实测，
// 与其摆一个连不出内容的空目录，不如不收进 WebDAV（索引页会如实说明有几个云盘挂载不在其中）。

const davPrefix = "/dav"

var (
	// errDavReadOnly 只读位置（统一根）上的写操作
	errDavReadOnly = errors.New("该位置在 WebDAV 上为只读")
	// errDavCrossMount 跨挂载的移动或复制
	errDavCrossMount = errors.New("不能跨挂载移动或复制")
)

// DavMount 一台可经 WebDAV 访问的挂载，以及它在统一根下的路径段
type DavMount struct {
	Pol *model.Policy
	Seg string
}

// DavMounts 返回全部可经 WebDAV 访问的挂载（本机文件夹，停用的除外）。
// 顺序按盘符稳定排序，重名自动去重。
func DavMounts() []DavMount {
	var ps []model.Policy
	model.DB.Where("status <> ? AND type = ?", "disabled", "local").Order("letter").Find(&ps)
	used := make(map[string]bool, len(ps))
	out := make([]DavMount, 0, len(ps))
	for i := range ps {
		p := &ps[i]
		seg := davSeg(p.Name, p.Letter)
		if used[seg] {
			// 重名：追加盘符后缀（盘符唯一，通常一次就够），仍冲突则再加序号
			base := seg + "-" + strings.ToLower(p.Letter)
			seg = base
			for n := 2; used[seg]; n++ {
				seg = fmt.Sprintf("%s-%d", base, n)
			}
		}
		used[seg] = true
		out = append(out, DavMount{Pol: p, Seg: seg})
	}
	return out
}

// davNonLocalNames 不经 WebDAV 的挂载名（云盘），仅用于索引页如实说明
func davNonLocalNames() []string {
	var ps []model.Policy
	model.DB.Where("status <> ? AND type <> ?", "disabled", "local").Order("letter").Find(&ps)
	out := make([]string, 0, len(ps))
	for _, p := range ps {
		out = append(out, p.Name)
	}
	return out
}

// davSeg 挂载名 → URL 路径段。名称不含可用字符时退回盘符。
func davSeg(name, letter string) string {
	var b strings.Builder
	for _, r := range strings.TrimSpace(name) {
		switch {
		case r < 0x20, r == 0x7f, strings.ContainsRune(`/\:*?"<>|`, r):
			b.WriteRune('_')
		default:
			b.WriteRune(r)
		}
	}
	// 首尾的点和空格在多数客户端上会被吃掉，去掉避免"看得见进不去"
	s := strings.Trim(b.String(), " .")
	if s == "" {
		s = strings.TrimSpace(letter)
	}
	if s == "" {
		s = "mount"
	}
	return s
}

// davPathOf 该挂载在统一根下的路径（如 /dav/电影/）；不可用时返回空串。
func davPathOf(id uint) string {
	for _, m := range DavMounts() {
		if m.Pol.ID == id {
			return davPrefix + "/" + m.Seg + "/"
		}
	}
	return ""
}

// davFind 按路径段找挂载：先精确匹配挂载名，再大小写不敏感，最后退回盘符别名。
func davFind(seg string) *DavMount {
	ms := DavMounts()
	for i := range ms {
		if ms[i].Seg == seg {
			return &ms[i]
		}
	}
	for i := range ms {
		if strings.EqualFold(ms[i].Seg, seg) {
			return &ms[i]
		}
	}
	for i := range ms {
		if ms[i].Pol.Letter != "" && strings.EqualFold(ms[i].Pol.Letter, seg) {
			return &ms[i]
		}
	}
	return nil
}

// davTarget 解析后的目标
type davTarget struct {
	Mount *DavMount // nil = 统一根（虚拟目录）
	Path  string    // 挂载内的虚拟路径，始终以 / 开头
}

func (d *DavFS) target(name string) (davTarget, error) {
	p := strings.Trim(name, "/")
	if p == "" {
		return davTarget{Path: "/"}, nil
	}
	seg, rest, _ := strings.Cut(p, "/")
	m := davFind(seg)
	if m == nil {
		return davTarget{}, os.ErrNotExist
	}
	vp, err := fscore.Clean("/" + rest)
	if err != nil {
		return davTarget{}, os.ErrNotExist
	}
	return davTarget{Mount: m, Path: vp}, nil
}

// davFileInfo 虚拟条目（统一根、挂载点、云盘条目）
type davFileInfo struct {
	name  string
	dir   bool
	size  int64
	mtime time.Time
}

func (f *davFileInfo) Name() string { return f.name }
func (f *davFileInfo) Size() int64  { return f.size }
func (f *davFileInfo) Mode() fs.FileMode {
	if f.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (f *davFileInfo) ModTime() time.Time { return f.mtime }
func (f *davFileInfo) IsDir() bool        { return f.dir }
func (f *davFileInfo) Sys() interface{}   { return nil }

// ---- 目录文件：统一根 ----

type davDirFile struct {
	info  os.FileInfo
	items []os.FileInfo
	off   int
}

func (r *davDirFile) Stat() (os.FileInfo, error) { return r.info, nil }
func (r *davDirFile) Readdir(n int) ([]os.FileInfo, error) {
	if r.off >= len(r.items) {
		return nil, io.EOF
	}
	if n <= 0 {
		out := r.items[r.off:]
		r.off = len(r.items)
		return out, nil
	}
	end := r.off + n
	if end > len(r.items) {
		end = len(r.items)
	}
	out := r.items[r.off:end]
	r.off = end
	return out, nil
}
func (r *davDirFile) Read([]byte) (int, error)       { return 0, errors.New("是一个目录") }
func (r *davDirFile) Write([]byte) (int, error)      { return 0, errors.New("是一个目录") }
func (r *davDirFile) Seek(int64, int) (int64, error) { return 0, errors.New("是一个目录") }
func (r *davDirFile) Close() error                   { return nil }

// ---- 写入缓冲：本机挂载的 PUT/COPY 目标 ----
//
// 与 LocalDriver.CreateFile 同一策略：先写同目录临时文件，Close 时原子替换正式文件。
// 下游（同步/播放/预览）永远不会读到写了一半的文件。
//
// 注意 Close 里的顺序：必须先关掉读句柄再删临时文件——Windows 上删一个仍被打开的
// 文件会失败（共享受限），漏掉就会在用户的目录里留下 .cp-put-* 垃圾。

type davWriteFile struct {
	*os.File
	ld *fscore.LocalDriver
	vp string
}

func (w *davWriteFile) Close() error {
	tmp := w.File.Name()
	if err := w.File.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	src, err := os.Open(tmp)
	if err != nil {
		_ = os.Remove(tmp)
		return err
	}
	cerr := w.ld.CreateFile(w.vp, src)
	_ = src.Close()
	_ = os.Remove(tmp)
	return cerr
}

// DavFS 把存储策略映射为 WebDAV 文件系统
type DavFS struct {
	Svc *fscore.Service
}

// davLocal 本机挂载的驱动（挂载根即真实目录；不做 MkdirAll，挂载根消失就报不存在）
func davLocal(m *DavMount) (*fscore.LocalDriver, error) {
	if m.Pol.Type != "local" {
		return nil, os.ErrNotExist
	}
	abs, err := filepath.Abs(m.Pol.RootPath)
	if err != nil {
		return nil, os.ErrNotExist
	}
	st, err := os.Stat(abs)
	if err != nil || !st.IsDir() {
		return nil, os.ErrNotExist
	}
	return &fscore.LocalDriver{Root: abs}, nil
}

func (d *DavFS) Stat(ctx context.Context, name string) (os.FileInfo, error) {
	t, err := d.target(name)
	if err != nil {
		return nil, err
	}
	if t.Mount == nil {
		return &davFileInfo{name: "/", dir: true, mtime: time.Now()}, nil
	}
	ld, err := davLocal(t.Mount)
	if err != nil {
		return nil, err
	}
	phys, err := ld.Physical(t.Path)
	if err != nil {
		return nil, os.ErrNotExist
	}
	st, err := os.Stat(phys)
	if err != nil {
		return nil, os.ErrNotExist
	}
	if t.Path == "/" {
		// 挂载点用挂载名对外，避免暴露本机物理目录名
		return &davFileInfo{name: t.Mount.Seg, dir: true, mtime: st.ModTime()}, nil
	}
	return st, nil
}

func (d *DavFS) OpenFile(ctx context.Context, name string, flag int, perm os.FileMode) (webdav.File, error) {
	t, err := d.target(name)
	if err != nil {
		return nil, err
	}
	write := flag&(os.O_WRONLY|os.O_RDWR|os.O_CREATE|os.O_TRUNC|os.O_APPEND) != 0

	// 统一根：虚拟目录，只读
	if t.Mount == nil {
		if write {
			return nil, errDavReadOnly
		}
		items := make([]os.FileInfo, 0, 8)
		for _, m := range DavMounts() {
			items = append(items, &davFileInfo{name: m.Seg, dir: true, mtime: m.Pol.CreatedAt})
		}
		return &davDirFile{info: &davFileInfo{name: "/", dir: true, mtime: time.Now()}, items: items}, nil
	}

	ld, err := davLocal(t.Mount)
	if err != nil {
		return nil, err
	}
	if err != nil {
		return nil, os.ErrNotExist
	}
	phys, err := ld.Physical(t.Path)
	if err != nil {
		return nil, os.ErrNotExist
	}
	if write {
		// 只在真正要写时补父目录：读请求（PROPFIND/GET）绝不能凭一个不存在的路径建出目录
		_ = os.MkdirAll(filepath.Dir(phys), 0o755)
		tmp := fmt.Sprintf("%s.cp-put-%d", phys, time.Now().UnixNano())
		f, err := os.OpenFile(tmp, os.O_RDWR|os.O_CREATE|os.O_TRUNC, perm)
		if err != nil {
			return nil, err
		}
		return &davWriteFile{File: f, ld: ld, vp: t.Path}, nil
	}
	return os.OpenFile(phys, flag, perm)
}

func (d *DavFS) Mkdir(ctx context.Context, name string, perm os.FileMode) error {
	t, err := d.target(name)
	if err != nil {
		return err
	}
	if t.Mount == nil || t.Path == "/" {
		return errDavReadOnly
	}
	ld, err := davLocal(t.Mount)
	if err != nil {
		return err
	}
	phys, err := ld.Physical(t.Path)
	if err != nil {
		return os.ErrNotExist
	}
	return os.Mkdir(phys, perm)
}

func (d *DavFS) RemoveAll(ctx context.Context, name string) error {
	t, err := d.target(name)
	if err != nil {
		return err
	}
	if t.Mount == nil || t.Path == "/" {
		return errDavReadOnly
	}
	ld, err := davLocal(t.Mount)
	if err != nil {
		return err
	}
	phys, err := ld.Physical(t.Path)
	if err != nil {
		return os.ErrNotExist
	}
	return os.RemoveAll(phys)
}

func (d *DavFS) Rename(ctx context.Context, oldName, newName string) error {
	ot, err := d.target(oldName)
	if err != nil {
		return err
	}
	nt, err := d.target(newName)
	if err != nil {
		return err
	}
	if ot.Mount == nil || nt.Mount == nil {
		return errDavReadOnly
	}
	if ot.Mount.Pol.ID != nt.Mount.Pol.ID {
		return errDavCrossMount
	}
	ld, err := davLocal(ot.Mount)
	if err != nil {
		return err
	}
	oldPhys, err := ld.Physical(ot.Path)
	if err != nil {
		return os.ErrNotExist
	}
	newPhys, err := ld.Physical(nt.Path)
	if err != nil {
		return os.ErrNotExist
	}
	return os.Rename(oldPhys, newPhys)
}

// ---- 认证 ----

// DavAuth Basic Auth：Web 登录账号 + 独立 WebDAV 密码
func DavAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		settings := GetSiteSettings()
		if settings["webdav_enabled"] != "true" {
			c.String(http.StatusForbidden, "WebDAV 未启用")
			c.Abort()
			return
		}
		user, pass, ok := c.Request.BasicAuth()
		if !ok {
			c.Header("WWW-Authenticate", `Basic realm="CloudPan WebDAV"`)
			c.String(http.StatusUnauthorized, "需要认证")
			c.Abort()
			return
		}
		// 与 Web 登录同一套失败锁定（IP+用户名，5 次失败锁 15 分钟），防 WebDAV 密码暴破
		lockKey := c.ClientIP() + "|" + user
		if loginLocked(lockKey) {
			c.String(http.StatusTooManyRequests, "失败次数过多，已临时锁定，请 15 分钟后再试")
			c.Abort()
			return
		}
		var u model.User
		if err := model.DB.Where("username = ? AND disabled = false", user).First(&u).Error; err != nil {
			loginFail(lockKey)
			c.String(http.StatusUnauthorized, "用户不存在")
			c.Abort()
			return
		}
		if u.WebdavPasswordHash == "" || bcryptCompare(u.WebdavPasswordHash, pass) != nil {
			loginFail(lockKey)
			c.String(http.StatusUnauthorized, "WebDAV 密码错误（请在设置中先行设置）")
			c.Abort()
			return
		}
		loginFails.Delete(lockKey)
		// 单用户私有部署：WebDAV 可用性与只读标志都来自代码写死的权限档案
		perm := model.AdminPerms()
		if !perm.AllowWebdav {
			c.String(http.StatusForbidden, "WebDAV 未启用")
			c.Abort()
			return
		}
		if perm.ReadOnly {
			switch c.Request.Method {
			case http.MethodPut, http.MethodDelete, http.MethodPost, "MKCOL", "COPY", "MOVE", "PROPPATCH":
				c.String(http.StatusForbidden, "当前为只读模式")
				c.Abort()
				return
			}
		}
		c.Set("davUser", &u)
		c.Next()
	}
}

// davIndexPage 浏览器直接访问统一根时的索引页（WebDAV 客户端走 PROPFIND，不看这个）
func davIndexPage() string {
	var b strings.Builder
	b.WriteString(`<!doctype html><html lang="zh-CN"><head><meta charset="utf-8">`)
	b.WriteString(`<meta name="viewport" content="width=device-width,initial-scale=1">`)
	b.WriteString(`<title>CloudPan WebDAV</title><style>`)
	b.WriteString(`body{margin:0;padding:48px 20px;font:15px/1.7 system-ui,"Segoe UI","Microsoft YaHei",sans-serif;`)
	b.WriteString(`background:#f5f6f8;color:#1b1d21}h1{margin:0 0 6px;font-size:22px}p.sub{margin:0 0 26px;color:#666}`)
	b.WriteString(`ul{list-style:none;margin:0;padding:0;max-width:720px}li{margin:0 0 10px}`)
	b.WriteString(`a{display:flex;align-items:center;gap:10px;padding:13px 16px;background:#fff;border:1px solid #e2e5ea;`)
	b.WriteString(`border-radius:10px;text-decoration:none;color:#1b1d21;box-shadow:0 1px 2px rgba(0,0,0,.03)}`)
	b.WriteString(`a:hover{border-color:#b9c0cb}.n{flex:1;font-weight:600}.t{color:#8a9099;font-size:12.5px}`)
	b.WriteString(`.empty{padding:18px;background:#fff;border:1px dashed #ccd2da;border-radius:10px;color:#666;max-width:688px}`)
	b.WriteString(`.tip{max-width:688px;margin-top:26px;color:#666;font-size:13px;line-height:1.9}`)
	b.WriteString(`code{background:#eceff3;padding:2px 6px;border-radius:5px}</style></head><body>`)
	b.WriteString(`<h1>CloudPan WebDAV</h1><p class="sub">统一入口：挂这一个地址，就能看到下面全部挂载。</p>`)
	ms := DavMounts()
	if len(ms) == 0 {
		b.WriteString(`<div class="empty">还没有挂载任何文件夹。请到「此电脑 → 挂载文件夹」添加。</div>`)
	} else {
		b.WriteString(`<ul>`)
		for _, m := range ms {
			seg := url.PathEscape(m.Seg)
			sub := "本机文件夹"
			if m.Pol.RootPath != "" {
				sub += " · " + m.Pol.RootPath
			}
			b.WriteString(`<li><a href="` + davPrefix + `/` + seg + `/">` +
				`<span class="n">` + html.EscapeString(m.Seg) + `</span>` +
				`<span class="t">` + html.EscapeString(sub) + `</span></a></li>`)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<div class="tip">把这个地址加进资源管理器／手机的 WebDAV 客户端，就能一次看到以上全部挂载。<br>`)
	b.WriteString(`单个挂载也可以单独添加：<code>` + davPrefix + `/&lt;名称&gt;/</code>。<br>`)
	b.WriteString(`登录用网页账号，密码是设置里单独设的 WebDAV 密码。</div>`)
	// 如实交代边界：云盘挂载不在 WebDAV 里，免得用户以为"加了却没出现"
	if names := davNonLocalNames(); len(names) > 0 {
		b.WriteString(`<div class="tip">另有 ` + fmt.Sprint(len(names)) + ` 个云盘挂载不经 WebDAV：` +
			html.EscapeString(strings.Join(names, "、")) + `（请用网页文件管理器访问）。</div>`)
	}
	b.WriteString(`</body></html>`)
	return b.String()
}

func DavHandler(svc *fscore.Service) http.Handler {
	davFS := &DavFS{Svc: svc}
	h := webdav.Handler{
		Prefix:     davPrefix,
		FileSystem: davFS,
		LockSystem: webdav.NewMemLS(),
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// 统一根：浏览器直接打开时给一份人看的目录页；
		// WebDAV 客户端用 PROPFIND/OPTIONS，交给标准处理器（它走 FileSystem 的虚拟根）
		if p := strings.TrimSuffix(r.URL.Path, "/"); p == davPrefix {
			if r.Method == http.MethodGet || r.Method == http.MethodHead {
				w.Header().Set("Content-Type", "text/html; charset=utf-8")
				if r.Method == http.MethodGet {
					_, _ = io.WriteString(w, davIndexPage())
				}
				return
			}
		}
		h.ServeHTTP(w, r)
	})
}

// RegisterDav 挂到 gin（非 /api 前缀）。WebDAV 方法多为非标准动词，需逐个注册
func RegisterDav(r *gin.Engine, svc *fscore.Service) {
	handler := DavHandler(svc)
	dav := func(c *gin.Context) {
		// 功能门控：WebDAV 被管理员停用时拒绝挂载
		if !model.AppEnabled("webdav") {
			c.String(http.StatusForbidden, "WebDAV 已被管理员停用")
			return
		}
		DavAuth()(c)
		if c.IsAborted() {
			return
		}
		handler.ServeHTTP(c.Writer, c.Request)
	}
	methods := []string{"GET", "HEAD", "POST", "PUT", "DELETE", "OPTIONS", "PROPFIND", "PROPPATCH", "MKCOL", "COPY", "MOVE", "LOCK", "UNLOCK"}
	for _, m := range methods {
		// 两种入口都注册：/dav（无尾斜杠）与 /dav/…（含统一根）
		r.Handle(m, davPrefix, dav)
		r.Handle(m, davPrefix+"/*rest", dav)
	}
}
