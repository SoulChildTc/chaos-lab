# fake 插件

零依赖的测试替身。不创建任何真实环境，只用目录和端口模拟一个插件，用来验证框架行为，也是写插件的最小参考实现。

它是产品的一部分：框架的冲突检测、`dirty` 判定、`--json` 输出这些逻辑，靠它在没有 Docker 的机器上也能自动化测试。

```bash
chaoslab plugin install ./plugins/fake
chaoslab list
```

## 故障

`list` 里声明两个故障（`env`、`other`），都只是目录条目——插件启动时会把它们全部"注入"，`check` 直接通过，因为它本来就没有真故障。`broken=true` 时才判定为未修好。

状态放在 `~/.lab/fake-state/<实例 slug>/`。

## 参数

这些参数是这个插件的重点：每个都用来触发框架的一条错误路径。

| 参数 | 默认 | 作用 |
|---|---|---|
| `--port` | `0` | 非 0 时声明 `port:<n>` claim，用来测冲突检测 |
| `--fail-up` | `false` | `up` 失败 → 测「启动失败但 live=true」 |
| `--fail-down` | `false` | `down` 失败 → 测 `dirty` |
| `--leak-dir` | `false` | `down` 返回成功但留下目录 → 测「残留被实测抓出」 |
| `--broken` | `false` | `check` 退出 1，报 `passed:false` → 测未修好的判定 |
| `--check-timeout` | 无 | 覆盖 `check_timeout`，让 `--watch` 秒级超时 |

## 用法示例

**冲突检测**（需要第二个插件，因为同一插件不能起两个实例）：

```bash
chaoslab start fake --port=18080
chaoslab plugin install ./plugins/k8s   # 或任何别的插件
# 把它改成声明同一个端口后 start，会被拒：
# invalid input: fake#1 already holds port:18080
```

**dirty 状态**（down 说成功但目录还在）：

```bash
chaoslab start fake --leak-dir=true
chaoslab stop fake
# ✗ not clean fake#1
#   down reported success, but these are still held: dir:... (path still exists)
```

**dirty 状态**（down 直接失败）：

```bash
chaoslab start fake --fail-down=true
chaoslab stop fake
```

**watch 超时**：

```bash
chaoslab start fake --broken=true --check-timeout=3s
chaoslab check fake --watch     # 3 秒后返回 2，写 timeout 记录
```

## 自测

```bash
bash plugins/fake/plugin_test.sh
```

直接调用 `plugin`，不经过框架，验证协议本身：五个子命令的退出码、claims 是否随实例变化、`up` 是否输出 symptom/connect、`down` 是否幂等。

## 结构

```
plugins/fake/
  plugin            全部逻辑，约 120 行 bash
  plugin_test.sh    协议自测
```

没有 `faults/` 目录：故障清单直接写在 `plugin` 的 `list` 分支里。框架不关心插件内部怎么组织。
