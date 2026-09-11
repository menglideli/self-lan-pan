package handler

import (
	"github.com/gin-gonic/gin"

	"cloudpan/internal/config"
	"cloudpan/internal/middleware"
)

// Setup 装配全部路由
func Setup(r *gin.Engine, cfg *config.Config, site *SiteHandler) {
	api := r.Group("/api")

	// 公开
	api.GET("/site/public", site.PublicInfo)
	auth := &AuthHandler{Secret: cfg.Secret}
	// 登录/注册按来源 IP 限速，防密码暴破与批量注册
	loginLimiter := middleware.NewIPRateLimiter(10, 5) // 10 次/分钟，突发 5
	// 刷新令牌续期：401 静默续期入口，独立限流防刷
	refreshLimiter := middleware.NewIPRateLimiter(30, 10) // 30 次/分钟，突发 10
	api.POST("/auth/login", middleware.RateLimit(loginLimiter), auth.Login)
	// 单用户私有部署：不提供注册与游客登录入口（原 /auth/register、/auth/guest 已移除）
	api.POST("/auth/refresh", middleware.RateLimit(refreshLimiter), auth.Refresh)

	// 公开分享（受「公开分享」功能门控）
	sh := &ShareHandler{Site: site, Secret: cfg.Secret}
	// 分享提取码验证按 IP 限速：带密码的分享链接可被暴力试码
	verifyLimiter := middleware.NewIPRateLimiter(20, 8) // 20 次/分钟，突发 8
	sg := api.Group("/s/:token", middleware.AppGate("share"))
	{
		sg.GET("/info", sh.Info)
		sg.POST("/verify", middleware.RateLimit(verifyLimiter), sh.Verify)
		sg.GET("/list", sh.List)
		sg.GET("/download", sh.Download)
		sg.GET("/raw", sh.Raw)
	}

	// 需登录
	ug := api.Group("", middleware.Auth(cfg.Secret))
	{
		ug.GET("/auth/me", auth.Me)
		ug.PUT("/users/me", auth.UpdateMe)
		ug.PUT("/users/me/password", auth.ChangePassword)
		ug.PUT("/users/me/webdav-password", auth.SetWebdavPassword)

		ug.GET("/policies", site.Policies)

		ug.GET("/fs/list", site.List)
		ug.POST("/fs/mkdir", site.Mkdir)
		ug.POST("/fs/rename", site.Rename)
		ug.POST("/fs/move", site.Move)
		ug.POST("/fs/copy", site.Copy)
		ug.POST("/fs/cross-copy", site.CrossCopy)
		ug.POST("/fs/cross-move", site.CrossMove)
		ug.POST("/fs/delete", site.Delete)
		ug.GET("/fs/text", site.ReadText)
		ug.POST("/fs/text", site.WriteText)
		ug.GET("/fs/search", site.Search)
		ug.GET("/fs/global-search", middleware.AppGate("global_search"), site.GlobalSearch)
		ug.GET("/fs/properties", site.Properties)
		ug.GET("/fs/raw", site.Raw)
		ug.GET("/fs/download", site.Download)
		ug.POST("/fs/archive", site.Archive)

		// 文件版本管理（受「版本管理」功能门控）
		ug.GET("/fileversions", middleware.AppGate("version"), site.FileVersions)
		ug.GET("/fileversions/download", middleware.AppGate("version"), site.FileVersionDownload)
		ug.POST("/fileversions/restore", middleware.AppGate("version"), site.FileVersionRestore)

		up := &UploadHandler{Site: site}
		ug.POST("/upload/init", up.Init)
		ug.PUT("/upload/chunk/:sid/:idx", up.Chunk)
		ug.POST("/upload/complete", up.Complete)
		ug.DELETE("/upload/:sid", up.Abort)
		ug.GET("/upload/:sid/status", up.Status)

		ug.GET("/recycle", site.RecycleList)
		ug.POST("/recycle/restore", site.RecycleRestore)
		ug.POST("/recycle/purge", site.RecyclePurge)

		ug.GET("/stars", site.StarList)
		ug.POST("/stars", site.StarAdd)
		ug.DELETE("/stars", site.StarRemove)

		// 用户设置 KV（播放列表/观看进度等）
		ug.GET("/settings", site.UserSettingsGet)
		ug.PUT("/settings", site.UserSettingsSet)

		ug.GET("/shares", middleware.AppGate("share"), sh.Mine)
		ug.POST("/shares", middleware.AppGate("share"), sh.Create)
		ug.DELETE("/shares/:id", middleware.AppGate("share"), sh.Cancel)
		// 端到端加密分享：属主上传本地加密后的密文（body = 密文字节流）
		ug.POST("/shares/:id/encrypt-file", middleware.AppGate("share"), sh.EncryptFile)
		// 转存：公开分享内容一键保存到自己账号（登录态）
		ug.POST("/s/:token/save", sh.SaveToDrive)

		// 云盘授权（管理员专属：涉及云盘凭据绑定/token 交换，普通用户无权限操作存储策略）
		ca := &CloudAuth{Site: site}
		ug.GET("/cloud/auth-url", middleware.AdminOnly(), ca.AuthURL)
		ug.POST("/cloud/exchange", middleware.AdminOnly(), ca.Exchange)
		ug.GET("/cloud/status", middleware.AdminOnly(), ca.Status)

		// 离线下载（HTTP 或 BT 任一启用即放行）
		off := &OfflineHandler{}
		ug.POST("/offline", middleware.AppGateAny("offline_http", "bt"), off.Create)
		ug.GET("/offline", off.List)
		ug.DELETE("/offline/:id", off.Cancel)

		// 站内通知（每用户隔离，受「站内通知」功能门控）
		notify := &NotifyHandler{}
		ug.GET("/notify", middleware.AppGate("notify"), notify.List)
		ug.GET("/notify/unread", middleware.AppGate("notify"), notify.Unread)
		ug.GET("/notify/stream", middleware.AppGate("notify"), notify.Stream)
		ug.POST("/notify/read", middleware.AppGate("notify"), notify.Read)
		ug.POST("/notify/clear", middleware.AppGate("notify"), notify.Clear)

		// 站内用户共享（usershare）已删除：单用户私有部署没有"他人"可共享

		// 系统功能清单（应用中心数据源）
		ug.GET("/apps", site.AppList)

	}

	// 云盘 OAuth 回调（公开路由：厂商把用户浏览器重定向回这里，此时浏览器未必登录本系统；
	// 安全靠 HMAC 签名 state 而非登录态——state 绑定策略 ID/类型/回调地址，30 分钟过期）
	ca2 := &CloudAuth{Site: site}
	api.GET("/cloud/callback", ca2.Callback)

	// 直链提取：签发需登录，访问免登录
	dlh := &DLHandler{Site: site}
	ug2 := api.Group("", middleware.Auth(cfg.Secret))
	ug2.GET("/fs/dlink", dlh.Create)
	api.GET("/dl", dlh.Serve)

	// 管理端
	ag := api.Group("/admin", middleware.Auth(cfg.Secret), middleware.AdminOnly())
	{
		ad := &AdminHandler{Site: site}
		ag.GET("/dashboard", ad.Dashboard)

		// 系统资源监控（受「系统监控」功能门控）
		ag.GET("/system", middleware.AppGate("system_monitor"), ad.SystemInfo)

		// 单用户私有部署：用户管理与用户组管理端点已移除（只有管理员一个账号）
		ag.GET("/policies", ad.PolicyList)
		// 本机目录浏览：「挂载文件夹」的目录选择器数据源（仅管理员；按用户要求不设白名单）
		ag.GET("/fs/dirs", ad.BrowseDirs)
		ag.POST("/policies", ad.PolicyCreate)
		ag.PUT("/policies/:id", ad.PolicyUpdate)
		ag.PUT("/policies/:id/status", ad.PolicyToggle)
		ag.DELETE("/policies/:id", ad.PolicyDelete)

		ag.GET("/settings", ad.SettingsGet)
		ag.PUT("/settings", ad.SettingsSet)

		// 单用户私有部署：系统自更新端点已删除（二进制由部署者自行替换）

		ag.GET("/logs", ad.LogList)
		ag.GET("/logs/export", ad.LogExport)
		ag.GET("/notifications", ad.NotificationList)
		ag.GET("/tasks", ad.TaskList)
		ag.GET("/shares", ad.ShareAudit)
		ag.DELETE("/shares/:id", ad.ShareDelete)

		ag.POST("/apps/:key/toggle", ad.AppToggle)
	}
}
