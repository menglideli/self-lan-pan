CloudPan  —— 单用户私有网盘
============================================================

这个文件夹里就是全部东西，直接跑，不需要安装任何依赖。


【怎么启动】

  Windows      双击 start.bat
  Linux/macOS  ./start.sh

  如果 ./start.sh 提示 Permission denied，先执行一次：

      chmod +x start.sh cloudpan

  （从 Windows 拷过来的文件会丢掉可执行权限，这是 Windows 的限制）

  然后浏览器打开  http://localhost:18322

  账号固定是 admin。初始密码在首次启动时随机生成，只打印一次
  在启动窗口里，请立刻记下。登录后到「设置 → 账号」改掉它。


【每个文件是干什么的】

  cloudpan.exe / cloudpan    主程序。网页界面已经打进这一个文件里了
  start.bat / start.sh       启动脚本
  README.txt                 本说明
  LICENSE                    MIT 许可

  data/                      第一次跑起来后自动生成，你的全部数据都在这里


【备份 / 迁移 —— 只有一件事】

  把整个文件夹拷走。就这样。

  data/ 就在本文件夹里，所以拷走文件夹 = 带走了数据和密钥。
  千万不要只拷主程序 —— 程序是程序，数据是数据。

  如果你只想备份数据，data/ 里这些必须一起拷，少一个都算不完整：

    cloudpan.db / cloudpan.db-wal / cloudpan.db-shm   数据库，含最近的事务
    secret.key                                        加密密钥，丢了分享链接会失效
    recycle/                                          回收站
    uploads/                                          未完成的上传分片
    thumbs/                                           缩略图缓存


【换端口 / 换数据目录】

  用环境变量，不用改配置文件：

    CP_PORT    监听端口，默认 18322
    CP_DATA    数据目录，默认 ./data（建议给绝对路径）

  注意：数据目录是相对「当前工作目录」算的。
  所以请通过 start.bat / start.sh 启动，它们会先切到正确目录。
  从别处启动会得到一套全新的空数据库，看起来就像「文件全没了」。


【想给别的系统用】

  主程序是纯 Go 静态编译的，不挑系统库。
  用交叉编译产出目标平台的二进制，覆盖掉这里的主程序即可：

    cd server
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o cloudpan .

  完整说明见仓库 README.md。


【提醒】

  · 默认监听本机所有网卡，局域网里的设备可以用 http://<本机内网IP>:18322 访问
  · 全站是明文 HTTP，不要把端口直接暴露到公网
  · 手机端走 WebDAV：地址 http://<本机内网IP>:18322/dav/
    账号 admin，密码是「设置 → WebDAV 独立密码」里单独设的那个，不是登录密码
