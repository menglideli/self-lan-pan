# self-lan-pan（CloudPan）项目长期备忘

## 定位与约定
- 自托管私有网盘：Go(Gin + GORM + glebarez/sqlite) 后端 + Vue 3 + TS + Vite + Pinia 前端；`go:embed all:dist` 打进单个 exe（`server/main.go` 显式监听器，端口 18322）。
- 环境变量：`CP_PORT` / `CP_DATA` / `CP_PUBLIC_URL` / `CP_TRUSTED_PROXIES`。数据目录 `server/data`（DB / secret.key / 回收站 / 上传临时区 / 缩略图）。
- 构建：`build.bat` = `npm run build` → 拷 `web/dist` → `server/internal/web/dist` → `go build -o cloudpan.exe`。Go 1.27 在 `C:\Program Files\Go`；Node 用 workbuddy 托管版。
- git：`main` + `origin = github.com/menglideli/self-lan-pan`。`.gitignore` 已忽略 `server/data`、`web/dist`、`server/cloudpan.exe`、`C/`、`D/`。
- **本项目用户画像**：单用户私有部署（只服务内网、只有管理员一个账号）。新会话不要默认它是多用户产品——多用户那套是上游遗留，正在被裁剪。

## 扩展点（改动必看）
- **功能开关三处联动**：`server/internal/apps/manifest.go` 的 `Manifest` → 路由上的 `middleware.AppGate(key)` → 前端 `stores/appstate.ts.isAvailable()` + `stores/apps.ts` 的 `APPS[].feature` + `themes/registry.ts` 的 `APP_COMPONENTS`。删功能漏一处就会出现"入口没了接口还在"或"接口 403 图标还在"。
- 路由唯一装配点：`server/internal/handler/router.go`。
- 前端主题插件化：`themes/<id>/index.ts` 导出 `ThemeDef` → `themes/registry.ts` 登记；主题 CSS 由 registry 静态 import，删目录即自动摘除。`ThemeId` 联合类型在 `themes/types.ts`。
- 存储抽象：`fscore` Driver 注册表（`main.go` 注册 local/pan123/aliyun/baidu/tianyi）+ 多用户子目录隔离。

## 构建与验证环境（踩过坑）
- 本机 Go 1.26.3，`go.mod` 要 1.27.0：必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**，会导致工具链校验失败）。`proxy.golang.org` 不可达。
- `server/internal/web/dist/.keep` 已就位，后端可单独 `go build ./...`（`go:embed all:dist` 需要该目录非空）。
- 端到端验证套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓"初始管理员密码" → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → 重启触发启动期任务 → 查磁盘。
- 杀进程**必须**用 `powershell -Command "Stop-Process -Name <exe> -Force"`；git bash 的 `taskkill //F //IM` 无效，会导致"端口占用→Fatalf 秒退"，让验证假通过。

## 已知高危耦合（改动前必读）
1. ~~本地盘按用户子目录隔离~~ —— **已于批次 1 关闭**（`main.go`、`handler/webdav.go` 的 join 与 `fscore.UserDirOf` 均已删除）。
2. ~~游客 24h 清理任务会物理删文件~~ —— **已于批次 1 删除**（`sweepGuestWorkspace`/`sweepGuestDir` 已移除）。
   ⚠️ 教训留档：这两条曾构成"一小时内丢数据"的组合拳。**今后不要重新引入任何"按 mtime 自动物理删除挂载目录"的清理逻辑。**
3. `C/`、`D/` 曾被放在仓库目录下当挂载根 —— 属运行数据，`.gitignore` 已挡，别再提交。
4. `middleware/guest.go`、`model.IsGuestUser`、`model.GuestUsername` 仍在（批次 1 只摘了登录入口），随批次 2 的游客体系一并删除。

## 已锁定的改造决策（2026-09-11 用户拍板）
登录页只留密码框；文件管理器不要盘符、根视图直接列挂载点；挂载入口放文件管理器内（后端需新增目录浏览接口）；手机先走 WebDAV；公开分享暂留；**用户组/权限模型彻底删除、权限写死为管理员全开**（配额不限、回收站 30 天）。

## 与上游 README 的偏差（以代码为准）
README 描述的功能面大于实际裁剪目标：它写"16 个内置应用"但 `APPS` 有 20 条；公开分享/站内共享/终端/浏览器/Office/游客体系这些正在被逐项裁掉。**核对功能时不要拿 README 当事实。**

## 当前进度
- 2026-09-11：完成项目审计，产出 `docs/单用户私有化改造计划.md`（v2 定稿，7 个批次）。
- 2026-09-11 批次 1 **已完成并实测通过**：删游客清理任务、关用户目录隔离、移除游客登录。改前危害已复现、改后 4 个测试文件全部存活、`/auth/guest` 返回 404、`fs/list` 直接列出挂载根真实内容。
- 下一步：批次 2（删注册/用户管理/用户组/站内共享 + 权限写死）。
