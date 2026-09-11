# self-lan-pan（CloudPan）项目长期备忘

## 定位与约定
- 自托管私有网盘：Go(Gin + GORM + glebarez/sqlite) 后端 + Vue 3 + TS + Vite + Pinia 前端；`go:embed all:dist` 打进单个 exe（`server/main.go` 显式监听器，端口 18322）。
- 环境变量：`CP_PORT` / `CP_DATA` / `CP_PUBLIC_URL` / `CP_TRUSTED_PROXIES`。数据目录 `server/data`（DB / secret.key / 回收站 / 上传临时区 / 缩略图）。
- 构建：`build.bat` = `npm run build` → 拷 `web/dist` → `server/internal/web/dist` → `go build -o cloudpan.exe`。Go 1.27 在 `C:\Program Files\Go`；Node 用 workbuddy 托管版。
- git：`main` + `origin = github.com/menglideli/self-lan-pan`。`.gitignore` 已忽略 `server/data`、`web/dist`、`server/internal/web/dist/*`（仅保留 `.keep`）、`server/cloudpan.exe`、`C/`、`D/`。
- **本项目用户画像**：单用户私有部署（只服务内网、只有管理员一个账号）。新会话不要默认它是多用户产品——多用户那套是上游遗留，正在被裁剪。

## 扩展点（改动必看）
- **功能开关四处联动**：`server/internal/apps/manifest.go` 的 `Manifest` → 路由上的 `middleware.AppGate(key)` → 前端 `stores/appstate.ts.isAvailable()` + `stores/apps.ts` 的 `APPS[].feature` → `themes/registry.ts` 的 `APP_COMPONENTS`。漏一处就出现"入口没了接口还在"或"接口 403 图标还在"。
- ⚠️ **`model.AppEnabled` 对清单外的 key 返回 `true`**（源码注释写的是"向前兼容清单外功能"）。所以「从 Manifest 删 key、但留着 `AppGate(key)` 路由」= 门控静默失效、接口继续可用。删功能必须同时删路由，二者不可分开。
- 路由唯一装配点：`server/internal/handler/router.go`。
- 前端主题插件化：`themes/<id>/index.ts` 导出 `ThemeDef` → `themes/registry.ts` 登记；主题 CSS 由 registry 静态 import，删目录即自动摘除。`ThemeId` 联合类型在 `themes/types.ts`（现只剩 `'win12'`）。
- 存储抽象：`fscore` Driver 注册表（`main.go` 注册 local/pan123/aliyun/baidu/tianyi）。**用户子目录隔离已于批次 1 关闭**，本地盘直接暴露挂载根的真实内容。

## 构建与验证环境（踩过坑）
- 本机 Go 1.26.3，`go.mod` 要 1.27.0：必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**，会导致工具链校验失败）。`proxy.golang.org` 不可达。
- `server/internal/web/dist/.keep` 已就位，后端可单独 `go build ./...`（`go:embed all:dist` 需要该目录非空）。
- 本机 **Bash 工具的 PATH 完全失效**（`ls` / `dirname` / `cat` 全 command not found）。一律改用 PowerShell / Read / Grep / Glob。
- PowerShell 5.1 处理中文文件会写坏编码；批量改代码用 Node 脚本或 `[System.IO.File]` API。
- 端到端验证套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓"初始管理员密码" → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → 重启触发启动期任务 → 查磁盘。
- 杀进程**必须**用 `powershell -Command "Stop-Process -Name <exe> -Force"`；git bash 的 `taskkill //F //IM` 无效，会导致"端口占用→Fatalf 秒退"，让验证假通过。
- **后台/脱离进程会被会话清理杀掉**：`Start-Process`、detached spawn 起的服务在下次工具调用前就没了（浏览器报 `ERR_CONNECTION_REFUSED`）。正确做法是把「起服务 + 用服务 + 收服务」收进**同一个前台进程**（验证脚本自包含）。
- `vite build` **不做类型检查**；类型错误要另跑 `vue-tsc`（仓库里有 11 条历史遗留错误，判"是否新增"必须 `git show HEAD:<file>` 比对基线）。
- **同一文件并行下发多个 Edit 会丢改动**（工具报成功，但部分写入被并发读-改-写覆盖）。同一文件的多次编辑必须串行下发，改完用 Grep/Read 复核。

## 已知高危耦合（改动前必读）
1. ~~本地盘按用户子目录隔离~~ —— **已于批次 1 关闭**（`main.go`、`handler/webdav.go` 的 join 与 `fscore.UserDirOf` 均已删除）。
2. ~~游客 24h 清理任务会物理删文件~~ —— **已于批次 1 删除**（`sweepGuestWorkspace` / `sweepGuestDir` 已移除）。
   ⚠️ 教训留档：这两条曾构成"一小时内丢数据"的组合拳。**今后不要重新引入任何"按 mtime 自动物理删除挂载目录"的清理逻辑。**
3. `C/`、`D/` 曾被放在仓库目录下当挂载根 —— 属运行数据，`.gitignore` 已挡，别再提交。
4. ~~`middleware/guest.go`、`model.IsGuestUser`、`model.GuestUsername`~~ —— **已于批次 2 删除**，游客体系整体移除。
5. `dlink.go`（直链签名）复用了 `office.go` 里定义的 `officeTarget` 类型当载荷。删 Office 时必须先把这个类型迁走或内联，否则直链功能编译不过。
6. ~~`middleware/security.go` 的 CSP 按 ONLYOFFICE origin 动态放行~~ —— **已于批次 4c 改为静态 CSP**（`dsOrigins()`/`buildCSP()` 已删）。现在**不再放行任何第三方 origin**，是安全收益；今后接第三方服务请按最小必要精确加 origin，别退回通配。
7. **公共代码会"借住"在功能文件里**：`office.go` 除 ONLYOFFICE 外还定义了 `CloudAuth`（云盘 OAuth 授权，**保留功能**）和 `publicBaseOf()` / `parseUintQuery()`。删这类文件前**必须先列出它的顶层声明**，把不属于该功能的符号先迁走（本次落在 `handler/cloudauth.go`），否则编译直接崩。
8. **`Share.AllowEdit` 已删除**（`office.go` 是它唯一的读取方）。DB 的 `allow_edit` 列保留未迁移，无害；别再在前端加回「允许在线编辑」开关。

## 已锁定的改造决策（2026-09-11 用户拍板）
登录页只留密码框；文件管理器不要盘符、根视图直接列挂载点；挂载入口放文件管理器内（后端需新增目录浏览接口）；手机先走 WebDAV；公开分享暂留；**用户组 / 权限模型彻底删除、权限写死为管理员全开**（配额不限；`RecycleRetentionDays: 0` = 回收站永久保留，系统不再有任何"按时间自动物理删用户文件"的行为）。

## 与上游 README 的偏差（以代码为准）
README 描述的功能面大于实际裁剪目标。**核对功能时不要拿 README 当事实**——批次 4 落地后 README 必须重写。

## 当前进度
- 2026-09-11：项目审计，产出 `docs/单用户私有化改造计划.md`（v2 定稿，7 个批次）。
- 批次 1 ✅：删游客清理任务、关用户目录隔离、移除游客登录。
- 批次 2 ✅ `c410cfd`（后端）+ `cecceea`（前端）：删注册 / 用户管理 / 用户组 / 站内共享，权限写死为 `model.AdminPerms()`。
- 批次 3 ✅ `cecceea`：只留 win12 主题，macos / deepin 整包删除。
- 批次 4 ✅ `4e3da53`（4a/4b）+ `3992d50`（4c）：删掉 7 个应用 —— 计算器、壁纸中心、图库、网络测速、内置浏览器、终端（含 sshfs/SFTP 与 `apps/terminal/` 子树）、在线 Office（含本地 docx/xlsx/pptx 预览，用户拍板"全删"）。同时删 `Share.AllowEdit` 死字段、CSP 简化成静态策略。
- 桌面实测只剩：此电脑 / 回收站 / 记事本 / 任务中心 / 应用中心 / 设置 / 管理控制台（媒体中心是 installable，装后才出现）。apps 清单只剩 10 个 key。
- 待做：批次 5（文件管理器根视图列挂载点、无盘符）、批次 6（挂载菜单：目录浏览接口 + 文件夹选择器）、批次 7（全链路构建 + 冒烟 + 重写 README）。
