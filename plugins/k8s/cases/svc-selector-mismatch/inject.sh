#!/usr/bin/env bash
# inject: deploy, then prove the Service has no endpoints while pods are Ready.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"
dir="${K8S_CASE_DIR:?}"

sed "s/PLACEHOLDER/$ns/g" "$dir/manifests.yaml" | kubectl apply -n "$ns" -f - >&2 || exit 1
kubectl wait -n "$ns" --for=condition=Available deployment/frontend --timeout=300s >&2 || exit 1

for _ in $(seq 1 30); do
  endpoints="$(kubectl get endpoints frontend -n "$ns" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null)"
  if [ -z "$endpoints" ]; then
    echo "k8s: 故障已就绪，Service frontend 没有 endpoints" >&2
    exit 0
  fi
  sleep 2
done
echo "k8s: 故障未能复现（Service 竟然有 endpoints）" >&2
exit 1
