# fake 插件

零依赖的测试替身。不创建任何真实环境，只用目录和端口模拟一个场景，用来验证框架行为，也是写插件的最小参考实现。

它是产品的一部分：框架的冲突检测、`dirty` 判定、`--json` 输出这些逻辑，靠它在没有 Docker 的机器上也能自动化测试。

```bash
chaoslab plugin install ./plugins/fake
chaoslab list
```

## 场景

两个，`fake/env` 和 `fake/other`，行为完全相同。都是“环境已就绪、check 直接通过”——它本来就没有故障。

状态放在 `~/.lab/fake-state/<slug>/`。

## 参数

这些参数是这个插件的重点：每个都用来触发框架的一条错误路径。

| 参数 | 默认 | 作用 |
|---|---|---|
| `--port` | `0` | 非 0 时声明 `port:<n>` claim，用来测冲突检测 |
| `--fail-up` | `false` | `up` 失败 → 测「启动失败但 live=true」 |
| `--fail-down` | `false` | `down` 失败 → 测 `dirty` |
| `--leak-dir` | `false` | `down` 返回成功但留下目录 → 测「残留被实测抓出」 |
| `--broken` | `false` | `check` 退出 1 → 测未修好的判定 |
| `--check-timeout` | 无 | 覆盖 `check_timeout`，让 `--watch` 秒级超时 |

## 用法示例

**冲突检测**：

```bash
chaoslab start fake/env --port=18080
chaoslab start fake/other --port=18080
# 错误：fake/env#1 已经占用 port:18080
```

**dirty 状态**（down 说成功但目录还在）：

```bash
chaoslab start fake/env --leak-dir=true
chaoslab stop fake/env
# fake/env#1 未清理干净：down 报告成功，但仍有资源未释放：dir:... （path still exists）
```

**dirty 状态**（down 直接失败）：

```bash
chaoslab start fake/env --fail-down=true
chaoslab stop fake/env
```

**watch 超时**：

```bash
chaoslab start fake/env --broken=true --check-timeout=3s
chaoslab check fake/env --watch     # 3 秒后返回 2，写 timeout 记录
```

## 自测

```bash
bash plugins/fake/plugin_test.sh
```

直接调用 `plugin`，不经过框架，验证协议本身：五个子命令的退出码、claims 是否随实例变化、`down` 是否幂等。

## 结构

```
plugins/fake/
  plugin            全部逻辑，约 120 行 bash
  plugin_test.sh    协议自测
```

没有 `cases/` 目录：场景清单直接写在 `plugin` 的 `list` 分支里。框架不关心插件内部怎么组织。
