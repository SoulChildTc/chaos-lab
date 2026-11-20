#!/usr/bin/env bash
# verify: exit 0 only when the cache pod is actually scheduled and running.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"

pod="$(kubectl get pods -n "$ns" -l app=cache -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)"
[ -n "$pod" ] || { echo "k8s: 找不到 cache pod" >&2; exit 2; }

phase="$(kubectl get pod -n "$ns" "$pod" -o jsonpath='{.status.phase}' 2>/dev/null)"
ready="$(kubectl get pod -n "$ns" "$pod" -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null)"

if [ "$phase" = "Running" ] && [ "$ready" = "true" ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"Deployment cache 声明了 100 核 CPU request，而节点可分配 CPU 远小于此。调度器找不到满足条件的节点，Pod 一直 Pending。","clues":["kubectl describe pod 的事件里是 Insufficient cpu，不是镜像问题","kubectl describe node 看 Allocatable 与 Allocated resources","Pending 且 spec.nodeName 为空说明还没被调度"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（Pod phase=$phase ready=$ready）" >&2
exit 1
