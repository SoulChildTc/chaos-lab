# chaos-lab

本地可跑的故障演练环境：一条命令起环境、故障已埋好，你自己进去排查修复，修好由系统判定。

不是教程，不是模拟器。环境是真的，故障是真的，判据是实测的。修好之后给你根因复盘，也给你一条可验证的「环境已清干净」结论。

```
$ chaoslab start k8s/ingress-502
k8s: 创建集群 lab-k8s-ingress-502-1（镜像 kindest/node:v1.31.0）
k8s: 集群就绪（30s）
k8s: 故障已就绪
现象：访问站点返回 502
k8s/ingress-502#1 已就绪（slug k8s-ingress-502-1）
修好后运行：chaoslab check k8s/ingress-502#1
```

现在进集群自己查。查到了、改对了：

```
$ chaoslab check k8s/ingress-502
{"passed":true,"report":{"passed":true,"root_cause":"Service web 的 targetPort 是 8080，而容器实际监听 80……","clues":[…]}}
```

没修好时退出码 2，修好了是 0，检查本身出错是 3。

## 安装

需要 Go 1.22+ 与 Docker：

```bash
go build -o ~/.local/bin/chaoslab ./cmd/chaoslab
chaoslab plugin install ./plugins/k8s
```

K8s 场景还需要 `kind` 和 `kubectl`（插件自己会检查并给安装提示）。

## 五分钟上手

```bash
chaoslab list                           # 有哪些场景
chaoslab start k8s/pod-pending          # 起一个
chaoslab status                         # 它是什么状态
# …… 自己进集群查，比如：
#     kubectl --kubeconfig ~/.lab/k8s-state/k8s-pod-pending-1.kubeconfig get pods -A
chaoslab check k8s/pod-pending --watch  # 一边修一边让它轮询判定，最长 20 分钟
chaoslab stop k8s/pod-pending           # 清理，并实测是否真的清干净了
```

`start` 的 stderr 只给现象不给答案。修好后的 `check` 才告诉你根因和关键线索。

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
| `chaoslab plugin info <名字>` | 看插件支持哪些参数和场景 |
| `chaoslab plugin validate <目录>` | 写插件时自检 |

`<引用>` 可以是 `插件/场景`（该场景只有一个存活实例时）或完整实例 ID `插件/场景#2`。

退出码：`0` 成功/通过，`1` 用法或冲突，`2` 未通过，`3` 插件执行出错。

## 自带插件

| 插件 | 说明 | 文档 |
|---|---|---|
| `k8s` | 五个 Kubernetes 故障场景，每个实例一套一次性 kind 集群 | [plugins/k8s/README.md](plugins/k8s/README.md) |
| `fake` | 零依赖测试替身，验证框架行为，也是写插件的参考实现 | [plugins/fake/README.md](plugins/fake/README.md) |

```bash
chaoslab plugin install ./plugins/k8s
chaoslab list          # 这时会列出 5 个 K8s 场景
```

K8s 插件需要 `kind` 和 `kubectl`（插件自己检查并给安装提示）。每个场景的现象、埋的坑、进环境的方式、参数、清理行为，都写在那个插件的 README 里。

## 并行跑

不同场景可以同时跑。框架用插件声明的 claims 检测冲突——端口、Docker 资源、kind 集群、目录。撞了会明确告诉你是谁占了什么：

```
$ chaoslab start fake/other --port=18080
错误：fake/env#1 已经占用 port:18080
```

同一场景不能同时跑两次，先 `stop`。

## 环境清不干净的时候

框架不替你清理，清理永远是插件的 `down`。但框架会**实测**：`down` 说成功后，它自己去查端口、容器、集群、目录是否真的没了。

没清干净，实例标为 `dirty` 并列出残留，同场景再 `start` 会被拦住——宁可告诉你「没清干净」，也不假装清了。

## 给 Agent 能力评测用

所有命令都有 `--json`，`start` 会直接返回实例 ID，不需要解析人类文案。

每次运行结束会在 `~/.lab/runs/` 写一条 JSON：`result` 是 `passed` / `timeout` / `abandoned` / `dirty` / `start_failed`，`duration_sec` 是从起到修好的耗时。

```bash
chaoslab start k8s/ingress-502 --json        # {"instance":"k8s/ingress-502#1",...}
chaoslab status --json                       # 当前存活实例
chaoslab check k8s/ingress-502 --json        # {"passed":true,"report":{...}}
```

评测程序遍历 `~/.lab/runs/*.json`：`passed` 计数做分子，全部记录做分母。

## 写自己的插件

插件是任何语言写的一个可执行文件，框架对它内部零假设。MySQL、Redis、ES、Mongo 都可以这么做。

- **协议文档**：[docs/plugin-protocol.md](docs/plugin-protocol.md) —— 子命令、执行环境、claims、两份骨架代码、三个常见错误
- **参考实现**：[plugins/fake](plugins/fake/README.md)（零依赖最小例子）、[plugins/k8s](plugins/k8s/README.md)（真实场景）

每个插件自带 README，讲它自己的场景、参数和清理行为。

## V1 边界

- 没有图形界面，没有多用户
- 不碰真实生产集群（除非你用 `--kubeconfig` 显式指向它，且只写一个 namespace）
- `~/.lab/runs/` 只写不读，没有统计命令
- 不做插件版本管理和升级
- 目前只有 K8s 插件

## 目录

```
cmd/chaoslab/          入口
internal/cli/          每个子命令
internal/plugin/       插件协议：子进程调用、schema/list 解析、安装
internal/state/        实例状态，文件锁 + 原子写
internal/claims/       冲突检测与残留实测
internal/runlog/       run 记录
plugins/k8s/           K8s 插件（bash + kind），自带 README
plugins/fake/          测试替身插件，自带 README
docs/                  设计文档、实施计划、插件协议
scripts/e2e-k8s.sh     五个 K8s 场景的端到端验收
```
