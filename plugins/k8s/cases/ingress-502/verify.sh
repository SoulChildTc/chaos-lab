#!/usr/bin/env bash
# verify: exit 0 only when traffic from the gateway actually reaches the web pod.
# On success, print the report JSON the framework stores.
set -uo pipefail
ns="${K8S_NS:-${K8S_NAMESPACE:?}}"

kubectl get pods -n "$ns" >/dev/null 2>&1 || { echo "k8s: namespace $ns 不可用" >&2; exit 2; }

if kubectl exec -n "$ns" deploy/gateway -- wget -qO- --timeout=3 http://web/ 2>/dev/null | grep -qi "welcome to nginx"; then
  cat <<'JSON'
{"passed":true,"root_cause":"Service web 的 targetPort 是 8080，而容器实际监听 80。Service 有 endpoints，但流量打到没有监听的端口，网关侧表现为 502。","clues":["kubectl get endpoints web 显示有地址，容易误判为正常","kubectl exec 进 gateway 访问 web 会 connection refused","kubectl describe svc web 里 targetPort=8080 与容器 ports 不一致"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（网关访问 web 失败）" >&2
exit 1
