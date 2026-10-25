#!/bin/sh
# install.sh：awem 一键安装（POSIX sh，兼容 dash/busybox；T11-F2）。
# 从 GitHub Releases 下载指定版本产物、校验 sha256、安装到 ~/.local/bin。
# 用法：
#   curl -fsSL <repo>/install.sh | sh                          # 最新版
#   curl -fsSL <repo>/install.sh | sh -s -- v0.1.0             # 指定版本
#   curl -fsSL <repo>/install.sh | sh -s -- v0.1.0 <ghproxy镜像前缀>  # 镜像加速
set -eu

REPO="mantougg/awem"
VERSION="${1:-latest}"
MIRROR="${2:-}" # 如 https://ghproxy.example.com/https://github.com（空=直连）

OS="$(uname -s | tr '[:upper:]' '[:lower:]')"
ARCH="$(uname -m)"
case "$ARCH" in
  x86_64|amd64) ARCH=amd64 ;;
  aarch64|arm64) ARCH=arm64 ;;
  *) echo "不支持的架构：$ARCH（支持 amd64/arm64）"; exit 1 ;;
esac
case "$OS" in
  linux|darwin) ;;
  *) echo "不支持的系统：$OS（本脚本用于 Linux/macOS；Windows 请用 install.ps1）"; exit 1 ;;
esac

ASSET="awem-$OS-$ARCH"
BASE_URL="https://github.com/$REPO/releases/download"
if [ "$VERSION" = "latest" ]; then
  # 解析 latest release tag（失败则提示手动指定版本）
  VERSION="$(curl -fsSL "https://api.github.com/repos/$REPO/releases/latest" \
    | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1)" || VERSION=""
  if [ -z "$VERSION" ]; then
    echo "无法解析最新版本（网络/限流）；请显式指定：sh install.sh v0.1.0"
    exit 1
  fi
fi
URL_PREFIX="${MIRROR}${BASE_URL}/$VERSION"

BIN_DIR="${HOME}/.local/bin"
TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

echo ">> 下载 $URL_PREFIX/$ASSET"
curl -fSL "$URL_PREFIX/$ASSET" -o "$TMP/$ASSET"
echo ">> 下载 $URL_PREFIX/checksums.txt"
curl -fsSL "$URL_PREFIX/checksums.txt" -o "$TMP/checksums.txt" || {
  echo "校验和下载失败，拒绝安装（完整性优先）"; exit 1; }

echo ">> 校验 sha256"
want="$(grep " $ASSET\$" "$TMP/checksums.txt" | awk '{print $1}')"
if [ -z "$want" ]; then echo "checksums.txt 中无 $ASSET 条目，拒绝安装"; exit 1; fi
if command -v sha256sum >/dev/null 2>&1; then
  got="$(sha256sum "$TMP/$ASSET" | awk '{print $1}')"
else # macOS 无 sha256sum 时用 shasum
  got="$(shasum -a 256 "$TMP/$ASSET" | awk '{print $1}')"
fi
if [ "$got" != "$want" ]; then
  echo "sha256 不匹配：期望 $want，实际 $got —— 拒绝安装（下载可能损坏或被篡改）"
  exit 1
fi

mkdir -p "$BIN_DIR"
chmod +x "$TMP/$ASSET"
mv "$TMP/$ASSET" "$BIN_DIR/awem"

echo ">> 已安装 $BIN_DIR/awem（$VERSION）"
case ":$PATH:" in
  *":$BIN_DIR:"*) ;;
  *) echo "提示：$BIN_DIR 不在 PATH，可加入 shell 配置：export PATH=\"\$HOME/.local/bin:\$PATH\"" ;;
esac
"$BIN_DIR/awem" version
