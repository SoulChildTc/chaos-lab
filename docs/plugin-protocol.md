# chaos-lab 插件协议

这份文档是你写一个 chaos-lab 插件需要的全部。读完应当能写出可用的插件，不需要看框架代码。

框架是纯编排器：环境由插件创建，也由插件销毁。框架只调用、检测、记录、渲染。

---

## 1. 插件是什么

一个目录，里面有一个名为 `plugin` 的可执行文件。任何语言都行：bash、Python、Go 编译出的二进制。

```
myplugin/
  plugin            # 必须有，必须可执行，名字固定
  (其余文件随你组织)
```

安装：`chaoslab plugin install ./myplugin` 或 `chaoslab plugin install <git-url>`。
（这个仓库自带的两个插件最后有说明，请优先复制它们。）

---

## 2. 一个插件，多个故障

```
myplugin/
  plugin
  faults/
    slow-query/
    wrong-port/
```

`chaoslab list` 列出的是 `myplugin` 能注入的故障清单（`slow-query`、`wrong-port`）。这份清单是给用户看的目录，不是菜单：`chaoslab start myplugin` 会**一次注入全部故障**。

一个实例 = 一次运行，ID 是 `myplugin#1`。同一个插件不允许同时有两个存活实例——它们的资源会撞车，而且 `check` 也无法判断你修的是哪一份。

---

## 3. 五个子命令

框架以子进程方式调用 `plugin`，每次调用都是一个新进程，调用之间没有任何常驻状态。所有状态必须放在磁盘或真实环境里。

```
plugin schema --json
plugin list   --json [--instance=<slug>] [--args=@<file>]
plugin up            --instance=<slug> --args=@<file>
plugin down          --instance=<slug> --args=@<file>
plugin check         --instance=<slug> --args=@<file>
```

There is no scenario argument. A plugin decides which faults it injects, so
`up` injects **all of them**; `check` judges exactly what `up` put there; `down`
tears the whole environment down. The user never picks individual faults on the
command line — that is the plugin's call, and it is also what keeps the exercise
honest: you are told a symptom, not a list of faults.

### 3.1 Environment

| 项 | 值 |
|---|---|
| 工作目录 | 插件目录（可用相对路径引用自身文件） |
| stdin | `/dev/null` |
| stdout | **只放协议数据**（JSON 或 check 的报告） |
| stderr | **只放给人看的进度**，框架实时透传 |
| `LAB_INSTANCE` | 实例 slug，例如 `myplugin-slow-query-1` |
| `LAB_PLUGIN_DIR` | 插件目录绝对路径 |

超时到达时框架对**整个进程组** `SIGTERM`，5 秒后 `SIGKILL`。你自己派生的子进程也会一起终止——但也正因如此，**不要指望你的子进程在超时后还能活着完成任务**。

### 3.2 slug 可以直接当资源名用

slug 形如 `myplugin-1`，只含小写字母、数字、连字符。用它拼容器名、集群名、namespace、目录名是安全的，并且天然不冲突。

---

## 4. `schema --json`

```json
{
  "name": "myplugin",
  "version": "0.1.0",
  "description": "一句话说明这个插件管什么环境",
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
      "help": "复用已有集群；给出时不创建也不删除集群"
    }
  ]
}
```

- `name` 必须匹配 `^[a-z0-9][a-z0-9-]*$`，且会成为安装目录名
- `type` 只能是 `string` / `int` / `bool`
- 有 `default` 的参数是可选参数，没有的是可省略参数（不传就不出现在 `--args` 里）
- 参数表会直接变成 `chaoslab start <你的插件> --help` 的输出，写清楚

**插件收不到原始命令行。** 框架解析、校验、补默认值，然后把结果放进 `--args` 指向的 JSON 文件。

---

## 5. `list --json`

```json
{
  "scenarios": [
    {"id": "slow-query", "title": "查询一直不返回"},
    {"id": "deadlock",   "title": "下单偶发超时"}
  ],
  "claims": ["docker:container:myplugin-1", "port:3306"],
  "up_timeout": "5m",
  "check_timeout": "10m",
  "symptom": "订单接口偶尔超时，数据库连接数接近上限。",
  "connect": [
    "mysql -h 127.0.0.1 -P 3306 -u lab -p",
    "密码：lab"
  ]
}
```

- `scenarios` 是这个插件能注入的故障清单。**仅供展示**——`chaoslab list` 会把它列出来，`start` 不接受单独选择。每个 `id` 匹配 `^[a-z0-9][a-z0-9-]*$`，不可重复。
- `claims` 见第 7 节
- `up_timeout` / `check_timeout` 可选，默认 `5m` / `10m`
- **`symptom` 是必填的**：用户唯一能看到的问题描述。它必须描述**观察到的现象**（"访问返回 502"），不能点名故障（"Service targetPort 配错"）——后者是答案。
- **`connect` 是必填的**：至少一条，告诉用户怎么进到环境里。kubeconfig 路径、host/port/user/password、URL，whatever。没有连接信息用户无法开始。
- `symptom` 里提到了几处问题，`check` 通过时的 `root_cause` 就该逐一对应。`up` 的 stdout 也要输出同样结构的 JSON（`symptom` + `connect`），框架会在 `start` 结束后打印。

### 5.1 为什么 `list` 要收 `--instance` 和 `--args`

因为 claims 依赖实例名和参数。框架在 `start` 时会带着 slug 和参数调用 `list`，你按实际要占用的资源返回 claims。

举例，`--kubeconfig` 有值时你不创建集群，就不该声明集群所有权：

```bash
if [ -n "$(json_str kubeconfig)" ]; then
  echo "\"kube:$CTX/lab-$LAB_INSTANCE\""
else
  echo "\"kind:lab-$LAB_INSTANCE\""
fi
```

框架没有任何模板语言。条件、默认值、命名规则全在你手里。

---

## 6. `up` / `down` / `check`

### 6.1 `up`

创建环境并注入**全部**故障。成功退出 0，失败非 0（框架记 3）。

- **stdout 输出一份 JSON**：`{"symptom": "...", "connect": ["..."]}`。框架拿它给用户看，这是用户唯一的信息来源。**不要在里面列出注入的故障。**
- stderr 放进度日志，框架实时透传（会被用户看到，所以同样别泄露答案）
- 多个故障共用一套环境是**你的自由**：`up` 只被调用一次，你决定建一个集群还是几个、怎么隔离。框架不关心。
- 失败时框架不替你清理，实例会被标记为「持有资源但未就绪」，用户可以 `stop` 让你 `down` 一次
- **`up` 必须能从「上次失败的残留」里恢复**：先检查并删除同名残留再创建。重试是正常的

### 6.2 `down`

销毁 `up` 创建的一切。

- **必须幂等**：资源不存在时退出 0，不是错误
- **必须能在任意中间状态执行**：`up` 中途失败、`up` 超时被杀、只创建了一半，都要能清
- 清理干净才退出 0

框架在 `down` 之后会**自己实测** claims 是否真的释放了（见 7.2）。所以这里撒谎没用。

### 6.3 `check`

判断故障是否已被修复。退出码决定一切：

| 退出码 | 含义 |
|---|---|
| `0` | 已修好，stdout 必须是报告 JSON |
| `1` | 还没修好 |
| `2` | 检查本身出错（集群连不上、命令缺失） |

退出 0 时的报告：

```json
{
  "passed": true,
  "cases": [
    {"id": "slow-query", "passed": true},
    {"id": "deadlock",   "passed": true}
  ],
  "root_cause": "慢查询缺索引；死锁是两个事务加锁顺序相反",
  "clues": ["Endpoints 有地址，容易误判为正常", "curl 返回 connection refused"]
}
```

`cases` 逐条列出每个故障的判定。框架不解析它，原样存进 run 记录——评测程序靠它算单个故障的通过率。

退出 1 时也可以输出 JSON，同样带 `cases`，让用户知道**还剩哪几个**没修好：

```json
{"passed": false, "cases": [{"id": "slow-query", "passed": false}, {"id": "deadlock", "passed": true}]}
```

`root_cause` 和 `clues` 是用户修好之后看到的复盘，会在人工阅读模式下打印出来。

**`check` 只观察，不修复。** 修复是用户的事，这是本工具存在的意义。

---

## 7. claims：声明你占了什么

claims 是字符串数组，框架用它做两件事：拒绝冲突的并行实例，验证 stop 后真的清理干净了。

### 7.1 冲突检测

两个存活实例有任何相同的 claim，第二个 `start` 被拒：

```
invalid input: myplugin#1 already holds port:3306
```

框架能自己复查的四类前缀：

| 前缀 | 复查方式 |
|---|---|
| `port:<n>` | 本机该 TCP 端口无监听 |
| `docker:<container\|network\|volume>:<name>` | `docker inspect` 不存在 |
| `kind:<name>` | `kind get clusters` 不含该项 |
| `dir:<path>` | 路径不存在 |

其他前缀（`kube:ns/prod`、`remote:jenkins`）允许写，参与冲突检测，但**框架不复查**，`stop` 时会提示「以下 claim 框架未复查，由插件保证」。这类资源的清理完全靠你的 `down`。

### 7.2 stop 后的实测

`down` 退出 0 之后，框架会对四类已知前缀逐个实测：

- 全部释放 → 实例 `stopped`，可以再起
- 有残留 → 实例标 `dirty`，列出没释放的东西，同插件再 `start` 被拦

`dirty` 是诚实的失败。不要为了让状态好看而在 `down` 里说谎。

---

## 8. 最小骨架

### 8.1 bash

```bash
#!/usr/bin/env bash
set -uo pipefail   # 注意：不要 set -e

cmd="${1:-}"; shift || true
instance=""; args_file=""
for a in "$@"; do
  case "$a" in
    --instance=*) instance="${a#--instance=}" ;;
    --args=@*)    args_file="${a#--args=@}" ;;
  esac
done

state_dir="${CHAOSLAB_HOME:-$HOME/.lab}/myplugin-state"
work="$state_dir/$instance"

case "$cmd" in
  schema)
    echo '{"name":"myplugin","version":"0.1.0","args":[]}'
    ;;
  list)
    cat <<JSON
{"scenarios":[{"id":"demo","title":"示例故障"}],
 "claims":["dir:$work"],
 "symptom":"访问站点返回 500。",
 "connect":["日志目录：$work"]}
JSON
    ;;
  up)
    mkdir -p "$work" || exit 1
    # 在这里注入全部故障
    echo "环境已就绪" >&2
    cat <<JSON
{"symptom":"访问站点返回 500。","connect":["日志目录：$work"]}
JSON
    ;;
  down)
    rm -rf "$work"      # 幂等：不存在也不报错
    ;;
  check)
    # 修复的判据：比如某个文件出现
    if [ -f "$work/fixed" ]; then
      echo '{"passed":true,"cases":[{"id":"demo","passed":true}],"root_cause":"示例根因","clues":["先看日志"]}'
      exit 0
    fi
    echo '{"passed":false,"cases":[{"id":"demo","passed":false}]}'
    exit 1
    ;;
  *) exit 2 ;;
esac
```

### 8.2 Python

```python
#!/usr/bin/env python3
import json, os, sys, shutil

argv = sys.argv[1:]
cmd = argv[0] if argv else ""
instance = args_file = ""
for a in argv[1:]:
    if a.startswith("--instance="): instance = a.split("=", 1)[1]
    elif a.startswith("--args=@"):  args_file = a.split("=", 1)[1]

args = {}
if args_file and os.path.exists(args_file):
    args = json.load(open(args_file))

root = os.path.join(os.environ.get("CHAOSLAB_HOME", os.path.expanduser("~/.lab")), "myplugin-state")
work = os.path.join(root, instance or "x")

def out(obj): print(json.dumps(obj))
def log(msg): print(msg, file=sys.stderr)

if cmd == "schema":
    out({"name": "myplugin", "version": "0.1.0", "args": []})
elif cmd == "list":
    out({
        "scenarios": [{"id": "demo", "title": "示例故障"}],
        "claims": [f"dir:{work}"],
        "symptom": "访问站点返回 500。",
        "connect": [f"日志目录：{work}"],
    })
elif cmd == "up":
    os.makedirs(work, exist_ok=True)
    log("环境已就绪")
    out({"symptom": "访问站点返回 500。", "connect": [f"日志目录：{work}"]})
elif cmd == "down":
    shutil.rmtree(work, ignore_errors=True)   # 幂等
elif cmd == "check":
    if os.path.exists(f"{work}/fixed"):
        out({"passed": True, "cases": [{"id": "demo", "passed": True}],
             "root_cause": "示例根因", "clues": ["先看日志"]})
    else:
        out({"passed": False, "cases": [{"id": "demo", "passed": False}]})
        sys.exit(1)
else:
    sys.exit(2)
```

---

## 9. 三个反例

### 9.1 `down` 不幂等

```bash
down)
  rm -rf /mnt/myenv          # 不存在时 rm 退出 0，OK
  docker rm mycontainer      # 容器不存在时退出 1 -> 框架判 dirty
```

后果：用户第二次 `stop` 永远失败，实例永久卡在 `dirty`。
正确做法：`docker rm -f mycontainer || true`，或者先 `docker inspect` 判断。

### 9.2 `check` 用了 `set -e`

```bash
#!/usr/bin/env bash
set -euo pipefail
up_or_down_check() { kubectl get pod ready-pod; }   # 返回 1
up_or_down_check    # 脚本立刻退出 1... 但等等，1 是"没修好"，这里碰巧对了
```

更糟的情形：

```bash
set -euo pipefail
grep -q "ready" /tmp/status        # 没匹配 -> 退出 1
echo "全部就绪"                     # 永远执行不到
```

`set -e` 让你的脚本在第一个非零命令处静默死掉。此时退出码可能是 1（被当成"没修好"，错误地继续轮询）或 2 以外的别的值（被当成"检查出错"），永远到不了"已修好"。**`check` 的脚本不要用 `set -e`**，用 `set -uo pipefail`。

### 9.3 claims 写死了资源名

```bash
list)
  echo '{"scenarios":[...],"claims":["port:3306"]}'
```

端口写死成常量，两个插件实例就永远冲突。更糟的是 claim 和实际资源名不一致（写 `dir:/tmp/a` 而实际用了 `/var/tmp/b`）：冲突检测形同虚设，两个实例互相踩。

**claims 必须和真实资源一一对应，并且跟着 `--instance` 和 `--args` 变化。**

---

## 10. 写完之后

```bash
chaoslab plugin validate ./myplugin    # 静态检查，不执行 up
chaoslab plugin install ./myplugin
chaoslab list
chaoslab start myplugin/demo
```

`validate` 会检查协议结构、场景 id、claim 前缀。它不跑 `up`——真实校验只能靠 `start`。

本仓库里有两个可参考的完整实现：

- `plugins/fake/`：零依赖 bash，演示协议的每个细节（含故意失败、故意泄漏的参数），也是框架的测试替身
- `plugins/k8s/`：五个真实 K8s 故障，全部注入同一个临时 kind 集群；演示共享环境、镜像预热、幂等兜底清理、复用外部集群
