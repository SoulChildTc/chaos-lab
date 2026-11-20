<!-- prettier-ignore -->
<div align="center">

<img src="./docs/images/icon.svg" alt="" align="center" height="72" />

# chaos-lab

**本地可跑的故障演练实验室** —— 一条命令起环境，故障已埋好，你自己进去查，修好由系统判定。

[![Go](https://img.shields.io/badge/Go-%3E%3D1.22-00ADD8?style=flat-square&logo=go&logoColor=white)](https://go.dev)
[![Dependencies](https://img.shields.io/badge/dependencies-0-3c873a?style=flat-square)](go.mod)
[![Tests](https://img.shields.io/badge/tests-135%20passing-3c873a?style=flat-square)](#开发)
[![Plugin protocol](https://img.shields.io/badge/plugin%20protocol-5%20commands-8957e5?style=flat-square)](docs/plugin-protocol.md)

[概述](#概述) • [快速开始](#快速开始) • [命令](#命令) • [插件](#插件) • [评测 Agent](#评测-agent) • [工作原理](#工作原理) • [开发](#开发)

</div>

```console
$ chaoslab start k8s/ingress-502
k8s: 创建集群 lab-k8s-ingress-502-1（镜像 kindest/node:v1.31.0）
k8s: 集群就绪（30s）
k8s: 故障已就绪
现象：访问站点返回 502
k8s/ingress-502#1 已就绪（slug k8s-ingress-502-1）
修好后运行：chaoslab check k8s/ingress-502#1

$ # ……自己进集群查，找到问题、改对之后

$ chaoslab check k8s/ingress-502 --json
{"passed":true,"report":{"root_cause":"Service web 的 targetPort 是 8080，而容器实际监听 80……","clues":["Endpoints 有地址，容易误判为正常","curl 返回 connection refused"]}}
```

不是教程，不是模拟器。环境是真的，故障是真的，判据是实测的。

## 概述

学 K8s 和中间件的人常卡在同一个地方：文档看得懂，真出故障时不知道从哪下手。而生产环境不敢拿来练，自己搭又常常只搭出「一切正常」的那条路径。

**chaos-lab 把「故障已发生的现场」做成一条命令。** 它会起好环境、埋好坑，只告诉你现象（"访问返回 502"），不告诉你在哪。你自己进去查、自己修，系统在后台按插件定义的规则实测判定，通过后给你根因复盘和关键线索。

判定不是自说自话：`stop` 之后框架会自己去查端口、容器、集群、目录是否真的没了，没清干净就明确告诉你「`dirty`」，并且拦住同场景再次启动。

## 特性

- **只给现象，不给答案** —— 起环境时输出停在实际症状那一行，修好之后才揭示根因
- **判定是实测的** —— 插件声明自己占用什么资源，框架在 `stop` 后自己去验，不依赖插件的自我汇报
- **零污染** —— K8s 场景每个实例一套一次性 kind 集群，`stop` 就删，不碰你本机任何集群
- **可并行** —— 不同场景能同时跑，靠 `claims` 检测端口 / 容器 / 集群 / 目录冲突，撞了会告诉你是谁占了什么
- **可扩展** —— 插件是任何语言写的一个可执行文件，框架对内部零假设。MySQL、Redis、ES、Mongo 都能接
- **可机器消费** —— 所有命令支持 `--json`，每次运行落盘一条记录，适合评测 Agent 的排障成功率
- **零第三方依赖** —— Go 标准库，单二进制

## 快速开始

需要 [Go 1.22+](https://go.dev/dl/) 和 Docker（Docker Desktop 或 OrbStack）。

```bash
git clone https://github.com/soulchildtc/chaos-lab.git
cd chaos-lab

go build -o ~/.local/bin/chaoslab ./cmd/chaoslab
chaoslab plugin install ./plugins/k8s
```

K8s 场景还需要 `kind` 和 `kubectl`：

```bash
brew install kind kubectl
```

然后挑一个场景进来：

```bash
chaoslab list                        # 有哪些场景
chaoslab start k8s/pod-pending       # 起一个，只给现象

export KUBECONFIG=~/.lab/k8s-state/k8s-pod-pending-1.kubeconfig
kubectl get pods -A                  # 自己查

chaoslab check k8s/pod-pending --watch   # 一边修一边轮询判定（最长 20 分钟）
chaoslab stop k8s/pod-pending            # 清理，并实测是否真的清干净
```

起环境约 30–40 秒（节点镜像已在本机时）。

> [!TIP]
> 首次运行要拉约 900MB 的 `kindest/node` 镜像。网络慢的话先手动拉一次，或换 `--node-image` 指向你本机已有的镜像。

## 命令

| 命令 | 说明 |
|---|---|
| `chaoslab list [--json]` | 所有已装插件的场景 |
| `chaoslab start <插件>/<场景> [--参数=值] [--help] [--json]` | 起环境；`--help` 看该插件的参数表 |
| `chaoslab check <引用> [--watch] [--json]` | 判定是否修好；`--watch` 每 3 秒轮询到通过或超时 |
| `chaoslab stop <引用> [--json]` | 清理并实测残留 |
| `chaoslab stop --all` | 停掉全部 |
| `chaoslab status [<实例>] [--json]` | 活跃实例 |
| `chaoslab plugin install <目录\|git-url>` | 装插件 |
| `chaoslab plugin uninstall <名字>` | 卸插件（有存活实例时拒绝） |
| `chaoslab plugin info <名字>` | 看插件支持的参数和场景 |
| `chaoslab plugin validate <目录>` | 写插件时自检，不执行 `up` |

`<引用>` 可以是 `插件/场景`（该场景只有一个存活实例时），也可以是完整实例 ID `插件/场景#2`。

### 退出码

| 码 | 含义 |
|---|---|
| `0` | 成功 / 已修好 |
| `1` | 用法错误、场景不存在、资源冲突 |
| `2` | 检查通过但**尚未修好** |
| `3` | 插件执行出错（含协议不合规、检查超时） |

区分 `2` 和 `3` 很重要：`2` 是「用户还没修好」，`3` 是「这个场景本身坏了」。

## 插件

插件是一个可执行文件加它所需要的一切，框架只调 5 个子命令，对内部结构零假设。

| 插件 | 说明 | 文档 |
|---|---|---|
| `k8s` | 五个 Kubernetes 故障场景，每个实例一套一次性 kind 集群 | [plugins/k8s/README.md](plugins/k8s/README.md) |
| `fake` | 零依赖测试替身，验证框架行为，也是写插件的最小参考 | [plugins/fake/README.md](plugins/fake/README.md) |

```bash
chaoslab plugin install ./plugins/k8s
chaoslab list
```

自带的五个 K8s 场景：

| 场景 | 现象 |
|---|---|
| `k8s/ingress-502` | 访问站点返回 502 |
| `k8s/pod-pending` | Pod 一直 Pending |
| `k8s/image-pull-backoff` | Pod 卡在 ImagePullBackOff |
| `k8s/svc-selector-mismatch` | Service 没有 endpoints，访问超时 |
| `k8s/pvc-unbound` | PVC 一直 Pending |

### 写自己的插件

插件是任何语言写的一个可执行文件。想接 MySQL、Redis、ES、Mongo，或者干脆用 docker compose 起一套环境，都走同一套协议。

- **协议规范**：[docs/plugin-protocol.md](docs/plugin-protocol.md) —— 子命令、执行环境、claims、bash 与 Python 两份骨架、三个常见错误
- **参考实现**：[plugins/fake](plugins/fake/README.md)（最小）、[plugins/k8s](plugins/k8s/README.md)（真实场景）

```bash
chaoslab plugin validate ./myplugin   # 静态检查
chaoslab plugin install ./myplugin
```

## 评测 Agent

chaos-lab 也能当 Agent 的考场：起场景、把入口交给 Agent、由系统判定它有没有修好。

```bash
chaoslab start k8s/ingress-502 --json    # {"instance":"k8s/ingress-502#1","slug":"k8s-ingress-502-1","phase":"running"}
chaoslab status --json                   # 活跃实例
chaoslab check k8s/ingress-502 --json    # {"passed":true,"report":{...}}
chaoslab stop k8s/ingress-502 --json     # {"released":true,"verdicts":[...]}
```

出口在 `~/.lab/runs/`，每次运行一条 JSON：

```json
{
  "instance": "k8s/ingress-502#1",
  "result": "passed",
  "duration_sec": 412,
  "checks": 9,
  "report": { "passed": true, "root_cause": "...", "clues": ["..."] },
  "claims_released": true
}
```

`result` 取值 `passed` / `timeout` / `abandoned` / `dirty` / `start_failed`。`duration_sec` 是从起到修好的耗时。评测程序遍历这个目录：`passed` 计数做分子，全部记录做分母。

> [!NOTE]
> `~/.lab/runs/` 只写不读，框架本身没有统计命令。聚合、成功率、平均修复时长由评测程序自己算。

## 工作原理

```
chaoslab (Go 单二进制)
  ├── 发现插件        ~/.lab/plugins/<name>/plugin
  ├── 解析参数        插件 schema 声明的声明式参数，生成 --help 并校验
  ├── 检测冲突        claims 精确比对
  ├── 调用生命周期     up / down / check 子进程，超时杀整个进程组
  ├── 记录状态        ~/.lab/state/instances.json（文件锁 + 原子写）
  └── 实测残留        port / docker / kind / dir 四类 claim

plugin (任意语言)
  ├── 决定用什么环境   kind / docker compose / 复用已有集群 / 别的
  ├── 自己管理依赖     缺 kind 时自己报错并给安装提示
  └── 自己负责清理     down 必须幂等，且能在任意中间状态执行
```

框架**不创建、不删除任何环境资源**，也不重试失败的 `down`。它只协调、观察、记录。

### 实例状态

`phase` 和 `live` 是两个独立字段，这一点是刻意的：

- `phase` —— `starting` / `running` / `passed` / `stopped` / `dirty`，给人看的事件结果
- `live` —— 实例是否仍持有环境资源

修好（`passed`）之后环境**还在**，你还要能进去复盘，所以 `live` 仍是 `true`。只有 `down` 成功且残留复查通过，才置 `false`。冲突检测、重复启动拦截、卸载拦截都看 `live`，不看 `phase`。

### 出问题的时候

| 情况 | 行为 |
|---|---|
| `up` 失败或超时 | 实例标 `stopped` 但保留 `live=true`（可能留了资源），提示你 `stop` 收尾 |
| `down` 失败 | 实例标 `dirty`，保留原因，不重试、不代劳 |
| `down` 说成功但资源还在 | 框架实测抓出来，标 `dirty` 并列出具体残留 |
| 框架进程被 `kill -9` | 下次任意命令惰性收敛：`starting` 且 pid 已不存在 → 标 `stopped` + 中断原因，`stop` 能收尾 |
| claim 前缀框架不认识 | 参与冲突检测，但不复查，`stop` 时明确标注「由插件保证」 |

> [!IMPORTANT]
> 清理永远是插件的 `down`。框架只负责**证明**它到底清没清干净 —— 宁可告诉你「没清干净」，也不假装清了。

## 开发

```bash
go build ./...
go test ./...                     # 135 个测试，5 个包
go test ./... -race               # 并发安全

bash plugins/fake/plugin_test.sh  # 插件协议自测
bash plugins/k8s/plugin_test.sh
bash scripts/e2e-k8s.sh           # 五个 K8s 场景端到端验收（需要 kind）
```

```
cmd/chaoslab/          入口
internal/cli/          每个子命令、实例解析、输出格式
internal/plugin/       插件协议：子进程调用、schema/list 解析、安装
internal/state/        实例状态，文件锁 + 原子写
internal/claims/       冲突检测与残留实测
internal/runlog/       run 记录
plugins/k8s/           K8s 插件（bash + kind）
plugins/fake/          测试替身插件
docs/                  设计文档、实施计划、插件协议
scripts/e2e-k8s.sh     端到端验收
```

设计取舍、被否决的方案、为什么不用第三方库，都写在 [设计文档](docs/superpowers/specs/2026-11-20-chaos-lab-design.md) 里。

## 边界

- 没有图形界面，没有多用户
- 不碰真实生产集群（除非你用 `--kubeconfig` 显式指向它，且只写一个 namespace）
- `~/.lab/runs/` 只写不读，没有统计命令
- 不做插件版本管理和升级
- 目前只有 K8s 插件；其他中间件走插件协议接入，核心零改动

<div align="center">
<sub>本地演练，随便搞坏。坏了能重来，才是练出来的前提。</sub>
</div>
