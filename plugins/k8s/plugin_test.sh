#!/usr/bin/env bash
# Protocol-level self-test for the k8s plugin. Does NOT create clusters: that is
# the end-to-end script's job, because it needs kind and network access.
set -uo pipefail

here="$(cd "$(dirname "$0")" && pwd)"
tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
export CHAOSLAB_HOME="$tmp"
fail=0

chk() { if ! "$@" >/dev/null 2>&1; then echo "FAIL: $*"; fail=1; fi; }
chk_fail() { if "$@" >/dev/null 2>&1; then echo "FAIL (expected non-zero): $*"; fail=1; fi; }

# schema
schema="$("$here/plugin" schema --json)"
echo "$schema" | grep -q '"name":"k8s"' || { echo "FAIL: schema name"; fail=1; }
echo "$schema" | grep -q 'node-image' || { echo "FAIL: node-image arg"; fail=1; }
echo "$schema" | grep -q 'kubeconfig' || { echo "FAIL: kubeconfig arg"; fail=1; }

# list: five cases, ids matching the directories
list="$("$here/plugin" list --json --instance=k8s-x-1)"
echo "$list" | grep -q '"kind:lab-k8s-x-1"' || { echo "FAIL: kind claim"; fail=1; }
count="$(echo "$list" | grep -o '"id":"[^"]*"' | wc -l | tr -d ' ')"
[ "$count" = "5" ] || { echo "FAIL: want 5 cases, got $count"; fail=1; }
for id in ingress-502 pod-pending image-pull-backoff svc-selector-mismatch pvc-unbound; do
  echo "$list" | grep -q "\"id\":\"$id\"" || { echo "FAIL: missing case $id"; fail=1; }
  [ -d "$here/cases/$id" ] || { echo "FAIL: missing case dir $id"; fail=1; }
  [ -f "$here/cases/$id/inject.sh" ] || { echo "FAIL: $id missing inject.sh"; fail=1; }
  [ -f "$here/cases/$id/verify.sh" ] || { echo "FAIL: $id missing verify.sh"; fail=1; }
done

# claims follow the instance, not the plugin
"$here/plugin" list --json --instance=k8s-y-2 | grep -q '"kind:lab-k8s-y-2"' || { echo "FAIL: claim not per-instance"; fail=1; }

# --kubeconfig switches the claim from owning a cluster to owning a namespace
args="$tmp/args.json"
echo '{"kubeconfig":"/nonexistent/kubeconfig"}' > "$args"
"$here/plugin" list --json --instance=k8s-z-3 --args=@"$args" | grep -q '"kind:' \
  && { echo "FAIL: reused cluster must not claim a kind cluster"; fail=1; }
"$here/plugin" list --json --instance=k8s-z-3 --args=@"$args" | grep -q '"kube:' || { echo "FAIL: reused cluster must claim a namespace"; fail=1; }

# unknown case is rejected before any cluster work
chk_fail "$here/plugin" up nosuchcase --instance=k8s-x-1 --args=@"$args"

# down on a nonexistent kind cluster is a success (idempotent). Skipped when kind
# is not installed, since the check would be about the binary, not the logic.
if command -v kind >/dev/null 2>&1; then
  chk "$here/plugin" down ingress-502 --instance=k8s-does-not-exist --args=@"$args"
fi

[ "$fail" -eq 0 ] && echo "OK"
exit "$fail"
