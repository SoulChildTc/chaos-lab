#!/usr/bin/env bash
# verify: exit 0 only when the Service actually selects the running pods.
set -uo pipefail
ns="${K8S_NS:-${K8S_NAMESPACE:?}}"

kubectl get svc frontend -n "$ns" >/dev/null 2>&1 || { echo "k8s: 找不到 Service frontend" >&2; exit 2; }

endpoints="$(kubectl get endpoints frontend -n "$ns" -o jsonpath='{.subsets[*].addresses[*].ip}' 2>/dev/null)"
ready="$(kubectl get deploy frontend -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)"

if [ -n "$endpoints" ] && [ "${ready:-0}" -ge 1 ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"Service frontend 的 selector 是 app=web-frontend，而 Pod 的标签是 app=frontend / tier=web。selector 匹配不到任何 Pod，Endpoints 为空，访问超时。","clues":["kubectl get endpoints frontend 输出为空","kubectl get pods --show-labels 与 kubectl describe svc frontend 的 Selector 对比","Pod 本身是 Ready 的，说明问题在 Service 一侧"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（endpoints=${endpoints:-空} ready=${ready:-0}）" >&2
exit 1
