#!/bin/bash
# Pake 桌面客户端打包脚本
# 使用 Pake (基于 Tauri) 将 NexusAgent 封装为原生桌面客户端
# 客户端连接到本地后端服务 http://127.0.0.1:8080
#
# 依赖:
#   - Node.js >= 18 + pnpm
#   - Rust >= 1.85 (cargo)
#   - pake-cli (pnpm install -g pake-cli@3.13.0)
#
# 用法: ./scripts/build-pake.sh [输出目录] [应用版本] [目标]
# 目标: app (macOS .app) | appimage (Linux) | x64 (Windows)
# 例如: PAKE_TARGET=appimage ./scripts/build-pake.sh dist 1.0.0

set -euo pipefail

OUT_DIR="${1:-dist}"
APP_VERSION="${2:-1.0.0}"
PAKE_TARGET="${3:-${PAKE_TARGET:-}}"
APP_NAME="NexusAgent-Client"
URL="http://127.0.0.1:8080"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ICON_PATH="$SCRIPT_DIR/assets/icon.png"

# Tauri 要求 semver 格式（如 1.0.0），非 semver 版本号回退到 1.0.0
if ! echo "$APP_VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+'; then
  APP_VERSION="1.0.0"
fi

# 按当前平台推断 Pake 构建目标
if [ -z "$PAKE_TARGET" ]; then
  case "$(uname -s)" in
    Darwin) PAKE_TARGET="app" ;;
    Linux)  PAKE_TARGET="appimage" ;;
    MINGW*|MSYS*|CYGWIN*|Windows*) PAKE_TARGET="x64" ;;
    *) echo "❌ 无法推断 Pake 目标，请设置 PAKE_TARGET"; exit 1 ;;
  esac
fi

mkdir -p "$OUT_DIR"

# ========== 依赖检查 ==========

if ! command -v pake &>/dev/null; then
  echo "❌ 未找到 pake-cli"
  echo "   请先安装: pnpm install -g pake-cli"
  exit 1
fi

if ! command -v cargo &>/dev/null; then
  echo "❌ 未找到 Rust/Cargo"
  exit 1
fi

# ========== 图标准备 ==========

ICON_ARG=()
if [ -f "$ICON_PATH" ]; then
  echo "==> 使用自定义图标: $ICON_PATH"
  ICON_ARG=(--icon "$ICON_PATH")
else
  echo "⚠️  未找到图标文件 ($ICON_PATH)，Pake 将使用默认图标"
fi

# ========== Pake 打包 ==========

echo "==> 使用 Pake 打包桌面客户端 (v$APP_VERSION, target=$PAKE_TARGET)..."
echo "    URL: $URL"
echo "    应用名: $APP_NAME"

TMP_BUILD=$(mktemp -d)
cd "$TMP_BUILD"

PAKE_ARGS=(
  "$URL"
  --name "$APP_NAME"
  --width 1280
  --height 800
  --min-width 900
  --min-height 600
  --app-version "$APP_VERSION"
  --iterative-build
  --safe-domain "127.0.0.1,localhost"
  --force-internal-navigation
  --targets "$PAKE_TARGET"
)

if [ "$PAKE_TARGET" = "app" ]; then
  export PAKE_CREATE_APP=1
fi

pake "${PAKE_ARGS[@]}" "${ICON_ARG[@]}"

# 移动打包结果到输出目录
shopt -s nullglob
ARTIFACTS=(
  "${APP_NAME}.app"
  "${APP_NAME}"*.AppImage
  "${APP_NAME}.exe"
  "${APP_NAME}"*.exe
  *.AppImage
)

MOVED=0
for artifact in "${ARTIFACTS[@]}"; do
  for f in $artifact; do
    [ -e "$f" ] || continue
    rm -rf "$SCRIPT_DIR/$OUT_DIR/$(basename "$f")"
    mv "$f" "$SCRIPT_DIR/$OUT_DIR/"
    echo "✅ Pake 桌面客户端已生成: $OUT_DIR/$(basename "$f")"
    MOVED=1
    break 2
  done
done

cd "$SCRIPT_DIR"
rm -rf "$TMP_BUILD"

if [ "$MOVED" -eq 0 ]; then
  echo "❌ Pake 打包失败，未找到输出文件"
  exit 1
fi
