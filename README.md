# CloudPan

**基于 [johngko/cloudpan](https://github.com/johngko/cloudpan) 修改的单用户私有网盘 —— Go + Vue，单文件部署**

![CloudPan 桌面](docs/screenshots/win12-02-desktop.png)

---

## 这是什么

CloudPan 是一个用 **Go（Gin + GORM + SQLite）** 和 **Vue 3 + TypeScript + Vite** 写的私有网盘。前端构建产物通过 `go:embed` 打进 Go 二进制，**一个 exe 就能跑** —— 不需要 Docker，不需要额外装数据库。

本项目**基于 [johngko/cloudpan](https://github.com/johngko/cloudpan) 修改而来**，在此向原作者致谢。上游是一个功能完整的通用网盘；本仓库是在它基础上做减法得到的**个人自用版**。

## 面向对象

只面向一种场景：**在自己的机器上跑，给自己用。**

- **单用户**：只有 `admin` 一个账号，登录只输密码；没有注册、没有用户管理、没有权限分级
- **内网自用**：家里或办公室局域网访问，手机走 WebDAV
- **把本机文件夹挂进网盘**：文件本来就躺在你的硬盘上，网盘只是给它们一个界面和一个统一入口

**不适合**：多用户协作、团队共享、公网多租户服务、企业权限体系。这些不是"还没做"，而是**刻意不做**。本项目按 MIT 正常开源，但设计目标始终只有一个：个人自用。

## 我们的原则

上游是个大而全的项目，本分支只保留"自己用得上"的部分：

1. **自己用** —— 只解决一个人的问题，不为假想的需求写代码
2. **可控** —— 功能少而明确，出问题看得懂、修得动；不引入黑盒依赖
3. **不复杂** —— 能删的都删。多用户体系、游客登录、站内共享、云盘挂载、macOS / Deepin 主题，以及一批用不到的内置应用，均已整体移除

体现在具体选择上：存储策略**只剩"本机磁盘目录"一种**（云盘驱动整包删除）；回收站**永久保留、不自动清理**；任务阶段提示与失败原因**分成两个字段**。目标都是"少一点意外"。

## 功能

**桌面外壳**

- Windows 12 概念风格：开机动画 → 登录 → 桌面 → 可拖拽窗口 → 开始菜单 / 全局搜索 / 控制中心
- 深浅色主题、通知中心、传输浮窗（多任务并发，可暂停 / 继续 / 取消）、全局右键菜单
- 内置应用 11 个：文件资源管理器、此电脑、回收站、记事本、图片查看器、媒体播放器、媒体中心（可安装）、任务中心、应用中心、设置、管理控制台

**挂载本机文件夹（核心动线）**

- 文件管理器根视图**直接列出已挂载的文件夹**，不显示盘符
- 「挂载文件夹」对话框内嵌本机目录浏览器，整盘可翻，选中即挂载；右键可卸载
- 路径必须**绝对**且**已存在**，不存在会被拒绝，不会静默创建

**网盘核心**

- **上传**：分块上传 + 断点续传 + SHA-256 秒传（硬链接去重）
- **下载**：单文件直下；多选 / 目录 zip 流式打包；图片、视频、音频 Range 预览
- **分享**：提取码、有效期、下载次数、端到端加密分享
- **回收站**：还原 / 彻底删除 / 清空（永久保留，无自动清理）
- **其他**：zip 压缩解压、版本管理、缩略图、全局搜索、审计日志
- **离线下载**：HTTP(S) 直链、BT 磁力、m3u8（HLS）。服务器代下载入网盘，任务队列重启自动恢复，SSRF 三层防护
  - 任务可看**详情**（原始链接、保存位置、分片进度、失败原因）、可**重试**、可**删除**
  - 文件名可自定义；不填则自动命名 `YYYYMMDD_NN.mp4`（当天从 `_01` 递增），同名不覆盖
- **WebDAV**：**一个统一入口 `/dav/` 就是全部挂载**，不用一个个加；单个挂载也可单独挂 `/dav/<挂载名>/`
  - 使用**独立的 WebDAV 密码**（在「设置 → WebDAV 独立密码」里设置，与登录密码是两回事）
  - 手机端填：地址 `http://<本机内网IP>:18322/dav/`、账号 `admin`、密码＝那个独立密码
  - `/dav/` 是只读的索引层，要传文件请进到 `/dav/<挂载名>/` 里面

**管理控制台**：仪表盘、存储策略、任务监控、站点设置、审计日志

## 快速开始

```bat
:: Windows（需 Go 1.27+ 与 Node.js 18+）
build.bat      :: 构建前端并嵌入，产出 server\cloudpan.exe
start.bat      :: 启动，浏览器访问 http://localhost:18322
```

```bash
# Linux / macOS
./build.sh && ./server/cloudpan
```

1. 浏览器打开 `http://localhost:18322`
2. 账号固定 `admin`；**初始密码在首次启动时随机生成，只打印一次到启动日志**，请立刻记下，登录后在 **设置 → 账号** 修改
3. 打开文件管理器 → 右上角「**挂载文件夹**」→ 选一个本机文件夹 → 确认，回到根视图即可看到它

**截图**

| 登录（只有密码框） | 桌面 |
|---|---|
| ![](docs/screenshots/win12-01-login.png) | ![](docs/screenshots/win12-02-desktop.png) |

| ① 根视图：直接列出挂载的文件夹，无盘符 | ② 挂载文件夹：内嵌本机目录浏览器 | ③ 进入挂载点：磁盘上的真实内容 |
|---|---|---|
| ![](docs/screenshots/win12-03-root.png) | ![](docs/screenshots/win12-04-mount.png) | ![](docs/screenshots/win12-05-browse.png) |

## 常用配置

| 环境变量 | 默认值 | 说明 |
|---|---|---|
| `CP_PORT` | `18322` | 监听端口 |
| `CP_DATA` | `./data` | 数据目录。**建议设成绝对路径** |
| `CP_PUBLIC_URL` | `http://localhost:<端口>` | 对外可达地址 |
| `CP_TRUSTED_PROXIES` | 空 | 部署在反向代理之后时声明代理网段（逗号分隔 CIDR/IP）。默认**不信任任何** `X-Forwarded-For` |

- 数据目录默认 `server/data/`，里面有 `cloudpan.db`、`cloudpan.db-wal`、`cloudpan.db-shm`、`secret.key`、`recycle/`、`uploads/`、`thumbs/`。**备份请把这几样一起拷** —— 只拷 `cloudpan.db` 会丢掉最近的事务
- **开机自启**：Windows 用 NSSM 注册成服务（`nssm install CloudPan <完整路径>\cloudpan.exe`，并在 `AppEnvironmentExtra` 里给 `CP_DATA` 一个绝对路径）；Linux 用 systemd，`Restart=always`
- **开发模式**：`cd web && npm install && npm run dev`（Vite 5173，`/api` 代理到 18322）；另开一个终端 `cd server && go run .`
- 历史改动记录见 [docs/CHANGELOG.md](docs/CHANGELOG.md)

## 许可与致谢

- 本项目**基于 [johngko/cloudpan](https://github.com/johngko/cloudpan) 修改**，沿用其 **MIT** 许可（见 [LICENSE](LICENSE)）。感谢原作者的开源
- 壁纸照片来自 Unsplash 免费授权
- Windows / Windows 11、12 是 Microsoft 的商标。本项目为独立实现，与 Microsoft 无任何关联
