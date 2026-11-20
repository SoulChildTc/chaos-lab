#!/usr/bin/env bash
# End-to-end check of the k8s plugin: start every scenario, prove it is broken,
# stop it, prove nothing is left behind.
#
# Requires: kind, kubectl, docker, and a built chaoslab binary.
#   go build -o /tmp/chaoslab ./cmd/chaoslab
#   bash scripts/e2e-k8s.sh
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="${CHAOSLAB_BIN:-/tmp/chaoslab}"
[ -x "$bin" ] || { echo "先 build：go build -o $bin ./cmd/chaoslab" >&2; exit 1; }
for c in kind kubectl docker; do
  command -v "$c" >/dev/null || { echo "缺少 $c" >&2; exit 1; }
done

export CHAOSLAB_HOME="$(mktemp -d)"
trap '"$bin" stop --all >/dev/null 2>&1; kind get clusters 2>/dev/null | grep "^lab-" | while read -r c; do kind delete cluster --name "$c" >/dev/null 2>&1; done' EXIT

"$bin" plugin install "$root/plugins/k8s" || exit 1

fail=0
for c in ingress-502 pod-pending image-pull-backoff svc-selector-mismatch pvc-unbound; do
  echo "=== $c"
  if ! "$bin" start "k8s/$c"; then
    echo "FAIL: $c 启动失败"
    fail=1
    continue
  fi

  # Un-fixed must not pass.
  if "$bin" check "k8s/$c" >/dev/null 2>&1; then
    echo "FAIL: $c 未修复却判定通过"
    fail=1
  fi

  if ! "$bin" stop "k8s/$c"; then
    echo "FAIL: $c 停止失败"
    fail=1
  fi

  if kind get clusters 2>/dev/null | grep -q "^lab-$c-"; then
    echo "FAIL: $c 停止后仍有 kind 集群残留"
    kind get clusters | grep "^lab-$c-"
    fail=1
  fi
  echo "OK $c"
done

[ "$fail" -eq 0 ] && echo "全部通过"
exit "$fail"
