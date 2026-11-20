#!/usr/bin/env bash
# Shared cluster lifecycle for the k8s plugin. Sourced by plugin, never executed
# directly.
#
# Two modes:
#   default        one throwaway kind cluster per instance, deleted on down
#   KUBECONFIG_EXTERNAL set   reuse a cluster the user already has; this plugin
#                              owns only one namespace inside it

need_cmd() {
  local missing=()
  for c in "$@"; do
    command -v "$c" >/dev/null 2>&1 || missing+=("$c")
  done
  if [ ${#missing[@]} -gt 0 ]; then
    echo "k8s: 缺少依赖：${missing[*]}" >&2
    case "${missing[0]}" in
      kind) echo "  安装：brew install kind   或者 https://kind.sigs.k8s.io/docs/user/quick-start/#installation" >&2 ;;
      kubectl) echo "  安装：brew install kubectl" >&2 ;;
      docker) echo "  安装 Docker Desktop / OrbStack 并确保 docker 命令可用" >&2 ;;
    esac
    return 1
  fi
}

plugin_state_root() {
  printf '%s/k8s-state' "${CHAOSLAB_HOME:-$HOME/.lab}"
}

cluster_name() { printf 'lab-%s' "$instance"; }

kubeconfig_path() {
  printf '%s/%s.kubeconfig' "$(plugin_state_root)" "$instance"
}

have_external() { [ -n "${KUBECONFIG_EXTERNAL:-}" ]; }

kubeconfig_use() {
  if have_external; then
    export KUBECONFIG="$KUBECONFIG_EXTERNAL"
    return 0
  fi
  local kc
  kc="$(kubeconfig_path)"
  [ -f "$kc" ] || { echo "k8s: 找不到实例的 kubeconfig：$kc" >&2; return 1; }
  export KUBECONFIG="$kc"
  kubectl cluster-info >/dev/null 2>&1
}

cluster_up() {
  mkdir -p "$(plugin_state_root)" || return 1

  if have_external; then
    export KUBECONFIG="$KUBECONFIG_EXTERNAL"
    if ! kubectl cluster-info >/dev/null 2>&1; then
      echo "k8s: 无法连接 --kubeconfig 指定的集群" >&2
      return 1
    fi
    kubectl create namespace "$K8S_NAMESPACE" >/dev/null 2>&1 || true
    echo "k8s: 复用外部集群，namespace $K8S_NAMESPACE" >&2
    return 0
  fi

  local name image
  name="$(cluster_name)"
  image="${NODE_IMAGE:-kindest/node:v1.31.0}"

  # A crashed earlier attempt may have left the cluster behind. Deleting it first
  # makes up idempotent, so retrying never collides with a half-built cluster.
  if kind get clusters 2>/dev/null | grep -qx "$name"; then
    echo "k8s: 发现残留集群 $name，先删除" >&2
    kind delete cluster --name "$name" >/dev/null 2>&1 || true
  fi

  echo "k8s: 创建集群 $name（镜像 $image）" >&2
  local started=$SECONDS
  if ! kind create cluster --name "$name" --image "$image" --wait 180s \
       --kubeconfig "$(kubeconfig_path)" >&2; then
    echo "k8s: 集群创建失败" >&2
    kind delete cluster --name "$name" >/dev/null 2>&1 || true
    return 1
  fi
  echo "k8s: 集群就绪（$((SECONDS - started))s）" >&2

  export KUBECONFIG
  KUBECONFIG="$(kubeconfig_path)"
  kubectl create namespace "$K8S_NAMESPACE" >/dev/null 2>&1 || true
  kubectl wait --for=condition=Ready nodes --all --timeout=120s >&2 || return 1
}

cluster_down() {
  if have_external; then
    export KUBECONFIG="$KUBECONFIG_EXTERNAL"
    # Only ever touches this instance's namespace. A reused cluster is not ours.
    kubectl delete namespace "$K8S_NAMESPACE" --ignore-not-found --timeout=120s >&2
    return $?
  fi

  local name
  name="$(cluster_name)"
  # Idempotent: a missing cluster is already the desired end state.
  if kind get clusters 2>/dev/null | grep -qx "$name"; then
    echo "k8s: 删除集群 $name" >&2
    kind delete cluster --name "$name" >&2
  else
    echo "k8s: 集群 $name 不存在，无需删除" >&2
  fi
  rm -f "$(kubeconfig_path)"
  return 0
}

# load_case applies the broken manifest and runs the case's inject step.
load_case() {
  local case_dir="$here/cases/$1"
  [ -d "$case_dir" ] || { echo "k8s: 场景目录不存在：$case_dir" >&2; return 1; }
  export K8S_CASE_DIR="$case_dir"

  if [ -f "$case_dir/inject.sh" ]; then
    ( cd "$case_dir" && bash ./inject.sh ) || return 1
  fi
  return 0
}
