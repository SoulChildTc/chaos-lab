> 状态：初版设计。后续有重大修订：实例从「插件/场景」改为「整个插件」一个实例，`up`/`down`/`check` 不再接受场景参数，`start` 输出插件提供的 symptom 与 connect。现行行为以 README 与 docs/plugin-protocol.md 为准，本文仅记录最初的设计取舍。

# chaos-lab 设计文档

日期：2026-11-20
状态：待评审

## 1. 意图

一套本地可跑的故障演练环境：一条命令起环境、故障已埋好，使用者在里面自己排查修复，修好由系统判定。

框架本身**不提供任何环境**。它只做编排：发现插件、解析参数、调用插件的生命周期动作、检测冲突、记录状态、渲染文案。起什么环境、怎么起、怎么清、算不算修好，全部由插件定义。

这是本设计的核心立场。框架与具体技术栈零耦合，加 MySQL / Redis / ES / Mongo 场景不需要改框架一行代码。

## 2. 目标

- 起环境的复杂度（时长、依赖、镜像）由插件承担，框架只负责超时与上报
- 插件文档是唯一接口：别人或 AI 只读一份协议文档就能写出可用插件
- 多个场景实例可以并行运行，冲突可检测、可解释
- 单场景反复失败重试不残留环境，且“是否残留”可被框架独立观察和证明
- Agent 能力评测可作为消费者：所有命令有 `--json`，结果按实例落盘可统计

## 3. 非目标（V1）

- 不做图形界面
- 不做多用户 / 多租户 / 远程服务端
- 不往真实生产集群注入故障
- 不实现 MySQL / Redis / ES / MongoDB 场景，只预留接口并在文档说明扩展方式
- 不提供结果查询 / 统计命令，`~/.lab/runs/` 只存不查
- 不做插件版本管理、分支 / tag 安装、强制覆盖

## 4. 反目标

- 框架内置任何一种环境底座的知识（K8s、Docker、云）。一旦框架出现 `if k8s` 这类分支，设计就失败了。
- 框架替插件收拾残局。框架观察到未释放就如实记录并阻止继续，**不尝试修复**。
- 插件为了适配框架改变自己的内部结构。插件内部随便组织，框架只认一个入口。

## 5. 约束

- 框架用 Go 写，产物是单二进制 `chaoslab`
- 插件是一个可执行文件，任意语言实现
- 协议走子进程一次性调用，无长驻进程、无 RPC、无 socket
- 本机必须有 Docker（插件自检自己的额外依赖，框架不管）
- 用户目录 `~/.lab/` 是唯一本地状态位置

## 6. 术语

| 术语 | 含义 | 类比 Terraform |
|---|---|---|
| 插件 (plugin) | 可安装单元，一种环境底座 | Provider |
| 场景 (case) | 插件内的一个具体故障 | Resource |
| 实例 (instance) | 一次运行中的场景 | Resource instance |
| claims | 场景运行时占用的外部资源声明 | — |

用户看到的是 `k8s/ingress-502`（插件/场景）。实例 ID 是 `k8s/ingress-502#1`。

## 7. 方案选择记录

| 议题 | 选项 | 选定 | 理由 |
|---|---|---|---|
| 场景定义形式 | 纯声明式 YAML（up/down/check 写成 shell 片段） vs 可执行入口 | 可执行入口 | 多步骤、条件分支、结构化失败上报写不进单段 shell；AI 生成 Python/bash 比生成巨型单行 shell 可靠 |
| 框架与插件通信 | 子进程一次性调用 vs 长驻进程 + gRPC | 子进程 | gRPC 的价值是长期状态机、并发加锁、plan/apply 分阶段，本设计全不需要；长驻引入 socket、僵尸进程、超时清理等纯自找的问题 |
| 框架如何知道插件内部结构 | 遍历 `cases/*/up.sh` vs 插件自己汇报 | 插件自己汇报 | 遍历目录等于把文件布局写进框架；汇报让插件可动态判断场景是否可用 |
| 一个插件几个场景 | 5 个故障 = 5 个插件 vs 1 个 k8s 插件 5 个场景 | 1 插件多场景 | 少重复；kind 镜像与依赖共用，第 2 个场景起得更快 |
| 元数据存放 | `plugin.yaml` + schema 子命令 vs 只有子命令 | 只有子命令 | 删掉 `plugin.yaml` 后框架对插件内部零假设，只有一份真相 |
| 多实例 | 单活跃 vs 并行 | 并行 | 评测和培训场景需要；靠 claims 检测冲突 |
| 同插件两个场景同跑 | 同集群注入两个故障 vs 各自集群 | 各自独立 | 塞同一集群时停一个会伤另一个，故障互相干扰（改 StorageClass 会波及 ingress 案例） |
| 重复启动同一场景 | 允许换参数再起 vs 拒绝 | 拒绝 | 提示已运行实例与时长，先 stop |

## 8. 架构

```
chaoslab (Go 二进制)
  ├── 插件发现与安装    ~/.lab/plugins/<name>/plugin
  ├── 参数解析与校验    schema 声明的声明式参数
  ├── 冲突检测          claims 比对
  ├── 生命周期调用      up / down / check 子进程
  ├── 状态与记录        ~/.lab/state/、~/.lab/runs/
  └── 文案渲染          现象、通过提示、根因复盘

plugin (可执行文件，任意语言)
  ├── 决定用什么环境    kind / docker compose / 远端 kubeconfig / 别的
  ├── 自己管理依赖自检  缺 kind → 报错并给安装提示
  ├── 自己组织内部文件  框架不管
  └── 自己负责清理干净  down 幂等且可在任意中间状态执行
```

框架不做的事：不创建资源、不删除资源、不修复、不重试失败的 down、不推断插件内部状态。

## 9. 插件协议

### 9.1 安装位置

```
~/.lab/plugins/
  k8s/
    plugin          # 必须有，必须可执行，名字固定
    (插件内部文件随意)
```

插件目录名由安装时读取 `schema` 的 `name` 决定。`name` 必须匹配 `^[a-z0-9][a-z0-9-]*$`。

### 9.2 五个子命令

所有子命令都是进程一次性执行。**stdout 只放协议数据，stderr 只放给用户看的进度**。框架把 stderr 实时透传，把 stdout 按子命令语义解析。

```
plugin list   --json                        # 汇报场景清单
plugin schema --json                        # 汇报插件级参数声明
plugin up     <case> --instance=<slug> --args=@<file>
plugin down   <case> --instance=<slug> --args=@<file>
plugin check  <case> --instance=<slug> --args=@<file>
```

`--instance` 的值是框架生成的 slug（见 9.6），已满足常见资源命名要求（小写字母数字与 `-`）。

`--args` 指向一个临时 JSON 文件，内容是**已补默认值、已校验**的对象。框架在调用结束后删除该文件。不散成环境变量，避免类型和特殊字符处理。

**执行环境约定**（插件必须能依赖）：

| 项 | 值 |
|---|---|
| 工作目录 | 插件目录 `~/.lab/plugins/<name>/`，插件可用相对路径引用自身文件 |
| stdin | 接到 `/dev/null` |
| stdout | 只放协议数据 |
| stderr | 只放给人看的进度，框架实时透传 |
| `LAB_INSTANCE` | 同 `--instance` |
| `LAB_PLUGIN_DIR` | 插件目录绝对路径 |
| `LAB_CASE` | 同 `<case>` |

超时到达时框架对**整个进程组**先 `SIGTERM`，5 秒后 `SIGKILL`。插件派生的子进程（如 `kind create cluster` 带出的 docker 调用）一并终止，避免留下孤儿进程导致清理不干净。

### 9.3 `list --json`

```
plugin list --json [--instance=<slug>] [--args=@<file>]
```

`--instance` 与 `--args` 可选。框架在 `start` 时两者都传，插件据此返回**已渲染好的** claims。不传时按默认值渲染（`validate` 用）。

```json
{
  "cases": [
    {
      "id": "ingress-502",
      "title": "站点访问返回 502",
      "claims": ["kind:lab-k8s-ingress-502-1"],
      "up_timeout": "10m",
      "check_timeout": "10m"
    }
  ]
}
```

- `id`：场景标识，`^[a-z0-9][a-z0-9-]*$`
- `title`：一句话现象标题
- `claims`：**字符串数组，已渲染完毕**。框架只做精确相等比对，不解析、不渲染（见 9.7）
- `up_timeout` / `check_timeout`：可选，框架默认 `5m` / `10m`。这是插件自己对自己起环境耗时的判断，框架不猜。

### 9.4 `schema --json`

```json
{
  "name": "k8s",
  "version": "0.1.0",
  "description": "Kubernetes 故障演练，每个实例一个临时 kind 集群",
  "args": [
    {
      "name": "node-image",
      "type": "string",
      "default": "kindest/node:v1.31.0",
      "help": "kind 节点镜像"
    },
    {
      "name": "kubeconfig",
      "type": "string",
      "help": "复用已有集群的 kubeconfig；给出时插件不创建集群，也不删除任何资源"
    }
  ]
}
```

`type` 支持 `string` / `int` / `bool`。`default` 可选。框架据此生成 `--help`、做类型校验、拒绝未知参数。**插件不接触原始 argv。**

插件级参数与场景级参数：V1 只有插件级。场景级参数留接口位置但不实现（避免两套合并规则）。

### 9.5 场景目录内文案

场景的现象描述、修好后的通过提示、根因说明由插件在对应时机输出到 stderr（给人看）和 `check` 的 JSON（给框架记录）。框架不做模板渲染，避免出现第二套模板语言。

具体来说：
- `up` 的 stderr：环境就绪提示 + 现象描述（例如“站点访问返回 502”）。不给答案。
- `check` 通过时 stdout 吐报告（见 9.8），stderr 打印通过提示与根因复盘。
- 框架不修改这些文本，只透传和存档。

### 9.6 实例 slug

实例 ID（人看）`k8s/ingress-502#1`，slug（给插件用）`k8s-ingress-502-1`。
生成规则：`<plugin>-<case>-<n>`，全部小写，非 `[a-z0-9-]` 字符替换为 `-`。序号按 `<plugin>/<case>` 单调递增，持久化在状态文件里，不复用已 stopped 的序号（避免遗留资源撞名）。

### 9.7 claims

声明式字符串，**由插件渲染好后返回**，框架只做精确相等比对。

`list` 收到 `--instance` 与 `--args` 后自行决定返回哪些 claim。例子：`k8s` 插件在 `kubeconfig` 参数为空时返回 `kind:lab-<slug>`，非空时返回 `kube:<context>/<namespace>`——因为复用时它不创建集群，也不该声明集群所有权。模板、条件、默认值全部由插件处理，框架没有任何模板语言。

框架内置四类校验器（用于 `stop` 后复查，见 10.4）：

| 前缀 | 复查方式 |
|---|---|
| `port:<n>` | 本机该 TCP 端口无监听 |
| `docker:<container\|network\|volume>:<name>` | `docker inspect` 不存在 |
| `kind:<name>` | `kind get clusters` 不含此项 |
| `dir:<path>` | 路径不存在 |

其他前缀（如 `kube:ns/prod`、`remote:jenkins`）允许声明，参与冲突比对，但框架不复查——插件自己保证。这是显式的表达力边界，写进协议文档，`plugin validate` 对未知前缀给提示而非报错。

### 9.8 check 的退出码与报告

| 退出码 | 含义 |
|---|---|
| 0 | 已修好 |
| 1 | 尚未修好 |
| 2 | 检查本身出错（如集群连不上） |

退出码 0 时 stdout 必须是一个 JSON 对象：

```json
{
  "passed": true,
  "root_cause": "Ingress 的 backend service 端口写成 8080，实际容器监听 80",
  "clues": ["Service endpoints 为空", "Ingress 事件报 connection refused"]
}
```

退出码 1 时 stdout 可为空，也可给 `{"passed": false, "message": "..."}`。退出码 2 时 stderr 必须说明原因。

框架不解析根因文本，原样存进 run 记录。

## 10. 框架行为

状态分两个独立字段，解决“阶段”和“是否还占着资源”混在一起的问题：

- `phase`——`starting` / `running` / `passed` 是人关心的事件结果
- `live`——布尔，实例是否仍持有环境资源

`passed` 的实例 `live=true`（集群还在，用户还没来得及 `stop`）。只有 `down` 成功且 claims 复查全绿才置 `live=false`。`stop` / 冲突检测 / `uninstall` 判断“活跃”一律看 `live`，不看 `phase`。

### 10.1 `start`

1. 校验插件已安装、场景存在于 `list` 输出
2. 解析参数（schema 默认值 + 用户传入），拒绝未知参数与类型不符
3. 分配 slug（需要序号，所以先加锁取号）
4. **加锁 → 读状态 → 分配序号 → 写入 `phase=starting, live=true, pid=<自身 pid>` → 解锁**。锁只覆盖这一段（毫秒级），不跨插件调用，否则并行实例全被串行化
5. 带 `--instance` 与 `--args` 调 `plugin list`，取该场景**已渲染的** claims
6. 冲突检查：与所有 `live=true` 实例比对 claims；同 `<plugin>/<case>` 已有 `live=true` 实例（含 `dirty`）则拒绝，提示实例 ID 与已跑时长。冲突则回滚刚写入的记录
7. 把 claims 写进实例记录
8. 执行 `plugin up`，超时按 `up_timeout`，超时杀进程组
9. 成功 → 加锁写入 `phase=running`；失败或超时 → `phase=stopped`，`live=true`（可能留了资源），把 stderr 抛给用户

失败时框架不尝试清理。提示用户 `chaoslab stop <id>` 让插件自己 `down`。

**崩溃恢复**：若框架自身被杀，会留下 `phase=starting` 且 pid 已不存在的记录。任何命令读状态时先做一次惰性检查——`phase=starting` 且 pid 不存在 → 标记为 `stopped` / `live=true` / `error="启动过程被中断"`。不做后台守护进程，也没有 `--force`；用户看到后走 `stop`。

### 10.2 `check`

- 单次：执行一次 `plugin check`，超时 `2m`（单次调用上限，与 `check_timeout` 无关）
- `--watch`：每 3 秒执行一次，直到通过或累计到 `check_timeout`。每次调用都是独立子进程
- 退出码 0 → `phase=passed`（`live` 保持 true），写 run 记录 `result=passed`，附根因与线索
- `--watch` 超时 → `phase` 保持 `running`，写 run 记录 `result=timeout`
- 退出码 2 → 向用户报错，不计入尝试次数，不写 run 记录

`passed` 之后环境仍然存在，用户还要能进去复盘。清理只能由 `stop` 完成。

### 10.3 `stop`

1. 解析实例：可给 `<plugin>/<case>`（该场景有多个 `live` 实例时要求写全 `#n`）
2. 执行 `plugin down`，超时 `up_timeout`，超时杀进程组
3. 成功 → 复查 claims（见 10.4）
   - 全释放 → `phase=stopped`，`live=false`，从活跃列表移除，序号不复用
   - 有残留 → `phase=dirty`，`live=true`，逐项列出未释放资源
   - 未知前缀 → 视为插件负责，不算残留，但在输出中标注“未复查”
4. `down` 非零退出 → `phase=dirty`，`live=true`，保留插件 stderr。框架不重试、不代劳

### 10.4 claims 复查

只观察，不修复。内置前缀（`port:` / `docker:` / `kind:` / `dir:`）实测，未知前缀跳过并标注“由插件负责，框架未复查”。结果写入 run 记录与 `status`。

这条是“连续失败重试不残留”唯一可证据化的落点。

### 10.5 状态文件

`~/.lab/state/instances.json`，读写加文件锁（`flock` 语义，锁文件 `instances.lock`）。结构：

```json
{
  "seq": {"k8s/ingress-502": 1},
  "instances": [
    {
      "id": "k8s/ingress-502#1",
      "plugin": "k8s",
      "case": "ingress-502",
      "slug": "k8s-ingress-502-1",
      "args": {"node-image": "kindest/node:v1.31.0"},
      "claims": ["kind:k8s-ingress-502-1"],
      "phase": "running",
      "live": true,
      "pid": 48213,
      "started_at": "2026-11-20T12:00:00Z",
      "checks": 3,
      "last_check": "2026-11-20T12:04:11Z",
      "error": null
    }
  ]
}
```

`live=false` 的实例保留在文件中供人查看，序号不复用。

### 10.6 run 记录

`~/.lab/runs/<started_at>_<slug>_<seq>.json`，每个实例最多一条，`<seq>` 防重名。写入时机是实例生命周期的终点：

| 场景 | `result` |
|---|---|
| `check` 通过 | `passed` |
| `--watch` 到 `check_timeout` | `timeout` |
| 用户 `stop` 但从未通过 | `abandoned` |
| `stop` 后 claims 有残留或 `down` 失败 | `dirty` |
| `start` 失败或超时 | `start_failed` |

```json
{
  "instance": "k8s/ingress-502#1",
  "plugin": "k8s",
  "case": "ingress-502",
  "args": {},
  "started_at": "...",
  "ended_at": "...",
  "duration_sec": 412,
  "result": "passed",
  "checks": 9,
  "report": {"passed": true, "root_cause": "...", "clues": ["..."]},
  "claims_released": true
}
```

`passed` 记录在通过那一刻写（`duration_sec` 是从 `start` 到通过，即修复耗时）；之后 `stop` 只更新同一文件里的 `claims_released` / `ended_at`，不新增记录。其余 `result` 在对应事件发生时写。

V1 只写不读。评测程序遍历此目录，`passed` 为分子，全部记录为分母。

## 11. CLI 表面

```
chaoslab list [--json]
chaoslab start <plugin>/<case> [--arg=value ...] [--json] [--help]
chaoslab stop (<plugin>/<case> | <instance-id>)
chaoslab status [<instance-id>] [--json]
chaoslab check (<plugin>/<case> | <instance-id>) [--watch] [--json]
chaoslab plugin install <git-url|dir>
chaoslab plugin uninstall <name>
chaoslab plugin info <name>
chaoslab plugin validate <dir>
```

- `<plugin>/<case>` 在某场景只有一个 `live` 实例时可直接引用，多个时必须写 `#n`
- `start --help` 打印场景标题、现象一句话、参数表（由 `schema` 生成）
- `start --json` 输出 `{"instance": "k8s/ingress-502#1", "slug": "...", "phase": "running"}`——评测程序靠它拿实例 ID，不需要解析人类文案
- `list --json` 给 `{plugin, case, title}` 数组
- `status --json` 给 `live=true` 实例数组（字段同状态文件）
- `check --json` 给最后一次判定 `{passed, report?, message?}`，与退出码语义一致
- `stop` 无 `--json`（V1）：评测程序用 `status --json` 判断是否释放
- 退出码：`0` 成功/通过，`1` 用户错误（不存在、冲突、参数错），`2` 未通过，`3` 插件执行出错

### 11.1 plugin install

1. `dir`：读取目录；`git-url`：浅克隆到临时目录
2. 确认 `plugin` 存在且可执行（不可执行则 `chmod +x`，仍失败则报错）
3. 执行 `plugin schema --json`，解析 `name`，校验命名规则
4. 执行 `plugin list --json`，校验结构（这一步同时验证插件协议实现正确）
5. `~/.lab/plugins/<name>` 已存在 → 报错“已安装”，**不覆盖**
6. 移动到目标位置

失败时清理临时目录，不留半装状态。

### 11.2 plugin uninstall

该插件有 `live=true` 实例（含 `dirty`、含 `passed` 未清理）时拒绝，提示 `stop`。否则删目录。

### 11.3 plugin validate

给插件作者（尤其是 AI）用，只读不执行 `up`：
- `plugin` 可执行
- `schema` / `list` 输出符合协议、字段合法
- 场景 id 唯一、命名合法
- `list` 在无参数、以及每个参数取默认值两种情况下都能跑通（参数无默认值则只用无参数那次）
- claims 是字符串数组，前缀是否已知（未知 → warning）

真实校验靠 install 后 `start`。

## 12. V1 插件：`plugins/k8s`

放在框架仓库内，`chaoslab plugin install ./plugins/k8s` 装上。

**实现语言：bash**。理由：安装无需编译步骤，插件作者门槛最低，`kind` / `kubectl` 本来就是命令行工具；框架本身用 Go，不受影响。

**每个实例一套临时 kind 集群**，集群名 `lab-$LAB_INSTANCE`，`down` 里 `kind delete cluster`。不复用本机已有集群（如 OrbStack），不加 namespace，确保零污染、可完整回收。集群级资源、CRD、PV 都不会留在本机任何长期集群里。

**依赖自检**：`kind` / `kubectl` / `docker` 缺失时 `up` 直接非零退出并在 stderr 给安装提示。这是插件的事，框架不探查。

**五个场景**：

| id | 现象 | 埋的坑 |
|---|---|---|
| `ingress-502` | 访问站点返回 502 | Service 的 targetPort 与容器端口不符 |
| `pod-pending` | Pod 一直 Pending | resources.requests 超过节点可分配 |
| `image-pull-backoff` | Pod 卡在 ImagePullBackOff | 镜像 tag 不存在 |
| `svc-selector-mismatch` | Service 无 endpoints，访问超时 | selector 与 Pod label 不匹配 |
| `pvc-unbound` | PVC 一直 Pending | StorageClass 不存在 |

manifest 与故障注入逻辑放各 case 目录，共享的集群创建 / 等待 / 清理放插件内 `lib.sh`，由 case 自行 source——框架不管插件内部如何组织。

**已知风险**：kind 首次拉节点镜像受网络影响，可能超过 5 分钟；`up_timeout` 默认给 10m 由插件声明。反复起集群会占内存，多实例并行需机器有余量。

**测试用插件**：仓库内 `plugins/fake/`，纯 bash，零依赖，每个实例只 `mkdir` 一个目录并监听一个端口（claim 声明 `dir:` 与 `port:`）。用途是让框架的冲突检测、claims 复查、`dirty` 流程能在无 Docker 的机器上自动化测试，也当协议示例。它不是产品插件，`install` 后同样可用。

## 13. 扩展方式（写入 `docs/plugin-protocol.md`）

文档内容就是本设计第 9、10 章，外加一份可复制的骨架（bash 与 Python 各一）和三个反例：down 不幂等、check 用 `set -e` 把未修好误判成检查出错、claims 写死实际资源名导致冲突漏检。

将来接 MySQL / Redis / ES / Mongo：写新插件，内部用 docker compose 起实例，`down` 里 `docker compose down -v`，claims 声明 `docker:network:` 与 `port:`。核心零改动。

AI 写插件另配 skill，skill 从同一份协议文档派生，避免两者漂移。

## 14. 留给 builder 的决定

- 锁的具体实现（`flock` / `O_EXCL` 锁文件）
- 状态文件版本字段与迁移策略（V1 可无）
- kind 集群就绪的等待策略（`kubectl wait` 还是轮询 node ready）
- 五个场景的 manifest 细节与镜像选择
- `--watch` 轮询间隔是否做成配置

## 15. 验收

- `chaoslab list` 在只装 k8s 插件时列出 5 个场景
- `chaoslab start k8s/ingress-502` 起环境、只出现象不给答案
- 手动修好后 `chaoslab check k8s/ingress-502` 退出码 0，写出 run 记录，含根因与线索；此时 `status` 仍显示该实例 `live=true`，`plugin uninstall k8s` 被拒
- `chaoslab start --json` 输出的 instance id 能直接喂给 `status` / `check` / `stop`
- 未修好时 `check` 退出码 2，实例保持 `running`
- `chaoslab stop` 后 `kind get clusters` 里无 `lab-*`
- 同一场景连续三次 `start` → 修坏 → `stop`，每次 `stop` 后 claims 复查全绿
- 用 `plugins/fake` 起两个实例，第三个声明相同 `port:` 的被拒并说明撞了哪个实例的哪个 claim
- 同一场景重复 `start` 被拒并提示实例 ID
- 用 `plugins/fake` 故意让 `down` 失败（它的 `--fail-down` 参数），实例标 `dirty`、`live=true`，`status` 列出未释放资源，且同场景再 `start` 与 `uninstall` 都被拦
- `kill -9` 框架进程于 `up` 期间，下次任意命令把该实例标为 `stopped` / `live=true` / 中断原因，`stop` 能正常收尾
- 全部命令 `--json` 输出可被脚本消费
