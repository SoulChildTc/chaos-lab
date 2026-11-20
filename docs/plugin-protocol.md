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

## 2. 一个插件装多个场景

```
myplugin/
  plugin
  cases/
    slow-query/
    wrong-port/
```

`chaoslab list` 看到的是 `myplugin/slow-query`。用户通过 `插件/场景` 引用它。

一个实例 = 一次运行，ID 是 `myplugin/slow-query#1`。同一个场景可以跑多个实例，但同一场景的第二个实例会被拒绝，除非先停掉第一个。

---

## 3. 五个子命令

框架以子进程方式调用 `plugin`，每次调用都是一个新进程，调用之间没有任何常驻状态。所有状态必须放在磁盘或真实环境里。

```
plugin schema --json
plugin list   --json [--instance=<slug>] [--args=@<file>]
plugin up     <case> --instance=<slug> --args=@<file>
plugin down   <case> --instance=<slug> --args=@<file>
plugin check  <case> --instance=<slug> --args=@<file>
```

### 3.1 执行环境

| 项 | 值 |
|---|---|
| 工作目录 | 插件目录（可用相对路径引用自身文件） |
| stdin | `/dev/null` |
| stdout | **只放协议数据**（JSON 或 check 的报告） |
| stderr | **只放给人看的进度**，框架实时透传 |
| `LAB_INSTANCE` | 实例 slug，例如 `myplugin-slow-query-1` |
| `LAB_PLUGIN_DIR` | 插件目录绝对路径 |
| `LAB_CASE` | 场景 id |

超时到达时框架对**整个进程组** `SIGTERM`，5 秒后 `SIGKILL`。你自己派生的子进程也会一起终止——但也正因如此，**不要指望你的子进程在超时后还能活着完成任务**。

### 3.2 slug 可以直接当资源名用

slug 形如 `myplugin-slow-query-1`，只含小写字母、数字、连字符。用它拼容器名、集群名、namespace、目录名是安全的，并且天然不冲突。

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
- 参数表会直接变成 `chaoslab start <你的插件>/<场景> --help` 的输出，写清楚

**插件收不到原始命令行。** 框架解析、校验、补默认值，然后把结果放进 `--args` 指向的 JSON 文件。

---

## 5. `list --json`

```json
{
  "cases": [
    {
      "id": "slow-query",
      "title": "查询一直不返回",
      "claims": ["docker:container:myplugin-slow-query-1", "port:3306"],
      "up_timeout": "5m",
      "check_timeout": "10m"
    }
  ]
}
```

- `id` 匹配 `^[a-z0-9][a-z0-9-]*$`，不可重复
- `title` 是一句话现象，用户在 `list` 里看到的就是它
- `claims` 见第 7 节
- `up_timeout` / `check_timeout` 可选，默认 `5m` / `10m`

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

创建环境并埋好故障。成功退出 0，失败非 0（框架记 3）。

- 通过 stderr 输出进度：`环境已就绪`、现象描述。**不要给答案。**
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
  "root_cause": "Service 的 targetPort 写成了 8080，容器实际监听 80",
  "clues": ["Endpoints 有地址，容易误判为正常", "curl 返回 connection refused"]
}
```

`root_cause` 和 `clues` 会原样存进 run 记录，是用户修好之后看到的复盘。

**`check` 只观察，不修复。** 修复是用户的事，这是本工具存在的意义。

---

## 7. claims：声明你占了什么

claims 是字符串数组，框架用它做两件事：拒绝冲突的并行实例，验证 stop 后真的清理干净了。

### 7.1 冲突检测

两个存活实例有任何相同的 claim，第二个 `start` 被拒：

```
错误：k8s/pod-pending#1 已经占用 port:3306
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
- 有残留 → 实例标 `dirty`，列出没释放的东西，同场景再 `start` 被拦

`dirty` 是诚实的失败。不要为了让状态好看而在 `down` 里说谎。

---

## 8. 最小骨架

### 8.1 bash

```bash
#!/usr/bin/env bash
set -uo pipefail   # 注意：不要 set -e

cmd="${1:-}"; shift || true
case_id=""; instance=""; args_file=""
for a in "$@"; do
  case "$a" in
    --instance=*) instance="${a#--instance=}" ;;
    --args=@*)    args_file="${a#--args=@}" ;;
    --*)          ;;
    *)            [ -z "$case_id" ] && case_id="$a" ;;
  esac
done

json_str() {
  [ -n "$args_file" ] && [ -f "$args_file" ] || return 0
  sed -n "s/.*\"$1\"[[:space:]]*:[[:space:]]*\"\([^\"]*\)\".*/\1/p" "$args_file" | head -1
}

state_dir="${CHAOSLAB_HOME:-$HOME/.lab}/myplugin-state"

case "$cmd" in
  schema)
    echo '{"name":"myplugin","version":"0.1.0","args":[]}'
    ;;
  list)
    printf '{"cases":[{"id":"demo","title":"示例场景","claims":["dir:%s/%s"]}]}\n' "$state_dir" "${instance:-x}"
    ;;
  up)
    mkdir -p "$state_dir/$instance" || exit 1
    echo "环境已就绪" >&2
    echo "现象：访问返回 500" >&2
    ;;
  down)
    rm -rf "$state_dir/$instance"      # 幂等：不存在也不报错
    ;;
  check)
    # 修复的判据：比如某个文件出现
    if [ -f "$state_dir/$instance/fixed" ]; then
      echo '{"passed":true,"root_cause":"示例根因","clues":["先看日志"]}'
      exit 0
    fi
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
case_id = instance = args_file = ""
for a in argv[1:]:
    if a.startswith("--instance="): instance = a.split("=", 1)[1]
    elif a.startswith("--args=@"):  args_file = a.split("=", 1)[1]
    elif not a.startswith("--") and not case_id: case_id = a

args = {}
if args_file and os.path.exists(args_file):
    args = json.load(open(args_file))

state = os.path.join(os.environ.get("CHAOSLAB_HOME", os.path.expanduser("~/.lab")), "myplugin-state")

def out(obj): print(json.dumps(obj))
def log(msg): print(msg, file=sys.stderr)

if cmd == "schema":
    out({"name": "myplugin", "version": "0.1.0", "args": []})
elif cmd == "list":
    out({"cases": [{"id": "demo", "title": "示例场景",
                    "claims": [f"dir:{state}/{instance or 'x'}"]}]})
elif cmd == "up":
    os.makedirs(f"{state}/{instance}", exist_ok=True)
    log("环境已就绪"); log("现象：访问返回 500")
elif cmd == "down":
    shutil.rmtree(f"{state}/{instance}", ignore_errors=True)   # 幂等
elif cmd == "check":
    if os.path.exists(f"{state}/{instance}/fixed"):
        out({"passed": True, "root_cause": "示例根因", "clues": ["先看日志"]})
    else:
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
  echo '{"cases":[{"id":"c","title":"t","claims":["port:3306"]}]}'
```

两个实例都被声明占用 `port:3306`——冲突检测永远拒绝第二个实例，而这本来可能是正确的（如果插件给每个实例分配不同端口）。反过来更糟：如果你写的 claim 和实际使用的资源名不一致（比如写 `dir:/tmp/a` 而实际用了 `/var/tmp/b`），冲突检测形同虚设，两个实例互相踩。

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
- `plugins/k8s/`：五个真实 K8s 场景，每个实例一个临时 kind 集群；演示镜像预热、幂等兜底清理、复用外部集群
