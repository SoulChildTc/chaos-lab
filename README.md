<!-- prettier-ignore -->
<div align="center">

<img src="./docs/images/icon.svg" alt="chaos-lab logo" height="64" />

# chaos-lab

*本地故障演练环境：起好带故障的环境，只给现象，你修，系统判定*

![Go 1.22+](https://img.shields.io/badge/Go->=1.22-00ADD8?style=flat-square&logo=go)
![License](https://img.shields.io/badge/License-MIT-yellow?style=flat-square)

[为什么用它](#为什么用它) • [安装](#安装) • [快速开始](#快速开始) • [用法](#用法) • [插件](#插件) • [Agent 评测](#agent-评测) • [实现](#实现) • [开发](#开发)

</div>

看文档能懂一个技术，真出故障时却不知道从哪下手。生产环境不能拿来练，自己搭的又常常只搭通了正常那一条路径。

chaos-lab 把故障现场做成一次性的：一条命令起好环境，只告诉你现象，要你自己从现象反推哪个组件坏了。改对了由脚本判定，才给根因和关键线索。

支持哪些技术栈取决于装了哪些插件，框架本身不关心。下面以自带的 `k8s` 插件为例：

```console
$ chaoslab start k8s/ingress-502
k8s: 创建集群 lab-k8s-ingress-502-1（镜像 kindest/node:v1.31.0）
k8s: 集群就绪（30s）
k8s: 故障已就绪
现象：访问站点返回 502
k8s/ingress-502#1 已就绪（slug k8s-ingress-502-1）
修好后运行：chaoslab check k8s/ingress-502#1

$ # 自己进环境查，改对之后

$ chaoslab check k8s/ingress-502
✓ k8s/ingress-502#1 通过

根因
  Service web 的 targetPort 是 8080，而容器实际监听 80。Service 有 endpoints，
  但流量打到没有监听的端口，网关侧表现为 502。

关键线索
  - kubectl get endpoints web 显示有地址，容易误判为正常
  - kubectl exec 进 gateway 访问 web 会 connection refused
  - kubectl describe svc web 里 targetPort=8080 与容器 ports 不一致
```

## 为什么用它

- **不点明故障。** 交互式演练平台通常直接告诉你哪个对象坏了。这里只给一句现象，你得自己缩小范围。
- **判定是产品的一部分。** 混沌工程工具往环境里注入故障，测的是系统韧性，不管你修得对不对。这里 `check` 通过才算修好，退出码 `2`（没修好）和 `3`（场景本身坏了）分开。
- **清理可验证。** 插件声明自己占用哪些资源，框架在 `stop` 后自己实测一遍，没清干净会被抓出来标 `dirty`。自己写脚本也能搭，但清理和验收都得自己写，失败一次就可能留一堆资源。
- **框架与技术栈零耦合。** Go 单二进制，不含任何具体技术栈的知识。加一个 MySQL 或 Redis 场景，不需要改框架一行代码。

## 安装

支持 macOS 和 Linux 的 amd64 / arm64。

```bash
curl -fsSL https://raw.githubusercontent.com/SoulChildTc/chaos-lab/main/scripts/install.sh | sh
```

脚本会识别系统与架构、下载对应包、校验 `SHA256SUMS`，把 `chaoslab` 装到 `/usr/local/bin`（不可写时退到 `~/.local/bin`），插件解压到 `~/.local/share/chaoslab/plugins`，装完打印下一步该敲什么。

想装指定版本或换位置，覆盖环境变量：

```bash
CHAOSLAB_VERSION=v0.1.0 CHAOSLAB_INSTALL_DIR=~/bin sh -c \
  'curl -fsSL https://raw.githubusercontent.com/SoulChildTc/chaos-lab/main/scripts/install.sh | sh'
```

> [!NOTE]
> Windows 没有原生二进制，请在 [WSL](https://learn.microsoft.com/windows/wsl/install) 里用 Linux 版。状态文件用 `flock` 加锁，这个调用在 Windows 上不存在。

从源码构建需要 Go 1.22+，框架无第三方依赖：

```bash
git clone https://github.com/SoulChildTc/chaos-lab.git
cd chaos-lab
go build -o ~/.local/bin/chaoslab ./cmd/chaoslab
```

**装插件。** 框架装好之后是空的，场景都来自插件：

```bash
# 用安装脚本
chaoslab plugin install ~/.local/share/chaoslab/plugins/k8s

# 从源码构建
chaoslab plugin install ./plugins/k8s

chaoslab list     # 这时才有东西可跑
```

插件依赖的工具由插件自己检查，缺什么会报错并给安装提示。

## 快速开始

```bash
chaoslab list                            # 有哪些场景
chaoslab start <插件>/<场景>              # 起一个，只给现象
chaoslab check <插件>/<场景> --watch      # 一边修一边轮询，每 3 秒，最长 20 分钟
chaoslab rm <插件>/<场景>                 # 清理并实测残留
```

`start` 之后环境在哪里、怎么连进去，由插件决定，它会告诉你。拿自带的 K8s 插件举例：

```bash
chaoslab start k8s/pod-pending
export KUBECONFIG=~/.lab/k8s-state/k8s-pod-pending-1.kubeconfig
kubectl get pods -A
```

修好之后环境还在，可以进去复盘，`stop`（或 `rm`）才会删掉。

## 用法

| 命令 | 说明 |
|---|---|
| `chaoslab list [--json]` | 所有已装插件的场景 |
| `chaoslab start <插件>/<场景> [--参数=值] [--help] [--json]` | 起环境，`--help` 看该插件的参数表 |
| `chaoslab check <引用> [--watch] [--json]` | 判定是否修好，`--watch` 每 3 秒轮询 |
| `chaoslab stop <引用> [--json]` | 清理并实测残留，别名 `rm` |
| `chaoslab stop --all` | 停掉全部，别名 `rm --all` |
| `chaoslab status [<实例>] [--json]` | 活跃实例 |
| `chaoslab plugin install <目录\|git-url>` | 装插件 |
| `chaoslab plugin uninstall <名字>` | 卸插件，有存活实例时拒绝 |
| `chaoslab plugin info <名字>` | 看插件支持的参数和场景 |
| `chaoslab plugin validate <目录>` | 写插件时自检，不执行 `up` |
| `chaoslab version [--json]` | 版本与构建信息 |

`<引用>` 可以写 `插件/场景`（该场景只有一个存活实例时），也可以写完整实例 ID `插件/场景#2`。

状态和行为记录都在 `~/.lab/` 下，用 `CHAOSLAB_HOME` 可以整个挪走，测试和评测常用这个。

退出码：

| 码 | 含义 | 你该做什么 |
|---|---|---|
| `0` | 修好了 / 命令成功 | — |
| `1` | 用法错误、场景不存在、资源冲突 | 改命令 |
| `2` | 还没修好 | 继续排查 |
| `3` | 场景本身坏了（插件出错、协议不合规、检查超时） | 去修插件 |

把 `2` 和 `3` 分开，是因为前者该继续排查，后者该去修插件。

不同场景的实例可以并行跑。冲突靠插件声明的 claim 检测，撞了会告诉你是谁占了什么；同一场景的第二个实例会被拒绝，除非先停掉第一个。

## 插件

插件是一个可执行文件加它需要的一切，任意语言实现。框架只调 5 个子命令（`schema` / `list` / `up` / `down` / `check`），不关心内部怎么组织。

| 插件 | 说明 | 文档 |
|---|---|---|
| `k8s` | 五个 Kubernetes 故障场景，每个实例一套一次性 kind 集群 | [plugins/k8s/README.md](plugins/k8s/README.md) |
| `fake` | 零依赖测试替身，也是写插件的最小参考 | [plugins/fake/README.md](plugins/fake/README.md) |

每个插件自带 README，讲它有哪些场景、各埋了什么坑、需要什么依赖、参数怎么用。

想接 MySQL、Redis、ES、Mongo，或者用 docker compose 起一套环境，都走同一套协议：

- [docs/plugin-protocol.md](docs/plugin-protocol.md)：子命令、执行环境、claims、bash 与 Python 骨架、三个常见错误
- 参考实现：[plugins/fake](plugins/fake/README.md)（最小）、[plugins/k8s](plugins/k8s/README.md)（真实场景）

```bash
chaoslab plugin validate ./myplugin   # 静态检查，不执行 up
chaoslab plugin install ./myplugin
```

## Agent 评测

起场景、把入口交给 Agent、由系统判定它修好没有。所有命令都有 `--json`：

```bash
chaoslab start <插件>/<场景> --json    # {"instance":"...","slug":"...","phase":"running"}
chaoslab status --json
chaoslab check <插件>/<场景> --json    # {"passed":true,"report":{...}}
chaoslab stop <插件>/<场景> --json     # {"released":true,"verdicts":[...]}
```

每次运行在 `~/.lab/runs/` 留一条 JSON：

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

`result` 是 `passed` / `timeout` / `abandoned` / `dirty` / `start_failed` 之一，`duration_sec` 是从起到修好的耗时。成功率、平均修复时长由评测程序自己遍历这个目录算，框架不提供统计命令。

## 实现

```
chaoslab (Go 单二进制)                 plugin (任意语言)
  ├── 发现插件                          ├── 决定用什么环境
  ├── 解析参数（schema 声明）            ├── 自己管理依赖
  ├── 检测冲突（claims 比对）            └── 自己负责清理
  ├── 调用 up / down / check
  ├── 记录状态（文件锁 + 原子写）
  └── 实测残留
```

框架不创建也不删除任何环境资源，失败的 `down` 也不重试，只协调、观察、记录。

实例有两个字段：`phase`（`starting` / `running` / `passed` / `stopped` / `dirty`）是给人看的事件结果，`live` 表示是否还占着资源。修好之后环境还在，你要能进去复盘，所以 `live` 仍是 `true`；只有 `down` 成功且残留复查通过才置 `false`。冲突检测、重复启动拦截、卸载拦截都看 `live`。

出问题的时候：

| 情况 | 行为 |
|---|---|
| `up` 失败或超时 | 标 `stopped` 但保留 `live=true`，可能留了资源，提示你 `stop` 收尾 |
| `down` 失败 | 标 `dirty`，保留原因，不重试 |
| `down` 说成功但资源还在 | 实测抓出来，标 `dirty` 并列出残留 |
| 框架进程被 `kill -9` | 下次任意命令惰性收敛，`stop` 能收尾 |
| claim 前缀框架不认识 | 参与冲突检测但不复查，`stop` 时标注「由插件保证」 |

清理永远是插件的 `down`，框架只负责验证它到底清没清干净。

## 开发

```bash
go build ./...
go test ./...
go test ./... -race

bash plugins/fake/plugin_test.sh  # 插件协议自测
bash plugins/k8s/plugin_test.sh
bash scripts/e2e-k8s.sh           # K8s 插件端到端验收，需要 kind
```

```
cmd/chaoslab/          入口
internal/cli/          每个子命令、实例解析、输出格式
internal/plugin/       插件协议：子进程调用、schema/list 解析、安装
internal/state/        实例状态，文件锁 + 原子写
internal/claims/       冲突检测与残留实测
internal/runlog/       run 记录
internal/home/         ~/.lab 位置，可用 CHAOSLAB_HOME 覆盖
plugins/               自带插件
scripts/               安装脚本与 K8s 端到端验收
```

发布由 tag 触发：推 `v*` tag 会先跑测试，再交叉编译四个平台、打包 `plugins/`、生成 `SHA256SUMS` 并创建 release，见 [.github/workflows/release.yml](.github/workflows/release.yml)。

## 状态

早期版本，接口还在动。

已验证：macOS + OrbStack 上五个 K8s 场景依次跑通 `start → check(未修=2) → stop → 无残留`，同一场景连续三轮零残留。

未验证：Docker Desktop 和 Linux 上的 kind 行为；`--kubeconfig` 复用已有集群这条路径没有实跑过。
