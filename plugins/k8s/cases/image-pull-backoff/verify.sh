#!/usr/bin/env bash
# verify: exit 0 only when the Deployment has a Ready replica. Reading the first
# Pod would keep seeing the old failing Pod after an image change creates a new
# ReplicaSet, so readiness comes from the Deployment.
set -uo pipefail
ns="${K8S_NAMESPACE:?}"

kubectl get deploy api -n "$ns" >/dev/null 2>&1 || { echo "k8s: 找不到 Deployment api" >&2; exit 2; }

ready="$(kubectl get deploy api -n "$ns" -o jsonpath='{.status.readyReplicas}' 2>/dev/null)"
if [ "${ready:-0}" -ge 1 ]; then
  cat <<'JSON'
{"passed":true,"root_cause":"Deployment api 使用的镜像 tag 不存在（nginx:1.27.0-does-not-exist）。kubelet 拉取失败后按退避重试，Pod 停在 ImagePullBackOff。","clues":["kubectl describe pod 的 Events 里是 manifest unknown，说明仓库可达但 tag 不存在","ImagePullBackOff 与 ErrImagePull 的区别：前者是退避重试中","imagePullSecrets 缺失会显示 unauthorized，与此处不同"]}
JSON
  exit 0
fi

echo "k8s: 仍未修好（deployment readyReplicas=${ready:-0}）" >&2
exit 1
