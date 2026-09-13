# self-lan-pan（CloudPan）长期备忘

> 详细过程看 `docs/单用户私有化改造计划.md`（§6 批次表 / §7 验证记录）与 `docs/CHANGELOG.md`。
> 本文件只留"改代码时必须知道"的硬知识。

## 定位与约定
- 单用户私有网盘：Go(Gin+GORM+glebarez/sqlite→modernc 纯 Go) + Vue3+TS+Vite+Pinia；`go:embed all:dist` 单二进制，端口 18322（`server/main.go` 显式监听器）。
- 环境变量：`CP_PORT` / `CP_DATA`（默认 cwd 下 `./data`）/ `CP_PUBLIC_URL` / `CP_TRUSTED_PROXIES`。
- **用户画像：只服务内网、只有 admin 一个账号。** 多用户那套是上游遗留，正在被裁剪，别默认它是多用户产品。
- git：`main` + `origin=github.com/menglideli/self-lan-pan`。
- **两个 `.sh` 在 git 里必须 `100755`，`.gitattributes` 锁 `*.sh eol=lf` / `*.bat eol=crlf`** —— 别改回去，否则 clone 下来 `Permission denied` 或 `/bin/bash^M: bad interpreter`。
- 构建：`build.bat` / `build.sh` = npm build → 拷 dist → `server/internal/web/dist` → `go build` → **装配 `out/` 交付目录**；启动脚本 `start.bat` / `start.sh` **自适应两种摆放**（exe 同级 = 发布包；exe 在 `./server/` = 源码树）。产物落 `out/`，`out/data` 被保留不删（那里可能是真实数据）。
- `.gitignore` 已忽略 `server/data`、`web/dist`、`server/internal/web/dist/*`（留 `.keep`）、`server/cloudpan*`、`out/`、`C/`、`D/`。

## 扩展点（改动必看）
- **功能开关四处联动**：`apps/manifest.go` 的 `Manifest` → 路由 `middleware.AppGate(key)` → 前端 `stores/appstate.ts.isAvailable()` + `stores/apps.ts` 的 `APPS[].feature` → `themes/registry.ts` 的 `APP_COMPONENTS`。漏一处 = "入口没了接口还在"或"接口 403 图标还在"。
- ⚠️ **`model.AppEnabled` 对清单外 key 返回 `true`** → 「从 Manifest 删 key 但留着 `AppGate(key)` 路由」= 门控静默失效。删功能必须同时删路由。
- 路由唯一装配点：`server/internal/handler/router.go`。前端主题插件化：`themes/<id>/index.ts` → `themes/registry.ts`（现只剩 `'win12'`）。
- 存储：`fscore` 只注册 `local` 一个驱动。`driver/`（pan123/aliyun/baidu/tianyi）与 `handler/cloudauth.go` **已整包删除**，`/api/cloud/*` 四条路由 404；策略传非 `local` 的 type → 直接 400。**用户子目录隔离已关闭**，本地盘直接暴露挂载根真实内容。
- **挂载 = 一条 `model.Policy`（type=local）**。`Letter` UI 上已不显示，但**不能删**——它仍是 WebDAV 兼容别名路径段（`/dav/<letter>/`）、排序键、唯一索引。前端不传时由 `localdir.go` 的 `genPolicyLetter()` 生成（ASCII ≤4 位；纯中文退化为 `M`/`M1`…）。
- **WebDAV = 统一入口 `/dav/`**：`/dav/` 是虚拟根（列全部 local 挂载、**不含云盘**、写一律拒）；`/dav/<挂载名>/` 是正式路径；`/dav/<盘符>/` 是别名。**路径段规则只在后端**（`webdav.go` 的 `davSeg()`/`davPathOf()`），经 `PolicyList` 的 `davPath` 字段下发前端 —— 改规则只改后端一处。
- **WebDAV 认证（用户问"手机怎么配"时的事实答案）**：账号=网页用户名 `admin`；密码=**WebDAV 独立密码**（设置页设，`PUT /api/users/me/webdav-password`），**不是登录密码**，未设时 401；地址 `http://<内网IP>:18322/dav/`；开关=`SiteSetting.webdav_enabled` + `AppEnabled("webdav")`；失败锁定与网页登录**共用同一张表**（IP+用户名，5 次锁 15 分钟）。
- **挂载根 = 策略 `RootPath` 本身**（不额外套一层）。写断言/建 fixture 时别以为外面还包一层目录名。
- **系统自更新已整体删除**：`handler/update.go` + 4 条 `/api/admin/update/*` 路由 + `model.UpdateLog` + `main.go` 自重启都没了。旧库残留 `update_logs` 表无害。
- **本机目录浏览**：`GET /api/admin/fs/dirs`（`localdir.go`，`/admin` 组=仅管理员）。不传 `path` 返回盘符/根，传 `path` 返回子目录。**按用户要求不设白名单、整机可翻**；两条底线=必须绝对路径 + 必须已存在。
- **挂载列表变化走 `cp-policies-changed` 广播**：管理台 `loadAll()` 算指纹（id/名称），变了才 `dispatchEvent`；`Explorer.vue` 收到后重拉 `fsApi.policies()`，当前挂载被卸载则退回根视图。**别复用 `cp-refresh-explorer`**（那个只刷当前目录文件列表，挂载根视图不更新）。
- **Explorer 导航历史是 `(policyId, path)` 二元组**（`policyId===null` = "此电脑"根视图）。`pushHistory()` 压的是**来处**，必须在改 `tab.policyId`/`tab.path` **之前**调用；写反了就是"后退可点但点了没反应"。
- **离线下载三条链路**：`offline`(HTTP 直链) / `bt`(.torrent+magnet) / `m3u8`(HLS)。类型判定在 `bt.go` 的 `sniffOfflineKind()`（**m3u8 分支必须在 http 之前**）+ `OfflineHandler.Create` 的 switch；分发在 `tasks.go` 的 `run()`。**无后缀清单靠运行时嗅探**：`runOffline` 里 `bufio.Peek(1024)` + `looksLikeM3U8Body()` 命中后改 `type=m3u8` 就地转 `runM3U8Task()`。
- **m3u8 下载器**：`handler/m3u8.go`，纯 Go 零依赖、不需要 ffmpeg。支持 master 按 `BANDWIDTH` 选最高码率、`#EXT-X-BYTERANGE`、`#EXT-X-MAP`、`AES-128`（**未给 IV 时按媒体序号推导 16 字节大端 IV**，`ivFromSeq()`）。所有取数走 `ssrfHTTP` 保住 SSRF 三层防护。默认 8 并发（上限 32）/20000 分片/20GB。
- **多网卡地址枚举**：`handler/netaddr.go` 的 `LocalAddresses(port)` → `GET /api/system/addresses`（`router.go` 的 `ug` 组），带网卡名 + `kind`(lan/virtual)。前端 `utils/lanaddr.ts`（模块级缓存 + single-flight）+ `components/AddressPicker.vue`。**AddressPicker 用自绘 `position:fixed` 遮罩** —— 复用 `.dialog-mask`(`position:absolute`) 会漂。
- **「复制链接类」入口必须一律走 AddressPicker，别写 `location.origin`**（多网卡机器上 origin 未必是对方能访问到的那个）。约定：**把「地址」与「路径」拆开存**（`dlPath` / `sharePath`），显示用 `preferred()` 拼，复制时弹选择器。已接入 5 处：设置页 WebDAV、设置页分享列表、管理台挂载路径、资源管理器（提取直链/分享）、记事本分享。
- **内网地址放行开关**：`SiteSetting.offline_allow_private`（默认关）→ `ssrf.go` 的 `ssrfAllowPrivate atomic.Bool`（用 **`.Store()`**，无 `.Set`）。由 `SettingsSet` 与 `main.go` 启动时各调一次 `ApplySSRFSetting()`。**新增站点开关要记得接"启动时应用"那条路径。**
- **任务表 `Msg` 与 `Error` 是两列，语义不能混**：`Msg`=运行中阶段提示（任务一结束**必须清空**）；`Error`=**只在失败时**写。早先共用 `error` 列 → `finished` 的任务永远挂「正在合并分片」。三层防御：独立 `Msg` 列 + `run()` 成功/取消分支清 `Msg` + 前端只在 `queued/processing` 显示 `msg`。**加进度文案写 `setTaskMsg()`，别写 `error`。**
- **离线任务三动作分开路由**：`POST /api/offline/:id/cancel`、`/retry`、`DELETE /api/offline/:id`（早先 `DELETE` 实际是取消，失败任务永远删不掉）。**重试必须先 `uncancel(id)`**（不清取消标记 → "状态排队中但永远不动"）+ `resetTaskProps()`；**跑动中的任务禁止删除**（记录没了文件照样落盘）。
- **产物命名收敛在 `tasks.go`**：`resolveOutName(d, dir, userName, defaultExt)` —— 填了用用户名字（缺扩展名补，含 `/`、`\` 直接 400），没填走 `nextDailyName()` 生成 `YYYYMMDD_NN<ext>`。`uniqueFileName()` 保证同名追加 `_1` 而不是覆盖。HLS 后缀由 `mediaDefaultExt=".mp4"` 一个常量控制。
- **策略「测通」走 `GET /api/policies/status?policyId=`（`handler/policycheck.go`）**：**先 `os.Stat` 再 `List`**。别退回 `fscore.NewLocal` —— 它内部 `os.MkdirAll`，会把已删的挂载目录悄悄重建再报「目录可读」，是**验证手段自己在制造假通过**。

## 跨平台（批次 12 实测，非推测）
- 服务端可跑：Windows x64/ARM64、Linux x64/ARM64/ARMv7、macOS Intel/Apple Silicon。七目标 `CGO_ENABLED=0` 交叉编译全 exit 0，且**读产物文件头核对**（PE/COFF、ELF、Mach-O；三个 Linux 产物是 `ET_EXEC` **静态**可执行，不依赖 glibc/musl）。
- 为什么能：SQLite 纯 Go（无 CGO 是出静态二进制的前提）；源码 **0 处** `syscall`、`//go:build`、`os/user`；已有 3 处 `runtime.GOOS` 分支本就写了跨平台逻辑。`go.mod` 里 `creack/pty` 是**从未 import 的残留**。
- **iOS 只能当客户端**：系统不允许 App 后台常驻监听端口（换任何语言一样），且 Go 编到 iOS 需 CGO+Xcode+gomobile。iPhone 用 Safari 或第三方 WebDAV App（**自带「文件」App 不支持 WebDAV**）。对外表述**不能含糊成"支持 iOS"**。
- 不为 NAS / 路由器做适配（用户明确不需要）。

## 环境坑（踩过的）
- **本机 Bash 工具 PATH 完全失效**（`ls`/`dirname`/`cat` 全 command not found）→ 一律用 PowerShell / Read / Grep / Glob。PowerShell 5.1 处理中文会写坏编码，批量改代码用 Node 或 `[System.IO.File]`。
- **沙箱下「我能写」≠「我拉起的子进程能写」**：`GOCACHE`/`GOTMPDIR` 指到工作区外时 Go 报 `Access is denied`，而 PowerShell 自己往同目录写是成功的。默认 `%LOCALAPPDATA%`/`%TEMP%` 可靠。排查这类失败先怀疑路径可写性。
- **磁盘满会伪装成代码缺陷**：`link.exe: resize output file failed: not enough space` 实为 C 盘只剩 0.2GB（Go 缓存 2.86GB）。`go clean -cache` 释放约 2.9GB。**`Get-PSDrive` 剩余空间读数有延迟**，清完立刻读可能显示"释放 0GB"，别据此判无效。
- 本机 Go 1.26.3，`go.mod` 要 1.27.0：必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**，工具链校验会失败）。`proxy.golang.org` 不可达。Go 在 `C:\Program Files\Go`。
- `build.bat` 会先 `rmdir` 整个 dist 再 xcopy，**所以它必须自己重建 `.keep`**（`build.sh` 本就有 `touch .keep`）。
- **`.bat` 里绝不能出现中文**（两次炸过）：文件是 UTF-8 无 BOM，cmd 按 GBK 读取，中文字节可能凑出 `&`/`|`/`>` 打断命令行（现象：0.2 秒 `BUILD FAILED` + `'ist' 不是内部或外部命令`；或中文 `rem` 吞掉后一行让 `setlocal` 消失）。脚本内容一律 ASCII。
- **PowerShell 工具禁止直接调 `cmd.exe`**：要跑 `.bat` 用 Node `spawn('cmd.exe',['/c','build.bat'])`（`%TEMP%\cp-verify2\run-build.mjs`）。
- **`build.bat` 在 PowerShell 里 `$LASTEXITCODE` 可能是 1 但实际成功**（尾部 `goto :eof` 带上最后一条命令的 errorlevel）→ **判成功看有没有打印 `BUILD OK`**。
- **杀进程必须** `powershell -Command "Stop-Process -Name <exe> -Force"`；git bash 的 `taskkill //F //IM` 无效 → "端口占用→Fatalf 秒退"会让验证假通过。
- **后台/脱离进程会被会话清理杀掉**（`Start-Process`、detached spawn 起的服务下次工具调用前就没了）→ 把「起服务+用服务+收服务」收进**同一个前台进程**。
- **跑 `start.bat` 会在仓库里建真实 `server/data`**（即建出真 admin 账号，密码只在那一瞬日志里，下次不重印 → 直接进不去）。**验证完必须删掉整个 `server/data`**，或先重定向 `CP_DATA`。
- **跑探针前先核对 `exe.LastWriteTime > 所有源文件`**，否则验证的是旧二进制的绿色，等于白测。
- `vite build` **不做类型检查**；要另跑 `vue-tsc`（仓库有 8 条历史遗留错误，判"是否新增"必须 `git show HEAD:<file>` 比对基线）。
- **两种错误报告形态别混**：业务错误走 `dto.Fail(c,400,msg)` = **HTTP 200 + body `{code:400}`**；只有"路由不存在/中间件拦截"才改 HTTP 状态码（`dto.FailHTTP`）。断言业务错误查 `r.json.code`，别查 `r.status`。
- **写进 `evalOr` 模板字符串的正则要双反斜杠，Node 侧单反斜杠** —— 多转义一层会让正则永远匹配不上，断言"恒为真"地空转却一直 PASS。
- **`el.click()` 会绕过 `pointer-events:none`**（不做命中测试）→ 用它断言"按钮点不动"**永远 PASS**。断言渲染层行为必须走 CDP `Input.dispatchMouseEvent`（`ui.mjs` 的 `realClick()`）。同理 `navigator.clipboard.writeText` 需要用户激活，headless 下程序化 click 必失败。
- **Node 的 `fetch`(undici) 会静默忽略 `init.auth`** → 做 HTTP Basic Auth 会让请求变成"无认证"，**一整批 401 断言假通过**。必须自己拼 `Authorization: 'Basic ' + Buffer.from(u+':'+p).toString('base64')`。
- **`#/app/<id>` 是 `StandaloneApp` 独立单应用模式**，切 hash 会**整个替换页面**。要测"两个窗口并存"（管理台改挂载→文件管理器刷新）必须走「桌面外壳→双击 `.desk-icon` 开窗」。`ContextMenu` 与 `DialogHost`(toast) **只挂在 `WinDesktop.vue` 上**，单应用模式下两者都不存在。
- **同一文件并行下发多个 Edit 会丢改动**（工具返回 `Successfully edited` 但部分写入被并发读-改-写覆盖）。**必须串行下发，改完立刻 Grep 复核。** 批次 11 第三次中招且最隐蔽（`probe-webdav.mjs` 发 5 处落 0 处）。
- ESM 恒为严格模式：探针里 `chrome = spawn(...)` 这类未声明赋值会 `ReferenceError`，而 `cleanup()` 引用它 → 顶部先 `let chrome = null;`。
- 端到端套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓初始管理员密码 → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → 重启触发启动期任务 → 查磁盘。

## 已锁定的设计决策
- 登录页只留密码框；文件管理器不要盘符、根视图直接列挂载点；挂载入口在文件管理器内；手机走 WebDAV；公开分享暂留；**用户组/权限模型彻底删除**（`model.AdminPerms()` 写死；配额不限；`RecycleRetentionDays:0` = 回收站永久保留，系统**不再有任何按时间自动物理删用户文件**的行为）。
- 三项遗留已拍板：`docs/test-evidence/` 清理；手机端前端**不做**；媒体中心保留但不作为动线。**别再当待办。**
- **不要重新引入任何「按 mtime 自动物理删除挂载目录」的清理逻辑**（批次 1 前曾构成"一小时内丢数据"的组合拳）。
- README 已于批次 7 重写、批次 11 清过云盘描述、批次 12 加「支持平台」「构建与运行」。Go 版本要求 **1.27+**。核对功能以代码为准，别拿旧 README 当事实。

## 进度与测试基线
- 13 个批次全部完成（详细见 `docs/单用户私有化改造计划.md` §6/§7）：1 删游客清理+关用户隔离 / 2 删注册·用户管理·用户组·站内共享 / 3 只留 win12 主题 / 4 删 7 个应用（计算器·壁纸·图库·测速·浏览器·终端·在线 Office）/ 5 挂载根视图去盘符 / 6 新增「挂载文件夹」+ 目录浏览器 / 7 全链路构建与 README 重写 / 8 修真机 5 问题 / 9 删自更新 + WebDAV 统一入口 / 10 滚动修复 + m3u8 + 多网卡地址 + AddressPicker 收口 / 11 任务 Msg-Error 拆列 + 三动作分离 + 存储策略收口 / 12 跨平台。
- 桌面实测只剩：此电脑 / 回收站 / 记事本 / 任务中心 / 应用中心 / 设置 / 管理控制台（媒体中心 installable）。apps 只剩 10 个 key。
- **测试基线（任何改动后应复跑）**：`smoke` 61 / `probe-policy` 26 / `probe-m3u8` 48 / `probe-offline` 22 / `probe-webdav` 45 / `probe-phone` 40 / `ui` 36 / `probe-addr` 27 / `probe-scroll` 13 = **318 条全绿**；`probe-dav-root` exit 0。探针在 `%TEMP%\cp-verify2\`。
- 待用户拍板：`.workbuddy/` 是否公开（内含对话原话与运维隐患，**agent 不擅自动手**）；LICENSE 是否追加自己版权行；是否 push 到 origin。
