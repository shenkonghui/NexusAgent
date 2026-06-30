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
# 用法: ./scripts/build-pake.sh [输出目录] [应用版本]
# 例如: ./scripts/build-pake.sh dist 1.0.0

set -euo pipefail

OUT_DIR="${1:-dist}"
APP_VERSION="${2:-1.0.0}"
APP_NAME="NexusAgent-Client"
URL="http://127.0.0.1:8080"
SCRIPT_DIR="$(cd "$(dirname "$0")/.." && pwd)"
ICON_PATH="$SCRIPT_DIR/assets/icon.png"

# Tauri 要求 semver 格式（如 1.0.0），非 semver 版本号回退到 1.0.0
if ! echo "$APP_VERSION" | grep -qE '^[0-9]+\.[0-9]+\.[0-9]+'; then
  APP_VERSION="1.0.0"
fi

mkdir -p "$OUT_DIR"

# ========== 依赖检查 ==========

if ! command -v pake &>/dev/null; then
  echo "❌ 未找到 pake-cli"
  echo "   请先安装: pnpm install -g pake-cli"
  echo "   或使用 npm: npm install -g pake-cli"
  exit 1
fi

if ! command -v cargo &>/dev/null; then
  echo "❌ 未找到 Rust/Cargo"
  echo "   请先安装: curl --proto '=https' --tlsv1.2 -sSf https://sh.rustup.rs | sh"
  exit 1
fi

# ========== 图标准备 ==========

ICON_ARG=""
if [ -f "$ICON_PATH" ]; then
  echo "==> 使用自定义图标: $ICON_PATH"
  ICON_ARG="--icon"
  ICON_VAL="$ICON_PATH"
else
  echo "⚠️  未找到图标文件 ($ICON_PATH)，Pake 将使用默认图标"
fi

# ========== Pake 打包 ==========

echo "==> 使用 Pake 打包桌面客户端 (v$APP_VERSION)..."
echo "    URL: $URL"
echo "    应用名: $APP_NAME"

# Pake 需在临时目录中运行（会在当前目录生成 .app）
TMP_BUILD=$(mktemp -d)
cd "$TMP_BUILD"

# 构建 Pake 参数
PAKE_ARGS=(
  "$URL"
  "--name" "$APP_NAME"
  "--width" "1280"
  "--height" "800"
  "--min-width" "900"
  "--min-height" "600"
  "--app-version" "$APP_VERSION"
  "--iterative-build"
  "--safe-domain" "127.0.0.1,localhost"
  "--force-internal-navigation"
)

if [ -n "$ICON_ARG" ]; then
  PAKE_ARGS+=("$ICON_ARG" "$ICON_VAL")
fi

pake "${PAKE_ARGS[@]}"

# 移动打包结果到输出目录
OUTPUT_APP="${APP_NAME}.app"
if [ -d "$OUTPUT_APP" ]; then
  rm -rf "$SCRIPT_DIR/$OUT_DIR/$OUTPUT_APP"
  mv "$OUTPUT_APP" "$SCRIPT_DIR/$OUT_DIR/"
  echo "✅ Pake 桌面客户端已生成: $OUT_DIR/$OUTPUT_APP"
else
  echo "❌ Pake 打包失败，未找到 $OUTPUT_APP"
  echo "   Pake 输出目录内容:"
  ls -la "$TMP_BUILD" || true
  cd "$SCRIPT_DIR"
  rm -rf "$TMP_BUILD"
  exit 1
fi

cd "$SCRIPT_DIR"
rm -rf "$TMP_BUILD"
