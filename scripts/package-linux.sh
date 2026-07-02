#!/bin/bash
# Linux 桌面应用打包脚本
#
# 将 Go 后端 + Pake AppImage 客户端组合为可分发目录:
#   NexusAgent/
#     bin/nexusagent
#     bin/NexusAgent-Client*.AppImage
#     config.yaml
#     web/
#     launch.sh
#
# 用法: ./scripts/package-linux.sh <输出目录> <二进制名称> [Pake客户端目录]

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-dist}"
BIN_NAME="${2:-nexusagent-linux-amd64}"
PAKE_DIR="${3:-dist}"
APP_DIR="$OUT_DIR/NexusAgent"
CLIENT_NAME="NexusAgent-Client"

if [ ! -f "$OUT_DIR/$BIN_NAME" ]; then
  echo "❌ 未找到后端 binary: $OUT_DIR/$BIN_NAME"
  exit 1
fi

CLIENT_APP=$(find "$PAKE_DIR" -maxdepth 1 -name "${CLIENT_NAME}*.AppImage" -print -quit)
if [ -z "$CLIENT_APP" ]; then
  echo "❌ 未找到 Pake AppImage: $PAKE_DIR/${CLIENT_NAME}*.AppImage"
  echo "   请先运行: PAKE_TARGET=appimage ./scripts/build-pake.sh $PAKE_DIR"
  exit 1
fi

echo "==> 创建 Linux 桌面应用目录..."
rm -rf "$APP_DIR"
mkdir -p "$APP_DIR/bin"

cp "$OUT_DIR/$BIN_NAME" "$APP_DIR/bin/nexusagent"
chmod +x "$APP_DIR/bin/nexusagent"
cp "$CLIENT_APP" "$APP_DIR/bin/"
chmod +x "$APP_DIR/bin/"*.AppImage

if [ -f "$SCRIPT_DIR/config.yaml" ]; then
  cp "$SCRIPT_DIR/config.yaml" "$APP_DIR/config.yaml"
fi

if [ -d "$SCRIPT_DIR/web/dist" ]; then
  cp -R "$SCRIPT_DIR/web/dist" "$APP_DIR/web"
else
  echo "⚠️  未找到前端构建产物 web/dist"
fi

cat > "$APP_DIR/launch.sh" << 'SCRIPT'
#!/bin/bash
set -euo pipefail

ROOT="$(cd "$(dirname "$0")" && pwd)"
BINARY="$ROOT/bin/nexusagent"
CLIENT=$(find "$ROOT/bin" -maxdepth 1 -name 'NexusAgent-Client*.AppImage' -print -quit)
DATA_DIR="${XDG_DATA_HOME:-$HOME/.local/share}/NexusAgent"
LOG_FILE="$DATA_DIR/launcher.log"
PORT=8080

mkdir -p "$DATA_DIR"
exec > "$LOG_FILE" 2>&1

echo "NexusAgent 启动于 $(date)"

EXISTING_PID=$(lsof -ti:$PORT 2>/dev/null || true)
if [ -n "$EXISTING_PID" ]; then
  kill "$EXISTING_PID" 2>/dev/null || true
  sleep 1
fi

export CONFIG_PATH="$ROOT/config.yaml"
export SERVER_MODE=release
export WEB_DIST="$ROOT/web"

"$BINARY" --data-dir "$DATA_DIR" &
BACKEND_PID=$!

cleanup() {
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  exit 0
}
trap cleanup SIGTERM SIGINT

READY=0
for _ in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
    READY=1
    break
  fi
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "后端启动失败"
    exit 1
  fi
  sleep 0.5
done

if [ "$READY" -eq 0 ]; then
  echo "后端启动超时"
  cleanup
fi

if [ -n "$CLIENT" ]; then
  "$CLIENT" &
  CLIENT_PID=$!
  wait "$CLIENT_PID" 2>/dev/null || true
else
  xdg-open "http://127.0.0.1:$PORT" >/dev/null 2>&1 || true
  wait "$BACKEND_PID"
fi

cleanup
SCRIPT

chmod +x "$APP_DIR/launch.sh"

echo ""
echo "✅ Linux 桌面应用已创建: $APP_DIR"
echo "   运行: $APP_DIR/launch.sh"
