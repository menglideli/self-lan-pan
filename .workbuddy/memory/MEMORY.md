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
- **挂载 = 一条 `model.Policy`（`type=local`）**。字段 `Letter` 在 UI 上已完全不显示（用户要求"不要盘符"），但**不能删**——它仍是 WebDAV 的路径段（`webdav.go` 的 `resolve()` 按 `letter` 匹配 → `/dav/<letter>/`）、列表排序键、唯一索引。前端不传 `letter` 时由 `handler/localdir.go` 的 `genPolicyLetter()` 自动生成：ASCII 优先（≤4 位），纯中文名退化为 `M`、`M1`、`M2`…
- **本机目录浏览接口**：`GET /api/admin/fs/dirs`（`handler/localdir.go`，挂在 `/admin` 组 = 仅管理员）。不传 `path` 返回盘符/根列表，传 `path` 返回该目录下子目录。**按用户明确要求不设白名单，整机可翻**；两条底线是"必须绝对路径"+"必须已存在的目录"。它是文件管理器里「挂载文件夹」选择器的数据源。

## 构建与验证环境（踩过坑）
- 本机 Go 1.26.3，`go.mod` 要 1.27.0：必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**，会导致工具链校验失败）。`proxy.golang.org` 不可达。
- `server/internal/web/dist/.keep` 已就位，后端可单独 `go build ./...`（`go:embed all:dist` 需要该目录非空）。
- 本机 **Bash 工具的 PATH 完全失效**（`ls` / `dirname` / `cat` 全 command not found）。一律改用 PowerShell / Read / Grep / Glob。
- PowerShell 5.1 处理中文文件会写坏编码；批量改代码用 Node 脚本或 `[System.IO.File]` API。
- 端到端验证套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓"初始管理员密码" → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → 重启触发启动期任务 → 查磁盘。
- 杀进程**必须**用 `powershell -Command "Stop-Process -Name <exe> -Force"`；git bash 的 `taskkill //F //IM` 无效，会导致"端口占用→Fatalf 秒退"，让验证假通过。
- **后台/脱离进程会被会话清理杀掉**：`Start-Process`、detached spawn 起的服务在下次工具调用前就没了（浏览器报 `ERR_CONNECTION_REFUSED`）。正确做法是把「起服务 + 用服务 + 收服务」收进**同一个前台进程**（验证脚本自包含）。
- `vite build` **不做类型检查**；类型错误要另跑 `vue-tsc`（仓库里还有 8 条历史遗留错误，判"是否新增"必须 `git show HEAD:<file>` 比对基线）。
- **两种错误报告形态别混**：业务错误走 `dto.Fail(c, 400, msg)` = **HTTP 200 + body `{code:400}`**；只有"路由不存在/中间件拦截"才真的改 HTTP 状态码（`dto.FailHTTP`）。写断言时业务错误查 `r.json.code`，别查 `r.status`。
- **写进 `evalOr` 模板字符串里的正则要双反斜杠，写在 Node 侧的要单反斜杠**——多转义一层会让正则永远匹配不上，断言"恒为真"地空转却一直 PASS。这类空转只能靠反向变异验证抓出来。
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
9. **`fscore.NewLocal` 内部是 `os.MkdirAll`** —— 它对不存在的路径会**静默创建目录**然后成功返回。这曾让"挂载一个打错的路径"返回 code 0 并在磁盘上留下空目录（已实测复现）。现在 `PolicyCreate` 对 local 类型加了 `os.Stat` 预检（目录必须已存在）。**改这里时别把预检删了**，也别指望 `NewLocal` 自己会拒绝。
10. **改 `Policy` 相关代码时注意 `letter` 的隐式约束**：空字符串在 SQLite 里是一个真实值，多个空 `letter` 会撞唯一索引。任何"清空 letter"的路径都必须先经过 `genPolicyLetter()` 或回退原值。

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
- 批次 5 ✅ `687325b`：文件管理器根视图改「已挂载的文件夹」（文件夹图标 + 名称 + 本机目录 + 已用空间），侧栏 / 标签 / 窗口标题 / 面包屑全部去掉盘符；全局搜索徽标改为挂载点名；根视图右键新增「挂载文件夹…/编辑此挂载/卸载」。
- 批次 6 ✅ `687325b`：新增「挂载文件夹」——后端 `GET /api/admin/fs/dirs` 列本机目录（仅管理员、无白名单）、`letter` 自动生成、本地挂载要求目录已存在；前端 Explorer 内嵌浏览式目录选择器（面包屑 / 上一级 / 点击进入），AdminConsole 去掉「虚拟盘符」输入与盘符列。
- 验证：`smoke.mjs` **60/60**、`ui.mjs` **22/22**、反向变异验证通过（把 `(letter)` 加回标签 → ui 如期 FAIL）。
- 待做：**批次 7**（全链路 `build.bat` 构建 + 冒烟 + 重写 README）。可选：手机端前端（暂缓，先 WebDAV）。
- 收尾动作：批次 7 之前记得检查 `docs/单用户私有化改造计划.md` 是否还有"待做"标记；README 必须重写（当前描述的功能面大于实际，见上文）。
