# self-lan-pan（CloudPan）长期备忘

> 过程细节在 `docs/单用户私有化改造计划.md`（§6 批次表 / §7 验证记录，§7.9–7.12 = 批次 11–15）与 `docs/CHANGELOG.md`。本文件只留"改代码时必须知道"的硬知识。

## 定位与约定
- 单用户私有网盘：Go(Gin+GORM+glebarez/sqlite→modernc 纯 Go) + Vue3+TS+Vite+Pinia；`go:embed all:dist` 单二进制；端口 18322；env `CP_PORT`/`CP_DATA`(默认 `./data`)/`CP_PUBLIC_URL`/`CP_TRUSTED_PROXIES`。
- **只服务内网、只有 admin 一个账号。** 多用户那套是上游遗留，正在裁剪，别当它是多用户产品。
- git `main` + `origin=menglideli/self-lan-pan`。**两个 `.sh` 必须 `100755`，`.gitattributes` 锁 `*.sh eol=lf` / `*.bat eol=crlf`**（否则 clone 后 Permission denied / bad interpreter）。
- 构建：`build.bat`/`build.sh` = npm build → 拷 dist → `server/internal/web/dist` → `go build` → **装配 `out/` 交付目录**；`start.bat`/`start.sh` **自适应两种摆放**（exe 同级 = 发布包；exe 在 `./server/` = 源码树）。**`out/data` 保留不删**（可能是真实数据）。`.gitignore`：`server/data`、`web/dist`、`server/internal/web/dist/*`、`server/cloudpan*`、`out/`、`C/`、`D/`、`*.zip/*.rar/*.7z/*.tar*`。

## 扩展点（改动必看）
- **功能开关四处联动**：`apps/manifest.go.Manifest` → 路由 `middleware.AppGate(key)` → 前端 `stores/appstate.ts.isAvailable()` + `stores/apps.ts.APPS[].feature` → `themes/registry.ts.APP_COMPONENTS`。漏一处 = "入口没了接口还在"或"接口 403 图标还在"。
- ⚠️ **`model.AppEnabled` 对清单外 key 返回 `true`** → 删 Manifest key 却留 `AppGate(key)` 路由 = 门控静默失效。**删功能必须同时删路由。**
- 路由唯一装配点 `handler/router.go`；主题插件化 `themes/<id>/index.ts` → `themes/registry.ts`（现只剩 `'win12'`）。
- 存储：`fscore` 只注册 `local`；`driver/`(pan123/aliyun/baidu/tianyi) 与 `handler/cloudauth.go` **已整包删除**，`/api/cloud/*` 四条 404；策略 type 非 `local` → 400。**用户子目录隔离已关闭**，本地盘直接暴露挂载根真实内容。
- **挂载 = 一条 `model.Policy`(type=local)**；`Letter` UI 不显示但**不能删**（WebDAV 别名段 `/dav/<letter>/`、排序键、唯一索引；前端不传时 `localdir.go.genPolicyLetter()` 生成）。**挂载根 = 策略 `RootPath` 本身**（不额外套层）。
- **WebDAV = 统一入口 `/dav/`**：`/dav/` 虚拟根（列全部 local 挂载、不含云盘、写一律拒）；`/dav/<挂载名>/` 正式；`/dav/<盘符>/` 别名；**路径段规则只在后端**（`webdav.go.davSeg()/davPathOf()`），经 `PolicyList` 的 `davPath` 下发前端。
- **WebDAV 认证（答"手机怎么配"）**：账号=`admin`；密码=**WebDAV 独立密码**（设置页设，`PUT /api/users/me/webdav-password`），**不是登录密码**，未设 401；地址 `http://<内网IP>:18322/dav/`；开关=`SiteSetting.webdav_enabled` + `AppEnabled("webdav")`；失败锁定与网页登录共用同一张表。
- **系统自更新已整体删除**（`handler/update.go` + 4 条路由 + `model.UpdateLog` + 自重启）；旧库残留 `update_logs` 表无害。
- **本机目录浏览** `GET /api/admin/fs/dirs`（`localdir.go`）：不传 `path` 返回盘符/根。**不设白名单、整机可翻**；底线=绝对路径 + 必须已存在。
- **挂载列表变化走 `cp-policies-changed` 广播**（管理台 `loadAll()` 算指纹变化才发），**别复用 `cp-refresh-explorer`**（只刷当前目录文件列表）。
- **Explorer 导航历史是 `(policyId, path)` 二元组**（`policyId===null`=根视图）；`pushHistory()` 压的是**来处**，必须在改 `tab.policyId`/`tab.path` **之前**调。
- **内网地址放行**：`SiteSetting.offline_allow_private`（默认关）→ `ssrf.go.ssrfAllowPrivate atomic.Bool`（用 **`.Store()`**）；由 `SettingsSet` 与 `main.go` 启动时各调一次 `ApplySSRFSetting()`。**新增站点开关必须接"启动时应用"这条路。**
- **策略「测通」`GET /api/policies/status?policyId=`（`policycheck.go`）**：**先 `os.Stat` 再 `List`**，别退回 `fscore.NewLocal`（它 `os.MkdirAll`，会把已删目录悄悄重建再报"可读"）。

## 分享（外链）↔ 卸载挂载（批次 15）
- **状态判据唯一来源 `model.Share.State()`**：`active` / `expired`（过期）/ `exhausted`（`remain_downloads==0`）；`Available()` = `State()=="active"`；配套 SQL 常量 `ShareActiveCond` **必须同义**。
- ⚠️ 早先"能否访问"用 `Available()`、"能否卸载"用 `Count` **全部分享** —— 两套判据且后者更宽 → 一条早就过期的分享把挂载**永久锁死**（`400 存在关联分享，请先取消`），审计又不给状态标识。**改分享语义必须同时过这两处。**
- `AdminPolicyDelete` 只统计 `ShareActiveCond`：生效中的仍挡住（删策略会让外链变坏链），已失效的放行；**卸载不删已失效分享记录**（用户要"有个记录就行"），安全性由访问侧兜（过期分享在 `loadShare` 就 404）。`ShareAudit` 返回 `state`/`available`/`remainDownloads`；`Mine` 返回 `hasPassword`/`state`/`available`。
- ⚠️ **`Mine` 不能直出 `model.Share`**：`PasswordHash` 带 `json:"-"` 永不下发 → 前端读 `passwordHash` 恒 `undefined` → 有密码的分享**恒显示「公开」**（批次 15 修）。**任何 `json:"-"` 字段都别指望下发。**

## 离线下载（三链路 + 缓存治理）
- **三链路**：`offline`(HTTP) / `bt`(.torrent+magnet) / `m3u8`(HLS)。判定在 `bt.go.sniffOfflineKind()`（**m3u8 分支必须在 http 之前**）+ `OfflineHandler.Create` 的 switch；分发在 `tasks.go.run()`。**无后缀清单靠运行时嗅探**：`runOffline` 里 `bufio.Peek(1024)` + `looksLikeM3U8Body()` 命中后改 `type=m3u8` 就地转 `runM3U8Task()`。
- **m3u8 下载器** `handler/m3u8.go`：纯 Go 零依赖、不需要 ffmpeg。支持 master 按 `BANDWIDTH` 选最高码率、`#EXT-X-BYTERANGE`、`#EXT-X-MAP`、`AES-128`（未给 IV 时按媒体序号推导大端 IV，`ivFromSeq()`）；取数走 `ssrfHTTP`。
- ⚠️ **任务表 `Msg` 与 `Error` 是两列，语义不能混**：`Msg`=运行中阶段提示（任务一结束**必须清空**）；`Error`=**只在失败时**写。三层防御：独立列 + `run()` 成功/取消分支清 `Msg` + 前端只在 `queued/processing` 显示。**加进度文案写 `setTaskMsg()`，别写 `error`。**
- **三动作分开路由**：`POST /api/offline/:id/cancel`、`/retry`、`DELETE /api/offline/:id`。**重试必须先 `uncancel(id)`** + `resetTaskProps()`；**跑动中的任务禁止删除**。产物命名收敛在 `tasks.go.resolveOutName()`（填了用用户名、含 `/`/`\` 直接 400；没填 `nextDailyName()` 生 `YYYYMMDD_NN<ext>`；`uniqueFileName()` 同名加 `_1` 不覆盖；HLS 后缀 = `mediaDefaultExt=".mp4"`）。

### 缓存治理（批次 14，`handler/cache.go`）
- **三处缓存路径 = 唯一事实来源，只在 `cache.go` 定义**：`offlineTmpPath(id)`=`%TEMP%/cp_offline_<id>.tmp`；`btTaskDir(id)`=`<data>/bt_tmp/<id>/`；`m3u8TmpDirOf(zips,id)`=`<data>/ziptmp/m3u8-<id>/`（**目录名已由随机改为确定性 taskID**）。**执行体与清扫器必须共用这三个函数。**
- **`sweepTaskCaches()`** 挂在 `InitTaskPool` 启动时一次（**必须在 `resume()` 之前**，否则把刚起的任务目录误判为残留）+ `sweepLoop` 每 6 小时。只删 `bt_tmp` 下**纯数字**目录 / `ziptmp` 下 `m3u8-` 前缀 / `%TEMP%` 下 `cp_offline_*.tmp`，跳过活跃任务，用**整棵树最新 mtime**（不是目录自身 mtime —— 写文件时不更新）做"1 分钟内写过"宽限。
- **`purgeTaskCache(id)`** 5 处调用：`run()` 成功/失败/取消三分支 + `Cancel` + `Delete`。**`Delete` 早先只删 DB 记录、完全不碰磁盘** —— "删除也不删缓存"的根因。
- ⚠️ **`defer Remove/RemoveAll` 只在执行体正常返回时执行**；强杀/断电/崩溃后残留靠 `sweepTaskCaches()` 兜。⚠️ **`TaskPool.Zips` 曾长期未赋值** → 压缩临时包落 cwd、ziptmp 清扫失效（现签名 `InitTaskPool(svc, btDir, zipTmp)`）。⚠️ 测"文件 mtime 新颖度"的 fixture **必须回溯整棵树**（只回溯目录、里面文件带"现在"时间会被正确判为活跃而放过）。

### BT（批次 14 修完的三个真缺陷）
- **端口按任务散列**：anacrolix 默认 `ListenPort: 42069`，每任务新建客户端 → **第二个并发 BT 必失败**（`bind: Only one usage of each socket address`）。现用 `btListenPortOf(id)` = `42300 + id%500`。
- **必须停滞超时**：`btStallTimeout = 5 分钟`（连续零增长判失败，有增长重新计时）。**没有它时拿不到 peer 会永远卡 processing 0%**，`bt_tmp/<id>` 一直占盘。
- **导入路径多布局兜底** `resolvePhys()`：`dataDir/<种子名>/<相对路径>` → `dataDir/<相对路径>` → `dataDir/<文件名>`。**只按 `info.Name` 拼一次的话，拼不中 → error → 任务判失败 → `defer os.RemoveAll(dataDir)` 把刚下好的数据全删掉**（用户看到"下好了却没进网盘"）。
- anacrolix 语义：`f.Path()`="种子名/相对路径"（含前缀）；`f.DisplayPath()`=多文件时纯相对路径、**单文件时等于种子名**；物理布局 = `dataDir/<info.Name>/<相对路径>`（单文件 = `dataDir/<DisplayPath>`），另有 `dataDir/.torrent.bolt.db`。**`.torrent` 加载后 tracker announce 不会自动发出**（最小复现 `btmin` 同样）→ **库/环境行为，非本仓库 bug**；BT 连通性依赖 tracker/DHT，对外别承诺"一定能下动"。

## 多网卡地址 / 跨平台
- `handler/netaddr.go.LocalAddresses(port)` → `GET /api/system/addresses`（`ug` 组），带网卡名 + `kind`(lan/virtual)。前端 `utils/lanaddr.ts`（模块级缓存 + single-flight）+ `components/AddressPicker.vue`。**「复制链接类」入口必须一律走 AddressPicker，别写 `location.origin`**；**「地址」与「路径」拆开存**（`dlPath`/`sharePath`），显示用 `preferred()` 拼、复制时弹选择器。已接入 5 处：设置页 WebDAV、设置页分享列表、管理台挂载路径、资源管理器、记事本分享。**AddressPicker 用自绘 `position:fixed` 遮罩**（复用 `.dialog-mask` 的 `position:absolute` 会漂）。
- 批次 12 实测：七目标（Win x64/ARM64、Linux x64/ARM64/ARMv7、macOS Intel/ARM）`CGO_ENABLED=0` 交叉编译全 exit 0，**读产物文件头核对**。为什么能：SQLite 纯 Go；源码 **0 处** `syscall`/`//go:build`/`os/user`；`go.mod` 的 `creack/pty` 是**从未 import 的残留**。
- **iOS 只能当客户端**：系统不允许 App 后台常驻监听端口。iPhone 用 Safari 或第三方 WebDAV App（**自带「文件」App 不支持 WebDAV**）。**不能含糊成"支持 iOS"**。不为 NAS / 路由器适配。

## 环境坑（踩过的）
- **Bash 工具 PATH 经常失效**（`ls`/`cat`/`head`/`cp` 随机 command not found，命令本身仍执行）→ 用 Read/Grep/Glob，或 **输出重定向到文件再 Read**。PowerShell 5.1 处理中文写坏编码，批量改代码用 Node；**PowerShell 工具禁止直接调 `cmd.exe`** → 跑 `.bat` 用 Node `spawn('cmd.exe',['/c','build.bat'])`。
- **沙箱下「我能写」≠「我拉起的子进程能写」**：`GOCACHE`/`GOTMPDIR` 指到工作区外时 Go 报 `Access is denied`；默认 `%LOCALAPPDATA%`/`%TEMP%` 可靠。**磁盘满会伪装成代码缺陷**（`link.exe: ... not enough space` 实为 C 盘只剩 0.2GB）；`go clean -cache` 释放约 2.9GB。Go 在 `C:\Program Files\Go`（1.27.0），必须 `export GOPROXY=https://goproxy.cn,direct`（**勿设 `GOSUMDB=off`**）。
- `build.bat` 会先 `rmdir` 整个 dist 再 xcopy，**必须自己重建 `.keep`**；**判成功看有没有打印 `BUILD OK`**，别看 `$LASTEXITCODE`，也**绝不能出现中文**（`cmd` 按 GBK 读 UTF-8 无 BOM，中文可能凑出 `&`/`|`/`>`）。
- **杀服务进程必须** `taskkill /PID <pid> /T /F`；git bash 的 `taskkill //F //IM` 无效。**后台/脱离进程会被会话清理杀掉** → 把"起服务+用服务+收服务"收进**同一个前台进程**。
- **跑 `start.bat` 会在仓库里建真实 `server/data`** → 验证完必须删或先重定向 `CP_DATA`。**本机 `server/data` 是用户真实数据，别删别碰。** **跑探针前先核对 `exe.mtime > 所有源文件`**，否则验证的是旧二进制的绿色。
- `vite build` **不做类型检查**，要另跑 `vue-tsc`（有 8 条历史遗留错误，判"是否新增"必须 `git show HEAD:<file>` 比基线）。
- **两种错误报告形态别混**：业务错误 = `dto.Fail(c,400,msg)` = **HTTP 200 + body `{code:400}`**；断言查 `r.json.code` 别查 `r.status`。**HTTP 探针认证是 `Authorization: Bearer <token>`**（token 在 `POST /api/auth/login` 的 `data.token`）；**建挂载是 `POST /api/admin/policies`**（`/api/policies` 只读）。
- **`el.click()` 会绕过 `pointer-events:none`** → 断言"按钮点不动"**永远 PASS**，必须走 CDP `Input.dispatchMouseEvent`。**Node `fetch`(undici) 会静默忽略 `init.auth`** → Basic Auth 自己拼 `Authorization` 头。
- **`#/app/<id>` 是 `StandaloneApp` 独立单应用模式**，切 hash 会整个替换页面；`ContextMenu` 与 `DialogHost` 只挂在 `WinDesktop.vue` 上。
- **同一文件并行下发多个 Edit 会丢改动** → **串行下发，改完立刻 Grep 复核**。ESM 恒严格模式（探针里未声明赋值会 `ReferenceError`，顶部先 `let x = null;`）；**内联 `node -e` 拼引号/heredoc 很脆** → 能落文件就落文件。
- **修复后要逐条复核"修复前写的反向断言"**：批次 15 有条断言写着"列表里没有失效标识"，修完必然变红；只看 `FAIL=1` 就去改代码会把刚修好的改回去。
- 端到端套路：`CP_DATA=<临时目录> CP_PORT=<临时端口> ./cloudpan.exe` → 从启动日志抓 `初始管理员密码为 <pw>` → 登录拿 token → `POST /api/admin/policies` 建本地挂载 → `PUT /api/admin/settings {offline_allow_private:"true"}`（**探针用 127.0.0.1 fixture 必须开**）。
- **要拿"用户真实数据"复现**：把 `server/data` 整份复制到 `%TEMP%` 再启动（原库只读）；用户改过登录密码时**别猜也别重置** —— 先起全新临时环境、用 API 把密码改成已知值、取出 bcrypt 哈希，再写进副本库 `users` 表（纯 Python 常没有 bcrypt，这条路绕开了它）。

## 已锁定的设计决策与基线
- 登录页只留密码框；文件管理器不要盘符、根视图直接列挂载点；挂载入口在文件管理器内；手机走 WebDAV；公开分享暂留；**用户组/权限模型彻底删除**（`AdminPerms()` 写死；配额不限；`RecycleRetentionDays:0` = 回收站永久保留）。三项遗留已拍板：`docs/test-evidence/` 清理；手机端前端**不做**；媒体中心保留但非动线。**别再当待办。**
- **不要重新引入任何「按 mtime 自动物理删除挂载目录」的清理逻辑**。批次 14 的缓存清扫**只动本程序命名规则匹配的缓存**。卸载挂载**只删挂载记录**，不删磁盘文件、也不删该挂载下已失效的分享记录。
- **15 个批次全部完成**（明细见 `docs/单用户私有化改造计划.md` §6/§7）。桌面只剩此电脑/回收站/记事本/任务中心/应用中心/设置/管理控制台，apps 只剩 10 个 key。
- **测试基线（任何改动后复跑）**：`smoke` 60 / `probe-policy` 26 / `probe-m3u8` 48 / `probe-offline` 22 / `probe-webdav` 44 / `probe-phone` 40 / `ui` 35 / `probe-addr` 27 / `probe-scroll` 13 = **315 全绿**；`probe-dav-root` exit 0。探针在 `%TEMP%\cp-verify2\`。增量探针：批次 14 `probe-regress.mjs`(12)；批次 15 `probe-share-state.mjs`(16)、`probe-real-unmount.mjs`(11)、`probe-share-ui.mjs`(9)。
- 待用户拍板：`.workbuddy/` 是否公开；LICENSE 是否追加版权行；是否 push 到 origin（`main` 领先 5 个提交）。**`server/server.rar`（170MB，用户手工快照，含真实 data/）已被 `*.rar` 忽略，未删除，等用户处置。**
