#!/bin/bash
# CloudPan 启动脚本（Linux / macOS）
#
# 支持两种摆放方式：
#   1) 发布包   —— cloudpan 与本脚本同级（out/ 目录）
#   2) 源码树   —— cloudpan 在 ./server/ 下（跑完 build.sh 后）
#
# 工作目录决定 ./data 落在哪。从别处启动会得到一套全新的空数据库和一把新的
# secret.key，界面看起来就像"文件全没了"（其实还在旧目录里）。
# 所以先定位到二进制所在目录，再启动。
set -e
cd "$(dirname "$0")"

APPDIR=""
if [ -f ./cloudpan ]; then
  APPDIR="$PWD"
elif [ -f ./server/cloudpan ]; then
  APPDIR="$PWD/server"
else
  echo "找不到 cloudpan。" >&2
  echo "请先运行 ./build.sh，或把本脚本与 cloudpan 放在同一个目录下。" >&2
  exit 1
fi

cd "$APPDIR"

if [ ! -x ./cloudpan ]; then
  echo "./cloudpan 没有执行权限，正在修复…"
  chmod +x ./cloudpan
fi

echo "CloudPan 启动中: http://localhost:18322   （Ctrl+C 退出）"
echo "管理员账号: admin  ——  初始密码只在下面的启动日志里打印一次，请及时保存"
echo "数据目录:   $APPDIR/data"
echo
exec ./cloudpan
