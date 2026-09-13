#!/bin/bash
# CloudPan 构建脚本（Linux/macOS）
set -e
cd "$(dirname "$0")"

# 纯 Go 构建（SQLite 驱动为 modernc 纯 Go 实现），便于跨发行版部署
export CGO_ENABLED=0

echo "[1/4] building frontend..."
cd web && npm run build && cd ..

echo "[2/4] embedding dist..."
rm -rf server/internal/web/dist
cp -r web/dist server/internal/web/dist
# 保留 embed 占位（.gitignore 排除 dist 内容，占位文件保证空克隆也能 go build）
touch server/internal/web/dist/.keep

echo "[3/4] building..."
# 用子 shell，避免把工作目录留在 server/ 里
( cd server && go build -o cloudpan . )

echo "[4/4] assembling out/ ..."
# 注意：这里不清空 out/。如果你已经在 out/ 里跑过，数据就在 out/data/，
# 重新构建不能把它删掉。
mkdir -p out
if [ -d out/data ]; then
  echo "      (out/data 已存在 —— 保留不动)"
fi
cp -f server/cloudpan       out/cloudpan
cp -f start.sh              out/start.sh
cp -f start.bat             out/start.bat
cp -f packaging/README.txt  out/README.txt
cp -f LICENSE               out/LICENSE
chmod +x out/cloudpan out/start.sh

# out/ 里的 .bat / .txt 统一成 CRLF：交付包是给 Windows 用户的（cmd + 记事本），
# 而这里的工作区可能是 LF 的（编辑器或任何重写文件的工具都会丢掉 CRLF，
# ".gitattributes eol=crlf" 只在 checkout 那一刻生效）。
for f in out/*.bat out/*.txt; do
  [ -f "$f" ] || continue
  awk '{ sub(/\r$/, ""); printf "%s\r\n", $0 }' "$f" > "$f.crlf" && mv "$f.crlf" "$f"
done

echo
echo "BUILD OK"
echo "  binary : server/cloudpan"
echo "  bundle : out/   (copy this whole folder anywhere, then run ./start.sh)"
