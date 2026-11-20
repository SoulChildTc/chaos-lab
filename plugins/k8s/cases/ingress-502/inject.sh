#!/usr/bin/env bash
# inject: deploy the broken stack, then prove the symptom is visible.
set -uo pipefail
ns="${K8S_NS:-${K8S_NAMESPACE:?}}"
dir="${K8S_CASE_DIR:?}"

sed "s/PLACEHOLDER/$ns/g" "$dir/manifests.yaml" | kubectl apply -n "$ns" -f - >&2 || exit 1

kubectl wait -n "$ns" --for=condition=Available deployment/web deployment/gateway --timeout=300s >&2 || exit 1

# The bug only shows up when traffic arrives: nothing listens on 8080, so the
# gateway's upstream fails.
reproduced=0
for _ in $(seq 1 30); do
  if kubectl exec -n "$ns" deploy/gateway -- wget -qO- --timeout=2 http://web/ >/dev/null 2>&1; then
    sleep 2
    continue
  fi
  reproduced=1
  break
done

if [ "$reproduced" -ne 1 ]; then
  echo "k8s: 故障未能复现（流量竟然是通的），构建失败" >&2
  exit 1
fi
echo "k8s: 故障已就绪" >&2
