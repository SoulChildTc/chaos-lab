#!/usr/bin/env bash
# verify: exit 0 only when the Deployment has a Ready replica. Looking at the
# first Pod would keep seeing the old Pending one after a patch creates a new
# ReplicaSet, so readiness is read from the Deployment itself.
set -uo pipefail
ns="${K8S_NS:-${K8S_NAMESPACE:?}}"

kubectl get deploy cache -n "$ns" >/dev/null 2>&1 || { echo "k8s: 找不到 Deployment cache" >&2; exit 2; }

ready="$(kubectl get deploy cache -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)"
if [ "${ready:-0}" -ge 1 ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"Deployment cache 声明了 100 核 CPU request，而节点可分配 CPU 远小于此。调度器找不到满足条件的节点，Pod 一直 Pending。","clues":["kubectl describe pod 的事件里是 Insufficient cpu，不是镜像问题","kubectl describe node 看 Allocatable 与 Allocated resources","Pending 且 spec.nodeName 为空说明还没被调度"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（deployment readyReplicas=${ready:-0}）" >&2
exit 1
