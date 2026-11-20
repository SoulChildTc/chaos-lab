# k8s 插件

五个 Kubernetes 故障场景，**每个实例一套独立的一次性 kind 集群**，`stop` 时整个删除。不碰你本机的任何集群、namespace 或 PV。

```bash
chaoslab plugin install ./plugins/k8s
chaoslab list
```

## 依赖

| 工具 | 用途 | 缺失时 |
|---|---|---|
| `docker` | 承载 kind 节点容器 | `start` 报错并提示 |
| `kind` | 创建/删除临时集群 | 同上 |
| `kubectl` | 注入故障、检查状态 | 同上 |

`brew install kind kubectl` 即可，Docker 用 Docker Desktop 或 OrbStack。

## 场景

| 场景 | 现象 | 埋的坑 |
|---|---|---|
| `k8s/ingress-502` | 访问站点返回 502 | Service 的 `targetPort` 写成 8080，容器实际监听 80 |
| `k8s/pod-pending` | Pod 一直 Pending | `resources.requests.cpu: 100`，没有节点能放下 |
| `k8s/image-pull-backoff` | Pod 卡在 ImagePullBackOff | 镜像 tag `nginx:1.27.0-does-not-exist` 不存在 |
| `k8s/svc-selector-mismatch` | Service 没有 endpoints | selector 是 `app=web-frontend`，Pod 标签是 `app=frontend` |
| `k8s/pvc-unbound` | PVC 一直 Pending | `storageClassName: fast-ssd`，集群里没有这个类 |

每个场景都验证过：未修复时 `check` 退出 2，修复后退出 0 并给出根因。

## 怎么进去查

`start` 之后会打印实例 slug，kubeconfig 在 `~/.lab/k8s-state/<slug>.kubeconfig`：

```bash
chaoslab start k8s/pod-pending
# k8s/pod-pending#1 已就绪（slug k8s-pod-pending-1）

export KUBECONFIG=~/.lab/k8s-state/k8s-pod-pending-1.kubeconfig
kubectl get pods -A
kubectl describe pod -n lab-k8s-pod-pending-1 -l app=cache
```

集群名是 `lab-<slug>`，场景资源都在 namespace `lab-<slug>` 里。

## 参数

| 参数 | 默认 | 说明 |
|---|---|---|
| `--node-image` | `kindest/node:v1.31.0` | kind 节点镜像 |
| `--kubeconfig` | 无 | 复用已有集群，见下 |

`chaoslab start k8s/pod-pending --help` 也能看到。

## 复用已有集群

```bash
chaoslab start k8s/pod-pending --kubeconfig ~/.kube/config
```

行为变化：

- **不创建集群**，只在目标集群里建一个 namespace `lab-<slug>`
- `stop` 只删这个 namespace，不动集群其他部分
- claim 从 `kind:lab-<slug>` 变成 `kube:<context>/lab-<slug>`，冲突检测仍然生效
- 该集群必须能拉取镜像（本插件不会向外部集群预加载镜像）

这是给“手边已经有一个集群、不想再起 kind”的场景用的。默认模式仍然是零污染的一次性集群。

## 时间

| 阶段 | 典型耗时 |
|---|---|
| 创建 kind 集群 | 30–40 秒（节点镜像已在本机时） |
| 注入故障 + 首次就绪 | 5–60 秒（取决于镜像是否已预加载） |
| `check` 单次 | 1–3 秒 |

`up_timeout` 12 分钟，`check_timeout` 20 分钟。首次运行要拉 `kindest/node` 镜像（约 900MB），网络慢时会明显更久。

## 镜像预加载

kind 集群**不共享**宿主 Docker 的镜像缓存，它会自己去 registry 拉。为了在离线或网络受限时也能用，插件会检查每个场景目录下的 `images.txt`，凡是本机 Docker 已有的镜像就灌进集群：

```
k8s: 预加载镜像 nginx:1.27-alpine
```

本机没有的镜像会被跳过，交给集群自己拉。想加快首次运行，先手动拉一次：

```bash
docker pull nginx:1.27-alpine redis:7-alpine postgres:16-alpine
```

> 注意：这里不能用 `kind load docker-image`。在 Apple Silicon 上 Docker Desktop 会存成多平台 OCI index，containerd 导入时报 `content digest ... not found`。插件改用 `docker save --platform` 再 `ctr import`。

`image-pull-backoff` 场景故意不预加载那个不存在的 tag——否则故障无法复现。

## 清理

`stop` 时插件执行：

1. 删除 kind 集群 `lab-<slug>`
2. 删除对应的 kubeconfig 文件

集群不存在时视为成功（幂等），所以你可以在任何中间状态下重试。

框架随后会自己执行 `kind get clusters` 确认集群真的没了。没删干净 → 实例标 `dirty` 并列出残留。

## 结构

```
plugins/k8s/
  plugin                协议入口：schema / list / up / down / check
  lib.sh                集群生命周期：创建、就绪等待、镜像预加载、删除
  cases/<场景>/
    manifests.yaml      带故障的 Kubernetes 资源
    images.txt          需要的镜像（用于预加载）
    inject.sh           部署 + 验证故障确实复现
    verify.sh           判定是否修好；通过时输出根因 JSON
```

框架不关心这个结构，只调用 `plugin`。想加第六个场景：建一个 `cases/<新场景>/` 目录，在 `plugin` 的 `CASE_IDS` 里加一行，写 `case_title` 分支即可。

## 已验证 / 未验证

已验证（本机 macOS + OrbStack + kind 0.33）：五个场景全部 `start → check(未修=2) → stop → 无残留`，同一场景连续三轮零残留。

未验证：Docker Desktop、Linux 上的 kind 行为；`--kubeconfig` 复用外部集群这条路径没有实跑过。
