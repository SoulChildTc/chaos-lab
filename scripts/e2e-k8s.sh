#!/usr/bin/env bash
# End-to-end check of the k8s plugin: bring the environment up, prove none of the
# faults is fixed yet, fix them all one by one, prove check passes, then prove
# nothing is left behind.
#
# Requires: kind, kubectl, docker, and a built chaoslab binary.
#   go build -o /tmp/chaoslab ./cmd/chaoslab
#   bash scripts/e2e-k8s.sh
set -uo pipefail

root="$(cd "$(dirname "$0")/.." && pwd)"
bin="${CHAOSLAB_BIN:-/tmp/chaoslab}"
[ -x "$bin" ] || { echo "build first: go build -o $bin ./cmd/chaoslab" >&2; exit 1; }
for c in kind kubectl docker; do
  command -v "$c" >/dev/null || { echo "missing $c" >&2; exit 1; }
done

export CHAOSLAB_HOME="$(mktemp -d)"
trap '"$bin" stop --all >/dev/null 2>&1; kind get clusters 2>/dev/null | grep "^lab-" | while read -r c; do kind delete cluster --name "$c" >/dev/null 2>&1; done' EXIT

"$bin" plugin install "$root/plugins/k8s" || exit 1

fail=0
note() { echo "FAIL: $*"; fail=1; }

echo "=== start"
if ! start_out="$("$bin" start k8s 2>&1)"; then
  note "start failed"
  echo "$start_out" | tail -5
  exit 1
fi
echo "$start_out" | grep -q "SYMPTOM" || note "start printed no symptom"
echo "$start_out" | grep -q "CONNECT" || note "start printed no connection details"
# Naming the faults would hand over the answer.
for f in ingress-502 pod-pending image-pull-backoff svc-selector-mismatch pvc-unbound; do
  echo "$start_out" | grep -q "$f" && note "start leaked the fault name $f"
done

# The plugin owns a throwaway cluster; point kubectl at it, not at whatever the
# host happens to be using.
export KUBECONFIG="$CHAOSLAB_HOME/k8s-state/k8s-1.kubeconfig"

echo "=== all faults present"
[ "$(kind get clusters | grep -c '^lab-k8s-')" = "1" ] || note "want exactly one kind cluster"
ns_count="$(kubectl get ns -o name 2>/dev/null | grep -c 'namespace/lab-k8s-')"
[ "$ns_count" = "5" ] || note "want 5 fault namespaces, got $ns_count"

echo "=== unfixed must not pass"
if "$bin" check k8s >/dev/null 2>&1; then
  note "check passed without any fix"
fi

echo "=== fix everything"
kubectl patch svc web -n lab-k8s-1-ingress-502 --type=json \
  -p='[{"op":"replace","path":"/spec/ports/0/targetPort","value":80}]' >/dev/null || note "ingress fix failed"
kubectl patch deployment cache -n lab-k8s-1-pod-pending --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/resources/requests/cpu","value":"100m"}]' >/dev/null || note "pod fix failed"
kubectl patch deployment api -n lab-k8s-1-image-pull-backoff --type=json \
  -p='[{"op":"replace","path":"/spec/template/spec/containers/0/image","value":"nginx:1.27-alpine"}]' >/dev/null || note "image fix failed"
kubectl patch svc frontend -n lab-k8s-1-svc-selector-mismatch --type=json \
  -p='[{"op":"replace","path":"/spec/selector","value":{"app":"frontend"}}]' >/dev/null || note "selector fix failed"
kubectl apply -f - >/dev/null <<'EOF' || note "storageclass fix failed"
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: fast-ssd
provisioner: rancher.io/local-path
volumeBindingMode: WaitForFirstConsumer
reclaimPolicy: Delete
EOF

echo "=== wait for convergence"
for _ in $(seq 1 60); do
  if "$bin" check k8s >/dev/null 2>&1; then break; fi
  sleep 5
done
if ! check_out="$("$bin" check k8s 2>&1)"; then
  note "check still failing after fixing every fault"
  echo "$check_out" | tail -5
fi
echo "$check_out" | grep -q "ROOT CAUSE" || note "passing check printed no root cause"

echo "=== stop"
"$bin" stop k8s || note "stop failed"
if kind get clusters 2>/dev/null | grep -q '^lab-k8s-'; then
  note "kind cluster left behind"
  kind get clusters | grep '^lab-k8s-'
fi

if [ "$fail" -eq 0 ]; then echo "ALL PASS"; fi
exit "$fail"
