#!/usr/bin/env bash
# inject: deploy, then prove the pod is actually stuck Pending.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"
dir="${K8S_CASE_DIR:?}"

sed "s/PLACEHOLDER/$ns/g" "$dir/manifests.yaml" | kubectl apply -n "$ns" -f - >&2 || exit 1

# Wait for the scheduler to give up on placing it. Pending with no node assigned
# is the observable symptom; the event only appears a little later.
for _ in $(seq 1 30); do
  phase="$(kubectl get pods -n "$ns" -l app=cache -o jsonpath='{.items[0].status.phase}' 2>/dev/null)"
  node="$(kubectl get pods -n "$ns" -l app=cache -o jsonpath='{.items[0].spec.nodeName}' 2>/dev/null)"
  if [ "$phase" = "Pending" ] && [ -z "$node" ]; then
    echo "k8s: 故障已就绪，Pod 处于 Pending 且未分配到节点" >&2
    exit 0
  fi
  if [ "$phase" = "Running" ]; then
    echo "k8s: 故障未能复现（Pod 竟然启动成功），检查节点容量" >&2
    exit 1
  fi
  sleep 2
done
echo "k8s: 等待 Pod 进入 Pending 超时" >&2
exit 1
