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
- **挂载 = 一条 `model.Policy`（`type=local`）**。字段 `Letter` 在 UI 上已完全不显示（用户要求"不要盘符"），但**不能删**——它仍是 WebDAV 的**兼容别名路径段**（`/dav/<letter>/`）、列表排序键、唯一索引。前端不传 `letter` 时由 `handler/localdir.go` 的 `genPolicyLetter()` 自动生成：ASCII 优先（≤4 位），纯中文名退化为 `M`、`M1`、`M2`…
- **WebDAV = 统一入口 `/dav/`（批次 9 定稿）**：`/dav/` 是**虚拟根**，PROPFIND 列出全部本机挂载（`type='local'` 且未 disabled；**云盘不纳入**），写操作一律拒绝；`/dav/<挂载名>/…` 是单挂载正式路径段；`/dav/<盘符>/…` 是兼容别名。**路径段命名规则只在后端实现**（`webdav.go` 的 `davSeg()` / `davPathOf()`），经 `PolicyList` 的 **`davPath` 字段**下发给前端展示——**改规则只改后端一处**，别在前端重写一套。
- **WebDAV 认证链路（用户问"手机怎么配"时的事实答案）**：账号 = 网页用户名（`admin`）；密码 = **WebDAV 独立密码**（设置页设，`PUT /api/users/me/webdav-password` → `user.webdav_password_hash`），**不是登录密码**，未设时 401「请在设置中先行设置」；地址 `http://<内网IP>:18322/dav/`；启用开关 = `SiteSetting.webdav_enabled`（默认 true，管理台可关）+ `AppEnabled("webdav")`；失败锁定与网页登录**共用同一张表**（IP+用户名，5 次锁 15 分钟）。
- **挂载根 = 策略的 `RootPath` 本身**（不额外套一层）：所以 `/dav/<挂载名>/` 列出的是该目录的内容，文件管理器根视图也是直接进这一层。写断言/建 fixture 时别再以为外面还包一层目录名。
- **系统自更新已整体删除**（批次 9）：`handler/update.go` 整文件 + 4 条 `/api/admin/update/*` 路由 + `model.UpdateLog` + `main.go` 的自重启路径都不在了。二进制由部署者手动替换。旧库残留 `update_logs` 表无害。
- **本机目录浏览接口**：`GET /api/admin/fs/dirs`（`handler/localdir.go`，挂在 `/admin` 组 = 仅管理员）。不传 `path` 返回盘符/根列表，传 `path` 返回该目录下子目录。**按用户明确要求不设白名单，整机可翻**；两条底线是"必须绝对路径"+"必须已存在的目录"。它是文件管理器里「挂载文件夹」选择器的数据源。
- **挂载列表变化走 `cp-policies-changed` 跨窗口广播**：管理台 `loadAll()` 对挂载列表算指纹（id/名称），变了才 `window.dispatchEvent`；`Explorer.vue` 监听后重拉 `fsApi.policies()`，当前挂载被卸载则退回根视图。**别复用 `cp-refresh-explorer`**——那个只刷"当前目录的文件列表"，不重拉挂载列表（挂载根视图不会更新）。
- **Explorer 的导航历史是 `(policyId, path)` 二元组**（`policyId === null` = "此电脑"根视图，这是"后退能退回根视图"的唯一表达方式）。`pushHistory()` 压的是**来处**，因此必须在改 `tab.policyId` / `tab.path` **之前**调用；反过来写就会出现"后退按钮可点但点了没反应"。

- **离线下载有三条链路（批次 10）**：`offline`（HTTP 直链）/ `bt`（`.torrent` + `magnet:`）/ `m3u8`（HLS）。类型判定在 `handler/bt.go` 的 `sniffOfflineKind()`（**m3u8 分支必须在 http 之前**，否则带 `.m3u8` 后缀的地址会落成普通直链）+ `OfflineHandler.Create` 的 `taskType` switch；任务分发在 `tasks.go` 的 `run()` switch。**无后缀的清单地址靠运行时嗅探**：`runOffline` 里 `bufio.Peek(1024)` + `looksLikeM3U8Body()` 命中后 `UpdateColumn("type","m3u8")` 并就地转 `runM3U8Task()`。
- **m3u8 下载器**：`server/internal/handler/m3u8.go`（纯 Go，零第三方依赖，不需要 ffmpeg）。协议面：master 按 `BANDWIDTH` 选最高码率、`#EXT-X-BYTERANGE`（省略 offset 紧接上一段）、`#EXT-X-MAP`（合并时先写）、`AES-128`（**未给 IV 时按媒体序号推导 16 字节大端 IV**，`ivFromSeq()`）。所有取数走 `ssrfHTTP`，保住 SSRF 三层防护。默认 8 并发（上限 32）/ 20000 分片 / 20GB。
- **多网卡地址枚举**：`server/internal/handler/netaddr.go` 的 `LocalAddresses(port)` → `GET /api/system/addresses`（在 `router.go` 的 `ug` 组）。每条带网卡名 + `kind`（`lan`/`virtual`，`classifyIface()` 按网卡名识别 VMware/Hyper-V/WSL/Docker/ZeroTier/Tailscale/WireGuard 等）。前端 `web/src/utils/lanaddr.ts`（模块级缓存 + single-flight）+ `web/src/components/AddressPicker.vue`。**`AddressPicker` 用的是自绘 `position:fixed` 遮罩** —— 复用的 `.dialog-mask` 是 `position:absolute`，在这里会漂。
- **内网地址放行开关**：`SiteSetting.offline_allow_private`（默认关）→ `ssrf.go` 的 `ssrfAllowPrivate atomic.Bool`（`SetSSRFAllowPrivate()` 用 **`.Store()`**，`atomic.Bool` 没有 `.Set`）。保存后由 `AdminHandler.SettingsSet` 与 `main.go` 启动时各调一次 `ApplySSRFSetting()`。**加了新站点开关要记得同时接上"启动时应用"那条路径。**

## 构建与验证环境（踩过坑）
- 本机 Go 1.26.3，`go.mod` 要 1.27.0：必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**，会导致工具链校验失败）。`proxy.golang.org` 不可达。
- `server/internal/web/dist/.keep` 已就位，后端可单独 `go build ./...`（`go:embed all:dist` 需要该目录非空）。`build.bat` 会先 `rmdir` 整个 dist 再 xcopy，**所以它必须自己重建 `.keep`**（批次 7 补上；`build.sh` 本就有 `touch .keep`）。
- 本机 **Bash 工具的 PATH 完全失效**（`ls` / `dirname` / `cat` 全 command not found）。一律改用 PowerShell / Read / Grep / Glob。
- PowerShell 5.1 处理中文文件会写坏编码；批量改代码用 Node 脚本或 `[System.IO.File]` API。
- 端到端验证套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓"初始管理员密码" → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → 重启触发启动期任务 → 查磁盘。
- 杀进程**必须**用 `powershell -Command "Stop-Process -Name <exe> -Force"`；git bash 的 `taskkill //F //IM` 无效，会导致"端口占用→Fatalf 秒退"，让验证假通过。
- **后台/脱离进程会被会话清理杀掉**：`Start-Process`、detached spawn 起的服务在下次工具调用前就没了（浏览器报 `ERR_CONNECTION_REFUSED`）。正确做法是把「起服务 + 用服务 + 收服务」收进**同一个前台进程**（验证脚本自包含）。
- `vite build` **不做类型检查**；类型错误要另跑 `vue-tsc`（仓库里还有 8 条历史遗留错误，判"是否新增"必须 `git show HEAD:<file>` 比对基线）。
- **两种错误报告形态别混**：业务错误走 `dto.Fail(c, 400, msg)` = **HTTP 200 + body `{code:400}`**；只有"路由不存在/中间件拦截"才真的改 HTTP 状态码（`dto.FailHTTP`）。写断言时业务错误查 `r.json.code`，别查 `r.status`。
- **写进 `evalOr` 模板字符串里的正则要双反斜杠，写在 Node 侧的要单反斜杠**——多转义一层会让正则永远匹配不上，断言"恒为真"地空转却一直 PASS。这类空转只能靠反向变异验证抓出来。
- **`el.click()` 会绕过 `pointer-events: none`**（程序化事件不做命中测试）→ 用它断言"按钮点不动"**永远 PASS**（空转）。断言渲染层行为（点不动 / 看不见 / 被遮挡）必须走 CDP `Input.dispatchMouseEvent` 打真实鼠标事件（`ui.mjs` 的 `realClick()`）。批次 8 实测：变异版本 detail = `click=true dlg=false`，只有真实鼠标点才抓得到。
- **Node 的 `fetch`（undici）会静默忽略 `init.auth`**（那是浏览器 XHR 的选项）→ 用它做 HTTP Basic Auth 会让请求变成"无认证"，**一整批 401 断言假通过**。必须自己拼 `Authorization: 'Basic ' + Buffer.from(u+':'+p).toString('base64')`。批次 9 追加实测踩到（16 PASS 全是空的）。
- **`#/app/<id>` 是 `StandaloneApp` 独立单应用模式**，切 hash 会**整个替换页面**（Explorer 被卸载重建）。要验证"两个窗口并存"的场景（如管理台改挂载 → 文件管理器刷新）必须走「桌面外壳 → 双击桌面图标开窗」的多窗口路径；用切 hash 的方式测，测到的其实是"窗口重建后重新拉取"，是假通过。
- **同一文件并行下发多个 Edit 会丢改动**（工具报成功，但部分写入被并发读-改-写覆盖）。同一文件的多次编辑必须串行下发，改完用 Grep/Read 复核。
- **`.bat` 里不要出现中文，已有中文也要清掉**（批次 7 + 批次 9 两次实测炸过）：`build.bat`/`start.bat` 是 **UTF-8 无 BOM**，cmd 按系统 ANSI（GBK）代码页读取；中文字节被 GBK 解读后可能凑出 `&`/`|`/`>` 等元字符，直接把命令行打断。现象是构建 0.2 秒 `BUILD FAILED` + `'ist' 不是内部或外部命令`（`dist` 被截断）；批次 9 的表现是**中文 `rem` 吞掉后一行**（`setlocal` 消失）+ 早段报「文件名、目录名或卷标语法不正确」。脚本内容一律 ASCII 英文。
- **PowerShell 工具禁止直接调 `cmd.exe`**（"cmd.exe cannot be used from the PowerShell tool"）。要跑 `.bat` 用 Node `spawn('cmd.exe', ['/c','build.bat'])`——`%TEMP%\cp-verify2\run-build.mjs`（跑 build.bat 并自检产物）与 `run-start.mjs`（跑 start.bat + taskkill）就是这么干的。
- **跑 `start.bat` 会在仓库里建真实 `server/data`**（它 `cd /d "%~dp0server"` 且不设 `CP_DATA`），即建出一个真实 admin 账号；密码只在那一瞬的日志里，用户没看到，下次启动又不会重印 → 直接进不去。**验证完必须删掉整个 `server/data`**（先确认 `CreationTime` 就是验证时刻），或先重定向 `CP_DATA` 到临时目录。
- **跑探针前先核对 `exe.LastWriteTime > 所有源文件`**（批次 10 踩到）：`m3u8.go` 比 `cloudpan.exe` 晚 2 分半，不重建就跑探针，验证的是**旧二进制的绿色**，等于白测。
- **`build.bat` 在 PowerShell 里 `$LASTEXITCODE` 可能是 1 但实际成功**：脚本尾部 `goto :eof` 会带上最后一条命令的 errorlevel。**判成功看有没有打印 `BUILD OK`**，别只看退出码。
- **PowerShell `Tee-Object` 回显中文乱码 ≠ 文件内容损坏**（控制台按 ANSI 解码所致）。判断探针结果以写入的结果文件为准。
- **ESM 恒为严格模式**（批次 10 探针直接崩）：`chrome = spawn(...)` 这种未声明赋值会 `ReferenceError`，而 `cleanup()` 里引用它 → 必须在顶部先 `let chrome = null;`。
- **`navigator.clipboard.writeText` 需要「用户激活」**：headless 下程序化 `element.click()` 不产生 user activation，**复制断言必然失败**。断言"复制按钮真能复制"必须走 CDP `Input.dispatchMouseEvent` 真实鼠标事件。

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
11. **WebDAV 跨挂载 MOVE/COPY 必须显式拒绝**（批次 9）。`golang.org/x/net/webdav` 的 `MOVE` 在目的地已存在且 `Overwrite: T` 时是 **先 `RemoveAll` 再 `Rename`** —— 放行跨挂载就会**先删掉源盘的文件再失败**。同理统一根（`Mount == nil`）上的 `Mkdir`/`RemoveAll`/`Rename` 也一律拒绝。别以为"只是挪不动而已"。
12. **`fscore.LocalDriver{Root}` 直接构造 vs `NewLocal()` 的区别在 webdav 里是安全边界**：`NewLocal` 内部 `os.MkdirAll` 会**静默建目录**。`webdav.go` 的 `davLocal()` 必须用直接构造（`&fscore.LocalDriver{Root: abs}`）。另外 `OpenFile` 的 `os.MkdirAll(dir)` 只能挂在 `write == true` 分支——读请求不能凭一个不存在的路径建目录。
13. **`LocalDriver.CreateFile` 是 `os.Remove(phys)` + `Rename`，所以"写文件"的路径必须先挡住"目标是目录"**（批次 9 追加实测到的数据破坏）。当 `phys` 是**空目录**时 `os.Remove` 成功 → 目录被整个替换成文件；非空目录才失败。`DavFS.OpenFile` 的写入分支现在先 `os.Stat` 判目录并返回 `davRejectWriteFile`（→ 405，零字节落盘）。**任何新的"写"入口（新的 driver / 新的上传路径）都要过同一道坎。**
14. **`x/net/webdav` 取 PROPFIND 的 displayname 走的是 `OpenFile(...).Stat()`，不是 `FileSystem.Stat`**（`prop.go` 的 `props()`）。所以想改对外显示的名字，只改 `FileSystem.Stat` 是**死代码**；要包一层 `webdav.File` 覆盖 `Stat()`（现由 `davReadFile` + `davAliasInfo` 实现，让挂载根显示挂载名而非物理目录名）。**两处都要改**，`Stat()` 里也留了分支给 walkFS 用。
15. **前端滚动条消失的经典陷阱组合（批次 10）**：`flex:1` **只在 flex 父容器里生效**；且 flex 子项默认 `min-height:auto`，会被内容撑开、把滚动能力压掉。**两个条件都要满足**（父容器 column flex + 子项 `min-height:0`）才出滚动条。`Explorer.vue` 的 `.file-area` 就是为此而设；改这块布局时别把 `.file-area` 去掉或丢掉 `min-height:0`。判断方法：`scrollHeight === clientHeight` 就说明容器被内容撑开了。

## 已锁定的改造决策（2026-09-11 用户拍板）
登录页只留密码框；文件管理器不要盘符、根视图直接列挂载点；挂载入口放文件管理器内（后端需新增目录浏览接口）；手机先走 WebDAV；公开分享暂留；**用户组 / 权限模型彻底删除、权限写死为管理员全开**（配额不限；`RecycleRetentionDays: 0` = 回收站永久保留，系统不再有任何"按时间自动物理删用户文件"的行为）。

## README 状态（批次 7 已重写，勿再拿旧描述当事实）
README 已于批次 7 按当前功能面重写（双语）：删掉整章《在线 Office（ONLYOFFICE）部署指南》，顶部介绍 / 功能特性 / 快速开始 / 目录结构 / 部署加固全部重写，截图由 25 张（三主题 + 含已删界面）换成当前状态实拍 5 张（登录页 / 桌面 / 根视图 / 挂载对话框 / 挂载点内容，由 `ui.mjs` 真实动线产出）。**《云盘接入与扫码绑定》与历史 changelog 原样保留**——后者开头有「范围说明」交代"其中描述的能力在本分支多已裁剪"。另外：Go 版本要求已从 `1.22+` 更正为 **`1.27+`**（`go.mod` 是 `go 1.27.0`）。核对功能仍以代码为准。

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
- 批次 7 ✅：全链路 `build.bat` 通过（exit 0 / `BUILD OK` / 24.7s / exe 61 MB / 嵌入 **134** 个 dist 条目）；用**构建产物本身**跑 `smoke.mjs` **60/60**、`ui.mjs` **22/22**；`start.bat` 语法与首部署日志验证通过；README 按当前功能面重写完成（见上节）。
- 批次 8 ✅ `3d8676a`：修复用户内网真机反馈的 5 个问题 —— ①空态「挂载文件夹」按钮点不动（根因：`base.css` 的 `.empty-hint` 带 `pointer-events:none`，空态里嵌的按钮被一起禁用）；②进入挂载文件夹后「后退」点了没反应（根因：`pushHistory` 压的是目标路径而非来处）；③管理台新增挂载后文件管理器不刷新（新增 `cp-policies-changed` 广播）；④删管理台「系统更新」页签及全部前端调用（**后端 `/api/admin/update/*` 路由保留**）；⑤离线下载"没下下来"（**后端本就正常**，实测 38MB / 3s 下完并落盘；真因是前端完成后不刷新目录 + 不显示失败原因）。
- 验证：`smoke.mjs` **60/60**、`ui.mjs` **34/34**（批次 8 新增 12 项）、`probe-all.mjs`（离线下载专项，新）、**4 处反向变异全部命中**（含离线下载变异精确复现了用户现象）。
- **全部 10 个批次已完成**，`docs/单用户私有化改造计划.md` 的 §6 状态表全绿（§7.3 = 批次 7，§7.4 = 批次 8，§7.5 = 批次 9，§7.6 = 批次 9 追加，§7.7 = 批次 10）。
- 批次 7 顺手修掉的真问题：① `build.bat` 不重建 `.keep`（先 `rmdir` 整个 dist）→ 空克隆后 `go:embed` 会失败，已补 `type nul > dist\.keep`；② README 的 Go 版本要求写着 `1.22+`（实际 `go.mod` 要 1.27.0）→ 更正为 `1.27+`；③ `start.bat` 注释还写着"默认账号 admin / admin123"（实际首部署是随机密码）→ 已改。
- 批次 9 ✅：① 删掉系统自更新（`handler/update.go` 整文件 536 行 + 4 条路由 + `UpdateLog` + `main.go` 自重启）；② WebDAV 改为**统一入口 `/dav/`**（列全部本机挂载）+ 单挂载 `/dav/<挂载名>/` + 兼容别名 `/dav/<盘符>/`；顺手修掉 `DavAuth()` 里令 README 写的地址一律 403 的遗留前缀校验。
- 批次 9 验证：`probe-webdav.mjs` **44/44**（新，含统一根只读/跨挂载拒绝/同名去重/401/编码）、`smoke.mjs` **60/60**、`ui.mjs` **35/35**、`build.bat` 真跑 exit 0 / 23.6 s / exe 64,166,400 B / 嵌入 119；**3 处反向变异全部命中**，其中 1 处（`davWriteFile.Close()` 临时文件残留）是首轮实测真实抓到的缺陷。
- 批次 9 追加 ✅：修掉手机端接入实测暴露的两个**真缺陷** —— ①`PUT /dav/<挂载名>/` 会把**空目录整个删掉换成文件**（`LocalDriver.CreateFile` 的 `os.Remove(phys)` 所致；现返回 405 且零字节落盘）；②统一根的 `displayname` 是物理目录名而非挂载名（webdav 走 `OpenFile().Stat()`，只改 `FileSystem.Stat` 是死代码）。新增 `probe-phone.mjs` **40/40**、`probe-dav-root.mjs`；批次 9 的 44/44、60/60、35/35 全部复跑无回归。
- 批次 10 ✅：用户真机反馈 4 件事 —— ①列表内容多时无法滚动（`flex:1` 只在 flex 父容器生效 + 子项缺 `min-height:0`）；②离线下载"不支持"= 后端实测都通、缺的是可诊断性（补 tracker/节点数回写/超时原因/Referer/内网开关/前端显示原因）；③新增 m3u8（HLS）下载；④多网卡地址全列出由用户挑。验证：`probe-m3u8` **28/28**、`probe-offline` **11/11**、`probe-addr` **16/16**、`probe-scroll` **13/13**，回归 `smoke` 60/60 / `probe-webdav` 44/44 / `probe-phone` 40/40 / `ui` 35/35，`go build`/`go vet`/`build.bat` exit 0，2 处反向变异精确命中。详见当日日志 §批次 10。
- 待用户拍板（均不阻塞）：① `docs/test-evidence/` 上游测试证据存档（含 guest / 终端旧截图，README 已不引用）是否清理；② 媒体中心当前是 `installable`（需去应用中心装），是否改为开机即在桌面；③ 手机端前端（暂缓，先走 WebDAV）。
