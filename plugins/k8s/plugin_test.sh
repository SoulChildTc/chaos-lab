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

# list: one catalogue with every fault, one claim for the whole instance
list="$("$here/plugin" list --json --instance=k8s-1)"
echo "$list" | grep -q '"kind:lab-k8s-1"' || { echo "FAIL: kind claim"; fail=1; }
count="$(echo "$list" | grep -o '"id":"[^"]*"' | wc -l | tr -d ' ')"
[ "$count" = "5" ] || { echo "FAIL: want 5 faults, got $count"; fail=1; }
for id in ingress-502 pod-pending image-pull-backoff svc-selector-mismatch pvc-unbound; do
  echo "$list" | grep -q "\"id\":\"$id\"" || { echo "FAIL: missing fault $id"; fail=1; }
  [ -f "$here/cases/$id/inject.sh" ] || { echo "FAIL: $id missing inject.sh"; fail=1; }
  [ -f "$here/cases/$id/verify.sh" ] || { echo "FAIL: $id missing verify.sh"; fail=1; }
done

# the symptom must be present and must not name the faults
echo "$list" | grep -q '"symptom"' || { echo "FAIL: no symptom"; fail=1; }
echo "$list" | grep -q '"connect"' || { echo "FAIL: no connect info"; fail=1; }
for id in ingress-502 pod-pending image-pull-backoff svc-selector-mismatch pvc-unbound; do
  case "$(echo "$list" | sed -n 's/.*"symptom":"\([^"]*\)".*/\1/p')" in
    *"$id"*) echo "FAIL: symptom leaks the fault name $id"; fail=1 ;;
  esac
done

# claims follow the instance
"$here/plugin" list --json --instance=k8s-2 | grep -q '"kind:lab-k8s-2"' || { echo "FAIL: claim not per-instance"; fail=1; }

# --kubeconfig switches from owning a cluster to owning namespaces
args="$tmp/args.json"
echo '{"kubeconfig":"/nonexistent/kubeconfig"}' > "$args"
"$here/plugin" list --json --instance=k8s-3 --args=@"$args" | grep -q '"kind:' \
  && { echo "FAIL: reused cluster must not claim a kind cluster"; fail=1; }
"$here/plugin" list --json --instance=k8s-3 --args=@"$args" | grep -q '"kube:' || { echo "FAIL: reused cluster must claim a namespace"; fail=1; }

# down on a nonexistent cluster is a success (idempotent)
if command -v kind >/dev/null 2>&1; then
  echo '{}' > "$args"
  chk "$here/plugin" down --instance=k8s-does-not-exist --args=@"$args"
fi

[ "$fail" -eq 0 ] && echo "OK"
exit "$fail"
