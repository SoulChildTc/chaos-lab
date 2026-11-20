#!/bin/sh
# chaoslab installer.
#
#   install / upgrade:
#     curl -fsSL https://raw.githubusercontent.com/SoulChildTc/chaos-lab/main/scripts/install.sh | sh
#   uninstall:
#     curl -fsSL https://raw.githubusercontent.com/SoulChildTc/chaos-lab/main/scripts/install.sh | sh -s -- --uninstall
#   show usage:
#     curl -fsSL .../install.sh | sh -s -- --help
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
action="install"

for arg in "$@"; do
  case "$arg" in
    --uninstall) action="uninstall" ;;
    --help | -h) action="help" ;;
    *) printf '未知参数：%s（用 --help 查看用法）\n' "$arg" >&2; exit 1 ;;
  esac
done

say() { printf '%s\n' "$*" >&2; }
die() { say "错误：$*"; exit 1; }

if [ "$action" = help ]; then
  sed -n '2,15p' "$0"
  exit 0
fi

# /usr/local/bin may be root-owned, so don't assume it's writable.
resolve_install_dir() {
  if [ -n "$install_dir" ]; then
    printf '%s' "$install_dir"
    return
  fi
  if [ -w /usr/local/bin ]; then
    printf '/usr/local/bin'
  else
    printf '%s' "$HOME/.local/bin"
  fi
}

if [ "$action" = uninstall ]; then
  bin_dir="$(resolve_install_dir)"
  bin="$bin_dir/chaoslab"
  share="${CHAOSLAB_SHARE_DIR:-$HOME/.local/share/chaoslab}"
  lab_home="${CHAOSLAB_HOME:-$HOME/.lab}"

  # Refuse while instances are live: uninstalling mid-run would orphan a real
  # environment that the user can no longer clean up.
  if [ -x "$bin" ]; then
    if "$bin" status --json 2>/dev/null | grep -q '"live":true'; then
      die "还有存活的实例，先 chaoslab stop --all，再卸载"
    fi
  fi

  [ -f "$bin" ] && rm -f "$bin" && say "已删除 ${bin}"
  [ -d "$share" ] && rm -rf "$share" && say "已删除 ${share}"
  if [ -d "$lab_home" ]; then
    rm -rf "$lab_home"
    say "已删除 ${lab_home}（含状态与运行记录）"
  fi
  say "完成"
  exit 0
fi

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

# GitHub's "latest" skips prereleases, so during the beta a releases/latest URL
# resolves to nothing. Ask the API for the newest release instead, including
# prereleases. If that fails (rate limit, no curl), fall back to the plain URL.
if [ "$version" = latest ]; then
  tag=$(curl -fsSL "https://api.github.com/repos/${repo}/releases" 2>/dev/null \
        | sed -n 's/.*"tag_name": *"\([^"]*\)".*/\1/p' | head -1 || true)
  if [ -n "$tag" ]; then
    version="$tag"
    say "最新版本：${version}"
  else
    base="https://github.com/${repo}/releases/latest/download"
  fi
fi
[ -n "${base:-}" ] || base="https://github.com/${repo}/releases/download/${version}"

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

bin_dir="$(resolve_install_dir)"
mkdir -p "$bin_dir"
new_bin="${tmp}/chaoslab"

# Re-running the script is the upgrade path. Show both versions so the user can
# tell whether anything changed.
if [ -x "${bin_dir}/chaoslab" ]; then
  old="$("${bin_dir}/chaoslab" version 2>/dev/null || true)"
  say "当前已装：${old:-无法识别}"
fi
mv "$new_bin" "${bin_dir}/chaoslab"
chmod 755 "${bin_dir}/chaoslab"
say "本次安装：$("${bin_dir}/chaoslab" version)"

share="${CHAOSLAB_SHARE_DIR:-$HOME/.local/share/chaoslab}"
mkdir -p "$share"
rm -rf "${share}/plugins"
cp -R "${tmp}/plugins" "${share}/plugins"

case ":${PATH}:" in
  *":${bin_dir}:"*) ;;
  *)
    say ""
    say "注意：${bin_dir} 不在 PATH 里，加一行到 shell 配置："
    say "    export PATH=\"${bin_dir}:\$PATH\""
    ;;
esac

say ""
say "插件已解压到 ${share}/plugins，装一个再开始："
say "    chaoslab plugin install ${share}/plugins/k8s"
say "卸载：curl -fsSL <同一 URL> | sh -s -- --uninstall"
