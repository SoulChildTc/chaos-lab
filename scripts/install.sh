#!/bin/sh
# chaoslab installer.
#
#   curl -fsSL https://raw.githubusercontent.com/SoulChildTc/chaos-lab/main/scripts/install.sh | sh
#
# Overrides:
#   CHAOSLAB_VERSION      install a specific tag, e.g. v0.1.0 (default: latest)
#   CHAOSLAB_INSTALL_DIR  where the binary goes (default: /usr/local/bin, or
#                         ~/.local/bin when /usr/local/bin is not writable)
#   CHAOSLAB_SHARE_DIR    where the bundled plugins are unpacked
#                         (default: ~/.local/share/chaoslab)
set -eu

repo="SoulChildTc/chaos-lab"
version="${CHAOSLAB_VERSION:-latest}"
install_dir="${CHAOSLAB_INSTALL_DIR:-}"

say() { printf '%s\n' "$*" >&2; }
die() { say "错误：$*"; exit 1; }

os=$(uname -s | tr '[:upper:]' '[:lower:]')
case "$os" in
  darwin | linux) ;;
  *) die "不支持的系统：${os}（Windows 请在 WSL 里用 Linux 版）" ;;
esac

arch=$(uname -m)
case "$arch" in
  x86_64 | amd64) arch=amd64 ;;
  arm64 | aarch64) arch=arm64 ;;
  *) die "不支持的架构：${arch}" ;;
esac

asset="chaoslab_${os}_${arch}.tar.gz"
if [ "$version" = latest ]; then
  base="https://github.com/${repo}/releases/latest/download"
else
  base="https://github.com/${repo}/releases/download/${version}"
fi

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

say "下载 ${asset}"
curl -fsSL "${base}/${asset}" -o "${tmp}/${asset}" || die "下载失败：${base}/${asset}"
curl -fsSL "${base}/SHA256SUMS" -o "${tmp}/SHA256SUMS" || die "下载 SHA256SUMS 失败"

expected=$(grep " ${asset}\$" "${tmp}/SHA256SUMS" | cut -d' ' -f1 || true)
[ -n "$expected" ] || die "SHA256SUMS 里找不到 ${asset}"

if command -v shasum >/dev/null 2>&1; then
  actual=$(shasum -a 256 "${tmp}/${asset}" | cut -d' ' -f1)
elif command -v sha256sum >/dev/null 2>&1; then
  actual=$(sha256sum "${tmp}/${asset}" | cut -d' ' -f1)
else
  die "需要 shasum 或 sha256sum 才能校验下载内容"
fi

if [ "$expected" != "$actual" ]; then
  die "校验和不匹配：期望 ${expected}，实际 ${actual}"
fi
say "校验和正确"

tar -xzf "${tmp}/${asset}" -C "$tmp"

if [ -z "$install_dir" ]; then
  if [ -w /usr/local/bin ]; then
    install_dir=/usr/local/bin
  else
    install_dir="$HOME/.local/bin"
  fi
fi
mkdir -p "$install_dir"
mv "${tmp}/chaoslab" "${install_dir}/chaoslab"
chmod 755 "${install_dir}/chaoslab"

share="${CHAOSLAB_SHARE_DIR:-$HOME/.local/share/chaoslab}"
mkdir -p "$share"
rm -rf "${share}/plugins"
cp -R "${tmp}/plugins" "${share}/plugins"

say ""
say "已安装 $("${install_dir}/chaoslab" version)"
case ":${PATH}:" in
  *":${install_dir}:"*) ;;
  *)
    say ""
    say "注意：${install_dir} 不在 PATH 里，加一行到 shell 配置："
    say "    export PATH=\"${install_dir}:\$PATH\""
    ;;
esac

say ""
say "插件已解压到 ${share}/plugins，装一个再开始："
say "    chaoslab plugin install ${share}/plugins/k8s"
