#!/bin/bash
# macOS .app 打包脚本（Pake 桌面客户端方案）
#
# 将 Go 后端 binary + Pake 桌面客户端组合成一个 macOS 应用包:
#
#   NexusAgent.app/
#     Contents/
#       Info.plist          → 指向 launcher 脚本
#       MacOS/
#         launcher          → 启动脚本（启动后端 + 打开 Pake 客户端）
#       Resources/
#         nexusagent        → Go 后端 binary
#         NexusAgent-Client.app → Pake 打包的桌面客户端
#
# 用法: ./scripts/package-darwin.sh <输出目录> <二进制名称> [Pake客户端目录]
# 例如: ./scripts/package-darwin.sh dist nexusagent-darwin-arm64 dist

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
OUT_DIR="${1:-dist}"
BIN_NAME="${2:-nexusagent-darwin-arm64}"
PAKE_DIR="${3:-dist}"
APP_NAME="NexusAgent"
CLIENT_NAME="NexusAgent-Client"
APP_VERSION="${GITHUB_REF_NAME:-${VERSION:-1.0.0}}"

APP_DIR="$OUT_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

# ========== 检查 ==========

if [ ! -f "$OUT_DIR/$BIN_NAME" ]; then
  echo "❌ 未找到后端 binary: $OUT_DIR/$BIN_NAME"
  exit 1
fi

if [ ! -d "$PAKE_DIR/$CLIENT_NAME.app" ]; then
  echo "❌ 未找到 Pake 客户端: $PAKE_DIR/$CLIENT_NAME.app"
  echo "   请先运行: ./scripts/build-pake.sh $PAKE_DIR"
  exit 1
fi

# ========== 创建目录结构 ==========

echo "==> 创建 macOS 应用包结构..."
rm -rf "$APP_DIR"
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

# ========== 复制后端 binary ==========

cp "$OUT_DIR/$BIN_NAME" "$MACOS_DIR/nexusagent"
chmod +x "$MACOS_DIR/nexusagent"

# 复制 config.yaml 到 Resources（后端需要配置文件）
if [ -f "$SCRIPT_DIR/config.yaml" ]; then
  cp "$SCRIPT_DIR/config.yaml" "$RESOURCES_DIR/config.yaml"
fi

# 复制前端构建产物到 Resources/web（release 模式下后端需要 serve 前端）
if [ -d "$SCRIPT_DIR/web/dist" ]; then
  echo "==> 复制前端构建产物..."
  cp -R "$SCRIPT_DIR/web/dist" "$RESOURCES_DIR/web"
else
  echo "⚠️  未找到前端构建产物 $SCRIPT_DIR/web/dist，请先运行 make build"
fi

# ========== 编译 launcher 二进制 wrapper ==========
# macOS 的 .app bundle 要求 CFBundleExecutable 是 Mach-O 二进制，不能是 shell 脚本。
# launcher.c 编译出一个小 wrapper，它会 exec 同目录的 launcher.sh。

echo "==> 编译 launcher wrapper..."
clang -O2 -o "$MACOS_DIR/launcher" "$SCRIPT_DIR/scripts/launcher.c" || {
  echo "❌ 编译 launcher 失败（需要 Xcode Command Line Tools）"
  exit 1
}

# ========== 复制 Pake 客户端 ==========

cp -R "$PAKE_DIR/$CLIENT_NAME.app" "$RESOURCES_DIR/"

# ========== 生成 Info.plist ==========

cat > "$CONTENTS_DIR/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>launcher</string>
	<key>CFBundleIdentifier</key>
	<string>com.nexusagent.app</string>
	<key>CFBundleName</key>
	<string>NexusAgent</string>
	<key>CFBundleDisplayName</key>
	<string>NexusAgent</string>
	<key>CFBundleVersion</key>
	<string>${APP_VERSION}</string>
	<key>CFBundleShortVersionString</key>
	<string>${APP_VERSION}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSUIElement</key>
	<false/>
	<key>NSAppTransportSecurity</key>
	<dict>
		<key>NSAllowsLocalNetworking</key>
		<true/>
	</dict>
</dict>
</plist>
EOF

# ========== 生成 launcher 启动脚本 ==========

cat > "$MACOS_DIR/launcher.sh" << 'SCRIPT'
#!/bin/bash
# NexusAgent macOS 启动器
# 负责启动后端服务并打开 Pake 桌面客户端

# 解析路径：launcher 和 nexusagent 都在 MacOS/ 目录，Pake 客户端在 Resources/
MACOS_DIR="$(cd "$(dirname "$0")" && pwd)"
RESOURCES_DIR="$(cd "$MACOS_DIR/../Resources" && pwd)"
BINARY="$MACOS_DIR/nexusagent"
CLIENT_APP="$RESOURCES_DIR/NexusAgent-Client.app"
DATA_DIR="$HOME/Library/Application Support/NexusAgent"
LOG_FILE="$DATA_DIR/launcher.log"
PORT=8080

mkdir -p "$DATA_DIR"

# 所有输出重定向到日志文件（通过 open 启动时 stdout/stderr 不可见）
exec > "$LOG_FILE" 2>&1

echo "========================================"
echo "NexusAgent 启动于 $(date)"
echo "MACOS_DIR: $MACOS_DIR"
echo "RESOURCES_DIR: $RESOURCES_DIR"
echo "DATA_DIR: $DATA_DIR"
echo "========================================"

# ========== 端口占用检测 ==========

EXISTING_PID=$(lsof -ti:$PORT 2>/dev/null || true)
if [ -n "$EXISTING_PID" ]; then
  echo "检测到端口 $PORT 被占用 (PID: $EXISTING_PID)，正在关闭..."
  kill "$EXISTING_PID" 2>/dev/null || true
  sleep 1
fi

# ========== 启动后端服务 ==========
# 设置 CONFIG_PATH 指向 app bundle 内的 config.yaml

echo "启动 NexusAgent 后端服务..."
export CONFIG_PATH="$RESOURCES_DIR/config.yaml"
export SERVER_MODE=release
export WEB_DIST="$RESOURCES_DIR/web"
echo "CONFIG_PATH: $CONFIG_PATH"

"$BINARY" --data-dir "$DATA_DIR" &
BACKEND_PID=$!
echo "后端 PID: $BACKEND_PID"

cleanup() {
  echo "正在关闭 NexusAgent..."
  kill "$BACKEND_PID" 2>/dev/null || true
  wait "$BACKEND_PID" 2>/dev/null || true
  exit 0
}
trap cleanup SIGTERM SIGINT

# ========== 等待后端就绪 ==========

echo "等待后端服务就绪..."
READY=0
for i in $(seq 1 60); do
  if curl -sf "http://127.0.0.1:$PORT/health" >/dev/null 2>&1; then
    READY=1
    break
  fi
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "后端服务启动失败，退出码: $?"
    exit 1
  fi
  sleep 0.5
done

if [ "$READY" -eq 0 ]; then
  echo "后端服务启动超时"
  kill "$BACKEND_PID" 2>/dev/null || true
  exit 1
fi

echo "后端服务已就绪"

# ========== 启动 Pake 桌面客户端 ==========

echo "启动桌面客户端..."
if [ -d "$CLIENT_APP" ]; then
  open "$CLIENT_APP"
  echo "桌面客户端已启动"
else
  echo "未找到桌面客户端，回退到浏览器"
  open "http://127.0.0.1:$PORT"
fi

# ========== 监控客户端进程 ==========

echo "等待客户端退出..."
while true; do
  sleep 2
  # 后端进程异常退出
  if ! kill -0 "$BACKEND_PID" 2>/dev/null; then
    echo "后端服务已停止"
    break
  fi
  # Pake 客户端进程退出
  if ! pgrep -f "NexusAgent-Client" >/dev/null 2>&1; then
    echo "桌面客户端已退出"
    break
  fi
done

cleanup
SCRIPT

chmod +x "$MACOS_DIR/launcher.sh"

# ========== 生成应用图标 ==========

if [ -f "$SCRIPT_DIR/assets/icon.png" ]; then
  ICON_DIR="$CONTENTS_DIR/Resources"
  if command -v iconutil &>/dev/null; then
    ICONSET_DIR="$CONTENTS_DIR/AppIcon.iconset"
    mkdir -p "$ICONSET_DIR"
    for size in 16 32 64 128 256 512 1024; do
      half=$((size / 2))
      sips -z "$size" "$size" "$SCRIPT_DIR/assets/icon.png" \
        --out "$ICONSET_DIR/icon_${half}x${half}.png" >/dev/null 2>&1 || true
      sips -z "$size" "$size" "$SCRIPT_DIR/assets/icon.png" \
        --out "$ICONSET_DIR/icon_${half}x${half}@2x.png" >/dev/null 2>&1 || true
    done
    iconutil -c icns "$ICONSET_DIR" -o "$ICON_DIR/AppIcon.icns" 2>/dev/null || true
    rm -rf "$ICONSET_DIR"

    # 在 Info.plist 中添加图标引用
    if [ -f "$ICON_DIR/AppIcon.icns" ]; then
      /usr/libexec/PlistBuddy -c "Add :CFBundleIconFile string AppIcon" "$CONTENTS_DIR/Info.plist" 2>/dev/null || true
    fi
  fi
fi

echo ""
echo "✅ macOS 桌面应用已创建: $APP_DIR"
echo "   双击 $APP_NAME.app 启动，数据保存在 ~/Library/Application Support/NexusAgent/"
echo "   包含: Go 后端服务 + Pake 桌面客户端"
