#!/bin/bash
# macOS .app 打包脚本
# 用法: ./scripts/package-darwin.sh <输出目录> <二进制名称>
# 例如: ./scripts/package-darwin.sh dist nexusagent-darwin-amd64

set -euo pipefail

OUT_DIR="${1:-dist}"
BIN_NAME="${2:-nexusagent-darwin-amd64}"
APP_NAME="NexusAgent"
APP_DIR="$OUT_DIR/$APP_NAME.app"
CONTENTS_DIR="$APP_DIR/Contents"
MACOS_DIR="$CONTENTS_DIR/MacOS"
RESOURCES_DIR="$CONTENTS_DIR/Resources"

# 清理旧包
rm -rf "$APP_DIR"

# 创建目录结构
mkdir -p "$MACOS_DIR" "$RESOURCES_DIR"

# 复制二进制
cp "$OUT_DIR/$BIN_NAME" "$MACOS_DIR/nexusagent"
chmod +x "$MACOS_DIR/nexusagent"

# 生成 Info.plist
cat > "$CONTENTS_DIR/Info.plist" << EOF
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleExecutable</key>
	<string>nexusagent</string>
	<key>CFBundleIdentifier</key>
	<string>com.nexusagent.app</string>
	<key>CFBundleName</key>
	<string>NexusAgent</string>
	<key>CFBundleDisplayName</key>
	<string>NexusAgent</string>
	<key>CFBundleVersion</key>
	<string>${GITHUB_REF_NAME:-1.0.0}</string>
	<key>CFBundleShortVersionString</key>
	<string>${GITHUB_REF_NAME:-1.0.0}</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>LSMinimumSystemVersion</key>
	<string>12.0</string>
	<key>NSHighResolutionCapable</key>
	<true/>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
</dict>
</plist>
EOF

# 生成启动脚本（先启动服务，再打开浏览器）
cat > "$MACOS_DIR/launcher" << 'SCRIPT'
#!/bin/bash
SCRIPT_DIR="$(cd "$(dirname "$0")" && pwd)"
BINARY="$SCRIPT_DIR/nexusagent"
DATA_DIR="$HOME/Library/Application Support/NexusAgent"
mkdir -p "$DATA_DIR"

# 启动服务（后台），检测到服务就绪后打开浏览器
"$BINARY" --open --data-dir "$DATA_DIR" &

# 等一小会儿让服务启动
sleep 1

# 打开默认浏览器访问本地服务
open "http://127.0.0.1:8080"

# 等待服务退出
wait
SCRIPT

chmod +x "$MACOS_DIR/launcher"

# 创建图标（用命令行工具生成占位图标，或使用内置图标）
# 如果项目目录有 icon.png，转换为 icns，否则跳过
if [ -f "web/public/icon.png" ]; then
  ICON_DIR="$RESOURCES_DIR"
  # 简单处理: 使用 sips 生成多种尺寸
  for size in 16 32 64 128 256 512; do
    sips -z $size $size "web/public/icon.png" --out "$ICON_DIR/icon_${size}x${size}.png" >/dev/null 2>&1 || true
  done
  # 生成 icns（需要 iconutil，仅在 macOS 可用）
  if command -v iconutil &>/dev/null; then
    ICONSET_DIR="$RESOURCES_DIR/AppIcon.iconset"
    mkdir -p "$ICONSET_DIR"
    for size in 16 32 64 128 256 512; do
      sips -z $size $size "web/public/icon.png" --out "$ICONSET_DIR/icon_${size}x${size}.png" >/dev/null 2>&1 || true
    done
    iconutil -c icns "$ICONSET_DIR" -o "$RESOURCES_DIR/AppIcon.icns" 2>/dev/null || true
    rm -rf "$ICONSET_DIR"
  fi
fi

# 修改启动脚本为入口点
# 将 Info.plist 指向 launcher 脚本，使其先启动服务再打开浏览器
# 注: 默认入口是 launcher，二进制被 launcher 内部调用

echo "✅ macOS .app 已创建: $APP_DIR"
echo "   双击 $APP_DIR 即可启动，服务数据保存在 ~/Library/Application Support/NexusAgent/"
