package main

import (
	"log"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	"cloudpan/internal/config"
	"cloudpan/internal/fscore"
	"cloudpan/internal/handler"
	"cloudpan/internal/middleware"
	"cloudpan/internal/model"
	"cloudpan/internal/web"
)

func fsService(cfg *config.Config) *fscore.Service {
	// 存储驱动：单用户私有部署只支持「本机磁盘目录」一种（云盘驱动已整体移除）。
	// 本地盘也不再按用户拼子目录，策略的挂载根就是真实目录。
	fscore.RegisterDriver("local", func(p *model.Policy, _ *model.User) (fscore.Driver, error) {
		return fscore.NewLocal(p.RootPath)
	})
	return fscore.NewService(cfg.Sub("upload_tmp"), cfg.Sub("recycle"), cfg.Sub("thumbs"), cfg.Sub("ziptmp"))
}

// warnLegacyCloudPolicies 启动时点名"更早版本留下的云盘策略"：
// 这类策略的驱动已不再注册，在网盘里只会看到一个打不开的挂载点，
// 启动日志说清楚原因，用户才知道该去管理台卸载它。
func warnLegacyCloudPolicies() {
	var rows []model.Policy
	model.DB.Where("type <> ? AND type <> ''", "local").Find(&rows)
	for _, p := range rows {
		log.Printf("[CloudPan] 存储策略 #%d %q 类型为 %s：该类型已不再支持，请在管理台「存储策略」里卸载", p.ID, p.Name, p.Type)
	}
}

func main() {
	cfg := config.Load()
	model.InitDB(cfg.DataDir)
	model.LoadAppCache()         // 应用中心：功能开关内存缓存
	warnLegacyCloudPolicies()    // 旧版本遗留的云盘策略：启动时点名，免得用户对着打不开的挂载点发呆
	handler.StartSystemMonitor() // NAS 系统监控采样器（仪表盘数据源）
	gin.SetMode(gin.ReleaseMode)

	r := gin.New()
	// ClientIP 信任边界：默认不信任任何 X-Forwarded-For（直接取 TCP 对端地址），
	// 防止攻击者伪造 XFF 头绕过登录限速/锁定等按 IP 的防护。
	// 部署在反向代理之后时，用环境变量 CP_TRUSTED_PROXIES 显式声明代理网段（逗号分隔 CIDR/IP），
	// 此时 gin 才从 XFF 链中取真实客户端 IP。
	if tp := strings.TrimSpace(os.Getenv("CP_TRUSTED_PROXIES")); tp != "" {
		var proxies []string
		for _, s := range strings.Split(tp, ",") {
			if s = strings.TrimSpace(s); s != "" {
				proxies = append(proxies, s)
			}
		}
		if len(proxies) > 0 {
			if err := r.SetTrustedProxies(proxies); err != nil {
				log.Printf("CP_TRUSTED_PROXIES 配置无效，回退为不信任任何代理: %v", err)
			}
		}
	} else {
		// 显式置空 = 不信任任何代理
		_ = r.SetTrustedProxies(nil)
	}
	// 访问日志打码敏感查询参数：t=JWT、token=直链签名、st=分享提取码、pt=浏览器代理票据
	r.Use(middleware.AccessLogger("t", "token", "st", "pt"), gin.Recovery(), middleware.SecurityHeaders())
	r.MaxMultipartMemory = 64 << 20

	site := &handler.SiteHandler{Cfg: cfg, Fs: fsService(cfg), ZipTmp: cfg.Sub("ziptmp")}
	handler.InitTaskPool(site.Fs, cfg.Sub("bt_tmp"), cfg.Sub("ziptmp"))
	// 站点设置里的运行时开关同步一次（如"离线下载是否允许访问内网地址"）
	handler.ApplySSRFSetting(handler.GetSiteSettings())
	handler.Setup(r, cfg, site)
	handler.RegisterDav(r, site.Fs)
	web.Register(r)

	// 显式监听器（而非 r.Run）：便于启动失败时给出明确日志，也留出平滑重启的余地
	ln, err := net.Listen("tcp", ":"+cfg.Port)
	if err != nil {
		log.Fatalf("监听 :%s 失败: %v", cfg.Port, err)
	}
	log.Printf("CloudPan 启动: http://localhost:%s  数据目录: %s", cfg.Port, cfg.DataDir)
	// 多网卡机器上把全部可用地址都打出来：手机/别的电脑该连哪个一眼可见，不用自己猜
	for _, a := range handler.LocalAddresses(cfg.Port) {
		log.Printf("  可访问地址: %-28s (%s)", a.URL, a.Iface)
	}
	if err := (&http.Server{Handler: r}).Serve(ln); err != nil {
		log.Fatalf("启动失败: %v", err)
	}
}
