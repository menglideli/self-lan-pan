package handler

import (
	"encoding/csv"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/dto"
	"cloudpan/internal/fscore"
	"cloudpan/internal/middleware"
	"cloudpan/internal/model"
)

type AdminHandler struct{ Site *SiteHandler }

// ---- 仪表盘 ----

func (h *AdminHandler) Dashboard(c *gin.Context) {
	var users, shares, policies, recycle int64
	model.DB.Model(&model.User{}).Count(&users)
	model.DB.Model(&model.Share{}).Count(&shares)
	model.DB.Model(&model.Policy{}).Count(&policies)
	model.DB.Model(&model.RecycleItem{}).Count(&recycle)
	var usedSum int64
	model.DB.Model(&model.User{}).Select("COALESCE(SUM(used_bytes),0)").Scan(&usedSum)
	// 最近 7 天新增用户
	dto.OK(c, gin.H{
		"users": users, "shares": shares, "policies": policies, "recycle": recycle,
		"usedBytes": usedSum, "serverTime": nowUnix(),
	})
}

// ---- 存储策略 ----

type policyIn struct {
	Name string `json:"name" binding:"required"`
	// Letter 可选：单用户私有部署下前端不再让用户填盘符，留空由后端自动生成（见 genPolicyLetter）
	Letter   string            `json:"letter"`
	Type     string            `json:"type" binding:"required"`
	RootPath string            `json:"rootPath"`
	Options  map[string]string `json:"options"`
}

func (h *AdminHandler) PolicyList(c *gin.Context) {
	var items []model.Policy
	model.DB.Order("letter").Find(&items)
	// admin 专用 DTO：包含 options（密钥），普通用户接口不会暴露
	out := make([]gin.H, 0, len(items))
	for _, p := range items {
		out = append(out, gin.H{
			"id": p.ID, "name": p.Name, "letter": p.Letter, "type": p.Type,
			"rootPath": p.RootPath, "options": p.Opts(), "status": p.Status,
			"statusMsg": p.StatusMsg, "usageBytes": p.UsageBytes, "createdAt": p.CreatedAt,
			// WebDAV 路径由后端算（路径段 = 挂载名，重名去重），前端只负责展示，避免两处规则各写一遍
			"davPath": davPathOf(p.ID),
		})
	}
	dto.OK(c, out)
}

func (h *AdminHandler) PolicyCreate(c *gin.Context) {
	var in policyIn
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	// 单用户私有部署：只支持挂载本机目录。云盘挂载（123云盘/阿里云盘/百度网盘/天翼云盘）
	// 与其授权流程已整体移除，这里显式拒绝，避免旧客户端或手写请求又建出一条打不开的策略。
	if t := strings.TrimSpace(in.Type); t != "" && t != "local" {
		dto.Fail(c, 400, "只支持挂载本机磁盘目录（云盘挂载已移除）：type 需为 local")
		return
	}
	in.Type = "local"
	u := middleware.CurrentUser(c)
	opts := "{}"
	if in.Options != nil {
		opts = marshalOpts(in.Options)
	}
	// 盘符留空 = 由后端生成唯一标识（前端「挂载文件夹」不再暴露该字段）
	letter := strings.TrimSpace(in.Letter)
	if letter == "" {
		letter = genPolicyLetter(in.Name)
	}
	p := model.Policy{Name: in.Name, Letter: letter, Type: in.Type, RootPath: in.RootPath, Options: opts, CreatedBy: u.ID}
	if in.RootPath == "" {
		dto.Fail(c, 400, "必须填写要挂载的本机目录")
		return
	}
	// 跨平台校验：必须为本机绝对路径；非 Windows 主机拒绝反斜杠，
	// 防止 Linux 上粘贴 E:\x 这类路径被当作相对目录名静默建到 CWD 下
	if runtime.GOOS != "windows" && strings.Contains(in.RootPath, "\\") {
		dto.Fail(c, 400, "根目录路径不合法：Linux/macOS 请使用 / 分隔的绝对路径")
		return
	}
	if !filepath.IsAbs(in.RootPath) {
		if runtime.GOOS != "windows" || !filepath.IsAbs(filepath.FromSlash(in.RootPath)) {
			dto.Fail(c, 400, "根目录必须是绝对路径（如 /data/storage）")
			return
		}
	}
	// 单用户私有部署：挂载的是「已有的本机文件夹」，不是预先规划的新存储目录。
	// 原实现只调 fscore.NewLocal → os.MkdirAll，会把打错的路径静默建成空目录，
	// 用户在网盘里看到一个空挂载点却不知道文件去哪了；这里要求目录必须已存在。
	st, err := os.Stat(in.RootPath)
	if err != nil {
		dto.Fail(c, 400, "目录不存在或无法访问："+err.Error())
		return
	}
	if !st.IsDir() {
		dto.Fail(c, 400, "所选路径不是文件夹")
		return
	}
	if _, err := fscore.NewLocal(in.RootPath); err != nil {
		dto.Fail(c, 400, "根目录不可用："+err.Error())
		return
	}
	if err := model.DB.Create(&p).Error; err != nil {
		// 唯一约束冲突（盘符重复）给出友好提示，避免把原始 SQLite 报错直接抛给前端
		if strings.Contains(err.Error(), "UNIQUE") || strings.Contains(err.Error(), "constraint") {
			dto.Fail(c, 400, "虚拟盘符已被占用，请更换")
			return
		}
		dto.Fail(c, 400, "创建存储策略失败："+err.Error())
		return
	}
	middleware.Audit(c, "admin", "挂载存储 "+p.Name)
	dto.OK(c, p)
}

// policyPatchIn 编辑挂载用的入参：与新建不同，这里 name / type 都是可选的。
// 早先直接复用 policyIn，而它把 name 和 type 都标了 binding:"required"，
// 于是「只改挂载目录」的请求（前端编辑框不再回传 type）会先被判「参数错误」，
// 校验分支根本进不去 —— 表现为改路径永远失败且看不出原因。
type policyPatchIn struct {
	Name     string            `json:"name"`
	Letter   string            `json:"letter"`
	Type     string            `json:"type"`
	RootPath string            `json:"rootPath"`
	Options  map[string]string `json:"options"`
}

func (h *AdminHandler) PolicyUpdate(c *gin.Context) {
	var p model.Policy
	if err := model.DB.First(&p, c.Param("id")).Error; err != nil {
		dto.Fail(c, 404, "策略不存在")
		return
	}
	var in policyPatchIn
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	// 云盘挂载已整体移除：即便旧客户端在编辑请求里带上云盘 type 也一并拒绝，
	// 不能让一条已经写成 local 的策略被改回打不开的类型。
	if t := strings.TrimSpace(in.Type); t != "" && t != "local" {
		dto.Fail(c, 400, "只支持挂载本机磁盘目录（云盘挂载已移除）：type 需为 local")
		return
	}
	// 名字留空 = 保持原值（编辑框允许只改目录，不能把挂载名清成空串）
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = p.Name
	}
	opts := p.Options
	if in.Options != nil {
		opts = marshalOpts(in.Options)
	}
	// 盘符留空 = 保持原值（它是 WebDAV 的路径段，前端编辑时不再展示，不能被清空）
	letter := strings.TrimSpace(in.Letter)
	if letter == "" {
		letter = p.Letter
	}
	// 改挂载目录同样要求是"已存在的本机绝对目录"（与新建同一套校验）：
	// 否则一个手滑的路径改下来，挂载点会变成空白页，却看不出问题在哪。
	rootPath := p.RootPath
	if in.RootPath != "" && in.RootPath != p.RootPath {
		np := strings.TrimSpace(in.RootPath)
		if !filepath.IsAbs(np) || (runtime.GOOS != "windows" && strings.Contains(np, "\\")) {
			dto.Fail(c, 400, "根目录必须是绝对路径")
			return
		}
		if st, err := os.Stat(np); err != nil {
			dto.Fail(c, 400, "目录不存在或无法访问："+err.Error())
			return
		} else if !st.IsDir() {
			dto.Fail(c, 400, "所选路径不是文件夹")
			return
		}
		if _, err := fscore.NewLocal(np); err != nil {
			dto.Fail(c, 400, "根目录不可用："+err.Error())
			return
		}
		rootPath = np
	}
	model.DB.Model(&p).Updates(map[string]interface{}{
		"name": name, "letter": letter, "root_path": rootPath, "options": opts,
		"status": orDefault(c.Query("status"), p.Status),
	})
	h.Site.Fs.Invalidate(p.ID)
	dto.OK(c, p)
}

func (h *AdminHandler) PolicyToggle(c *gin.Context) {
	var p model.Policy
	if err := model.DB.First(&p, c.Param("id")).Error; err != nil {
		dto.Fail(c, 404, "策略不存在")
		return
	}
	var in struct {
		Status string `json:"status" binding:"required"`
	}
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	model.DB.Model(&p).UpdateColumn("status", in.Status)
	h.Site.Fs.Invalidate(p.ID)
	dto.OK(c, p)
}

func (h *AdminHandler) PolicyDelete(c *gin.Context) {
	var p model.Policy
	if err := model.DB.First(&p, c.Param("id")).Error; err != nil {
		dto.Fail(c, 404, "策略不存在")
		return
	}
	var n int64
	model.DB.Model(&model.Share{}).Where("policy_id = ?", p.ID).Count(&n)
	if n > 0 {
		dto.Fail(c, 400, "存在关联分享，请先取消")
		return
	}
	model.DB.Delete(&p)
	h.Site.Fs.Invalidate(p.ID)
	middleware.Audit(c, "admin", "卸载存储 "+p.Name)
	dto.OK(c, nil)
}

// ---- 站点设置 ----

func (h *AdminHandler) SettingsGet(c *gin.Context) {
	dto.OK(c, GetSiteSettings())
}

func (h *AdminHandler) SettingsSet(c *gin.Context) {
	var in map[string]string
	if err := c.ShouldBindJSON(&in); err != nil {
		dto.Fail(c, 400, "参数错误")
		return
	}
	for k, v := range in {
		model.DB.Save(&model.SiteSetting{Key: k, Value: v})
	}
	// 运行时开关同步：offline_allow_private 影响离线下载的 SSRF 判定，存完立刻生效
	ApplySSRFSetting(GetSiteSettings())
	middleware.Audit(c, "admin", "更新站点设置")
	dto.OK(c, GetSiteSettings())
}

// ---- 审计日志 / 任务 ----

func (h *AdminHandler) LogList(c *gin.Context) {
	var in dto.PageIn
	_ = c.ShouldBindQuery(&in)
	offset, limit := in.Normalize()
	q := model.DB.Model(&model.AuditLog{})
	if in.Keyword != "" {
		q = q.Where("username LIKE ? OR action LIKE ? OR detail LIKE ?", "%"+in.Keyword+"%", "%"+in.Keyword+"%", "%"+in.Keyword+"%")
	}
	var total int64
	q.Count(&total)
	var items []model.AuditLog
	q.Order("id DESC").Offset(offset).Limit(limit).Find(&items)
	dto.OK(c, dto.PageOut{Total: total, Items: items})
}

// NotificationRecord 通知记录（含软清除），供管理员审计
type NotificationRecord struct {
	ID        uint      `json:"id"`
	Username  string    `json:"username"`
	Type      string    `json:"type"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Read      bool      `json:"read"`
	Cleared   bool      `json:"cleared"`
	CreatedAt time.Time `json:"createdAt"`
}

// NotificationList 全站通知记录（含已软清除的，审计用）；keyword 匹配类型/标题/内容/用户名
// 注意：不用 JOIN+Select(*, users.username) 扫进内嵌 struct——实测该写法会把 cleared 列误映射为 true。
// 改为直接查通知（扫描可靠，同 /api/notify），再批量取用户名在 Go 里拼装。
func (h *AdminHandler) NotificationList(c *gin.Context) {
	var in dto.PageIn
	_ = c.ShouldBindQuery(&in)
	offset, limit := in.Normalize()
	kw := in.Keyword
	q := model.DB.Model(&model.Notification{})
	if kw != "" {
		// 用户名匹配：先查出命中的用户 ID，再并入通知字段 LIKE 条件
		var uids []uint
		model.DB.Model(&model.User{}).Where("username LIKE ?", "%"+kw+"%").Pluck("id", &uids)
		if len(uids) == 0 {
			uids = []uint{0} // 空集占位，使 IN 恒假
		}
		q = q.Where("(type LIKE ? OR title LIKE ? OR content LIKE ? OR user_id IN ?)",
			"%"+kw+"%", "%"+kw+"%", "%"+kw+"%", uids)
	}
	if c.Query("cleared") == "true" {
		q = q.Where("cleared = ?", true)
	}
	var total int64
	q.Count(&total)
	var items []model.Notification
	q.Order("id DESC").Offset(offset).Limit(limit).Find(&items)
	// 批量取本页涉及的用户名
	uidSet := map[uint]struct{}{}
	for _, n := range items {
		uidSet[n.UserID] = struct{}{}
	}
	userMap := map[uint]string{}
	if len(uidSet) > 0 {
		uidSlice := make([]uint, 0, len(uidSet))
		for id := range uidSet {
			uidSlice = append(uidSlice, id)
		}
		var users []model.User
		model.DB.Where("id IN ?", uidSlice).Find(&users)
		for _, u := range users {
			userMap[u.ID] = u.Username
		}
	}
	out := make([]NotificationRecord, 0, len(items))
	for _, n := range items {
		out = append(out, NotificationRecord{
			ID: n.ID, Username: userMap[n.UserID], Type: n.Type, Title: n.Title,
			Content: n.Content, Read: n.Read, Cleared: n.Cleared, CreatedAt: n.CreatedAt,
		})
	}
	dto.OK(c, dto.PageOut{Total: total, Items: out})
}

// LogExport 审计日志 CSV 导出（流式，最多 20000 条；支持 keyword 过滤）
func (h *AdminHandler) LogExport(c *gin.Context) {
	keyword := c.Query("keyword")
	const maxRows = 20000
	q := model.DB.Model(&model.AuditLog{})
	if keyword != "" {
		q = q.Where("username LIKE ? OR action LIKE ? OR detail LIKE ?", "%"+keyword+"%", "%"+keyword+"%", "%"+keyword+"%")
	}
	c.Header("Content-Type", "text/csv; charset=utf-8")
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="audit-logs-%s.csv"`, time.Now().Format("20060102-150405")))
	// UTF-8 BOM：Excel 直接打开不乱码
	c.Writer.Write([]byte{0xEF, 0xBB, 0xBF})
	w := csv.NewWriter(c.Writer)
	_ = w.Write([]string{"时间", "用户", "操作", "详情", "IP"})
	w.Flush()
	const batchSize = 500
	var written int
	for offset := 0; offset < maxRows; offset += batchSize {
		var items []model.AuditLog
		q.Order("id DESC").Offset(offset).Limit(batchSize).Find(&items)
		if len(items) == 0 {
			break
		}
		for _, it := range items {
			_ = w.Write([]string{
				it.CreatedAt.Format("2006-01-02 15:04:05"),
				it.Username, it.Action, it.Detail, it.IP,
			})
			written++
		}
		w.Flush()
		if len(items) < batchSize {
			break
		}
	}
	w.Flush()
	middleware.Audit(c, "log_export", fmt.Sprintf("导出审计日志 %d 条（keyword=%q）", written, keyword))
}

func (h *AdminHandler) TaskList(c *gin.Context) {
	var items []model.Task
	model.DB.Order("id DESC").Limit(100).Find(&items)
	dto.OK(c, items)
}

// ShareAudit 全站分享审计
func (h *AdminHandler) ShareAudit(c *gin.Context) {
	var in dto.PageIn
	_ = c.ShouldBindQuery(&in)
	offset, limit := in.Normalize()
	q := model.DB.Model(&model.Share{})
	if in.Keyword != "" {
		q = q.Where("name LIKE ?", "%"+in.Keyword+"%")
	}
	var total int64
	q.Count(&total)
	var items []model.Share
	q.Order("id DESC").Offset(offset).Limit(limit).Find(&items)
	// 附上分享者用户名
	out := make([]gin.H, 0, len(items))
	for _, s := range items {
		var u model.User
		model.DB.Select("username", "nickname").First(&u, s.UserID)
		out = append(out, gin.H{
			"id": s.ID, "name": s.Name, "isDir": s.IsDir, "owner": u.Nickname, "ownerName": u.Username,
			"hasPassword": s.PasswordHash != "", "views": s.Views, "downloads": s.Downloads,
			"allowDownload": s.AllowDownload, "expiresAt": s.ExpiresAt, "createdAt": s.CreatedAt,
		})
	}
	dto.OK(c, dto.PageOut{Total: total, Items: out})
}

func (h *AdminHandler) ShareDelete(c *gin.Context) {
	var sh model.Share
	// 先取 token 以清理加密分享的密文目录（sharedata/<token>/）
	_ = model.DB.First(&sh, c.Param("id")).Error
	model.DB.Delete(&model.Share{}, c.Param("id"))
	if sh.Token != "" {
		_ = os.RemoveAll(filepath.Join(h.Site.Cfg.Sub("sharedata"), filepath.Base(sh.Token)))
	}
	middleware.Audit(c, "admin", "管理员取消分享 #"+c.Param("id"))
	dto.OK(c, nil)
}

// ---- 辅助 ----

// normalizeQuotaMB 归一化个人配额输入：<0 → -1（随组），0 → 0（不限量），>0 封顶 1PB
func normalizeQuotaMB(mb int64) int64 {
	if mb < 0 {
		return -1
	}
	if mb > 1024*1024 { // 1PB
		return 1024 * 1024
	}
	return mb
}

func orDefault(v, d string) string {
	if v == "" {
		return d
	}
	return v
}

func marshalOpts(m map[string]string) string {
	b, _ := jsonMarshal(m)
	return string(b)
}
