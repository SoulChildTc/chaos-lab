#!/usr/bin/env bash
# inject: deploy, then prove the pod reports an image pull failure.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"
dir="${K8S_CASE_DIR:?}"

sed "s/PLACEHOLDER/$ns/g" "$dir/manifests.yaml" | kubectl apply -n "$ns" -f - >&2 || exit 1

for _ in $(seq 1 60); do
  reason="$(kubectl get pods -n "$ns" -l app=api -o jsonpath='{.items[0].status.containerStatuses[0].state.waiting.reason}' 2>/dev/null)"
  case "$reason" in
    ImagePullBackOff|ErrImagePull)
      echo "k8s: 故障已就绪，Pod 卡在 $reason" >&2
      exit 0
      ;;
    "")
      sleep 2
      continue
      ;;
    *)
      echo "k8s: 故障未能复现（等待原因是 $reason）" >&2
      exit 1
      ;;
  esac
done
echo "k8s: 等待镜像拉取失败超时" >&2
exit 1
