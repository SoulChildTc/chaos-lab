#!/usr/bin/env bash
# inject: deploy, then prove the PVC is stuck Pending for lack of a provisioner.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"
dir="${K8S_CASE_DIR:?}"

sed "s/PLACEHOLDER/$ns/g" "$dir/manifests.yaml" | kubectl apply -n "$ns" -f - >&2 || exit 1

for _ in $(seq 1 30); do
  phase="$(kubectl get pvc data -n "$ns" -o jsonpath='{.status.phase}' 2>/dev/null)"
  case "$phase" in
    Pending)
      echo "k8s: 故障已就绪，PVC data 处于 Pending，Pod 因此无法启动" >&2
      exit 0
      ;;
    Bound)
      echo "k8s: 故障未能复现（PVC 竟然绑定了），检查是否已存在 fast-ssd StorageClass" >&2
      exit 1
      ;;
    *)
      sleep 2
      ;;
  esac
done
echo "k8s: 等待 PVC 进入 Pending 超时" >&2
exit 1
