#!/usr/bin/env bash
# verify: exit 0 only when the claim is Bound and the pod using it is running.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"

kubectl get pvc data -n "$ns" >/dev/null 2>&1 || { echo "k8s: 找不到 PVC data" >&2; exit 2; }

phase="$(kubectl get pvc data -n "$ns" -o jsonpath='{.status.phase}' 2>/dev/null)"
if [ "$phase" != "Bound" ]; then
  echo "k8s: 仍未修好（PVC phase=$phase）" >&2
  exit 1
fi

ready="$(kubectl get deploy db -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)"
if [ "${ready:-0}" -ge 1 ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"PVC data 指定的 storageClassName 是 fast-ssd，集群里没有这个 StorageClass，也没有动态供给者接手，PVC 一直 Pending，依赖它的 Pod 无法调度。","clues":["kubectl get pvc 的 Events 里是 storageclass not found，不是容量不足","kubectl get storageclass 看集群实际有哪个默认类","Pod 事件是 unbound immediate PersistentVolumeClaims，指向问题在 PVC"]}
JSON
  exit 0
fi

echo "k8s: PVC 已绑定，但 Pod 未就绪（ready=${ready:-0}）" >&2
exit 1
