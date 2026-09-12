#!/bin/bash
# CloudPan 启动脚本（Linux / macOS）
#
# 与 start.bat 行为一致：先把工作目录切到 server/，再启动二进制。
# 这一步不是可有可无的——数据目录默认取"当前工作目录下的 ./data"，
# 从别处启动会得到一套全新的空数据库和一把新的 secret.key，
# 界面看起来就像"文件全没了"（其实还在旧目录里）。
set -e
cd "$(dirname "$0")/server"

if [ ! -f ./cloudpan ]; then
  echo "找不到 ./cloudpan，请先运行 ./build.sh" >&2
  exit 1
fi
if [ ! -x ./cloudpan ]; then
  echo "./cloudpan 没有执行权限，正在修复…"
  chmod +x ./cloudpan
fi

echo "CloudPan 启动中: http://localhost:18322   （Ctrl+C 退出）"
echo "管理员账号: admin  ——  初始密码只在下面的启动日志里打印一次，请及时保存"
echo
exec ./cloudpan
