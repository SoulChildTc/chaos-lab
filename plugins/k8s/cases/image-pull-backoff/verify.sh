#!/usr/bin/env bash
# verify: exit 0 only when no container is waiting on an image pull.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"

pod="$(kubectl get pods -n "$ns" -l app=api -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)"
[ -n "$pod" ] || { echo "k8s: 找不到 api pod" >&2; exit 2; }

reason="$(kubectl get pod -n "$ns" "$pod" -o jsonpath='{.status.containerStatuses[0].state.waiting.reason}' 2>/dev/null)"
ready="$(kubectl get pod -n "$ns" "$pod" -o jsonpath='{.status.containerStatuses[0].ready}' 2>/dev/null)"

case "$reason" in
  ImagePullBackOff|ErrImagePull|InvalidImageName)
    echo "k8s: 仍未修好（$reason）" >&2
    exit 1
    ;;
esac

if [ "$ready" = "true" ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"Deployment api 使用的镜像 tag 不存在（nginx:1.27.0-does-not-exist）。kubelet 拉取失败后按退避重试，Pod 停在 ImagePullBackOff。","clues":["kubectl describe pod 的 Events 里是 manifest unknown，说明仓库可达但 tag 不存在","ImagePullBackOff 与 ErrImagePull 的区别：前者是退避重试中","imagePullSecrets 缺失会显示 unauthorized，与此处不同"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（pod 未就绪，reason=${reason:-none}）" >&2
exit 1
