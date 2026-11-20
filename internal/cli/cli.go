package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

// usage is the text printed when the command line is not actionable.
const usage = `chaoslab — 本地故障演练实验室

用法：
  chaoslab list [--json]
  chaoslab start <plugin>/<case> [--arg=value ...] [--json] [--help]
  chaoslab check (<plugin>/<case> | <instance-id>) [--watch] [--json]
  chaoslab stop|rm (<plugin>/<case> | <instance-id>) [--json]
  chaoslab stop|rm --all
  chaoslab status [<instance-id>] [--json]
  chaoslab plugin install <git-url|dir>
  chaoslab plugin uninstall <name>
  chaoslab plugin info <name>
  chaoslab plugin validate <dir>
  chaoslab version [--json]

退出码：0 成功/通过  1 用法或冲突  2 未通过  3 插件执行出错
`

// Run parses argv and dispatches one subcommand. The returned code is the
// process exit status; err is the message to print on stderr.
func Run(ctx context.Context, rt *Runtime, argv []string) (int, error) {
	args, asJSON := stripJSONFlag(argv)
	if len(args) == 0 {
		fmt.Print(usage)
		return ExitOK, nil
	}

	switch args[0] {
	case "help", "-h", "--help":
		fmt.Print(usage)
		return ExitOK, nil

	case "version", "-v", "--version":
		return RunVersion(asJSON)

	case "list":
		return RunList(ctx, rt, asJSON)

	case "status":
		if len(args) > 2 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab status [<instance-id>]", plugin.ErrUserInput)
		}
		ref := ""
		if len(args) == 2 {
			ref = args[1]
		}
		return RunStatus(ctx, rt, ref, asJSON)

	case "start":
		return runStartArgs(ctx, rt, args[1:], asJSON)

	case "check":
		if len(args) < 2 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab check <plugin>/<case> [--watch]", plugin.ErrUserInput)
		}
		watch := false
		var positional []string
		for _, a := range args[1:] {
			if a == "--watch" {
				watch = true
				continue
			}
			if strings.HasPrefix(a, "-") {
				return ExitUserError, fmt.Errorf("%w: 未知选项 %q", plugin.ErrUserInput, a)
			}
			positional = append(positional, a)
		}
		if len(positional) != 1 {
			return ExitUserError, fmt.Errorf("%w: check 需要且只能有一个实例引用", plugin.ErrUserInput)
		}
		return RunCheck(ctx, rt, positional[0], watch, asJSON)

	case "stop", "rm":
		all := false
		var positional []string
		for _, a := range args[1:] {
			if a == "--all" {
				all = true
				continue
			}
			if strings.HasPrefix(a, "-") {
				return ExitUserError, fmt.Errorf("%w: 未知选项 %q", plugin.ErrUserInput, a)
			}
			positional = append(positional, a)
		}
		if all {
			if len(positional) > 0 {
				return ExitUserError, fmt.Errorf("%w: --all 不能和实例引用一起用", plugin.ErrUserInput)
			}
			return RunStopAll(ctx, rt)
		}
		if len(positional) != 1 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab stop <plugin>/<case> | --all", plugin.ErrUserInput)
		}
		return RunStop(ctx, rt, positional[0], asJSON)

	case "plugin":
		if len(args) < 2 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab plugin <install|uninstall|info|validate>", plugin.ErrUserInput)
		}
		return RunPlugin(ctx, rt, args[1], args[2:], asJSON)

	default:
		return ExitUserError, fmt.Errorf("%w: 未知命令 %q\n\n%s", plugin.ErrUserInput, args[0], usage)
	}
}

// stripJSONFlag removes --json wherever it appears. Only one global flag exists,
// so a scan is simpler than a real flag parser.
func stripJSONFlag(argv []string) ([]string, bool) {
	out := make([]string, 0, len(argv))
	found := false
	for _, a := range argv {
		if a == "--json" {
			found = true
			continue
		}
		out = append(out, a)
	}
	return out, found
}

// runStartArgs parses `start <ref> [--arg=value ...] [--help]`.
func runStartArgs(ctx context.Context, rt *Runtime, args []string, asJSON bool) (int, error) {
	if len(args) == 0 {
		return ExitUserError, fmt.Errorf("%w: 用法 chaoslab start <plugin>/<case> [--arg=value ...]", plugin.ErrUserInput)
	}
	ref := args[0]
	if strings.HasPrefix(ref, "-") {
		return ExitUserError, fmt.Errorf("%w: start 需要 <plugin>/<case>", plugin.ErrUserInput)
	}

	kv := map[string]string{}
	wantHelp := false
	for _, a := range args[1:] {
		if a == "--help" || a == "-h" {
			wantHelp = true
			continue
		}
		if !strings.HasPrefix(a, "--") {
			return ExitUserError, fmt.Errorf("%w: 多余的参数 %q", plugin.ErrUserInput, a)
		}
		body := strings.TrimPrefix(a, "--")
		k, v, ok := strings.Cut(body, "=")
		if !ok {
			return ExitUserError, fmt.Errorf("%w: 参数 %q 缺少 = 值", plugin.ErrUserInput, a)
		}
		kv[k] = v
	}

	if wantHelp {
		return printStartHelp(ctx, rt, ref)
	}
	return RunStart(ctx, rt, ref, kv, asJSON)
}

func printStartHelp(ctx context.Context, rt *Runtime, ref string) (int, error) {
	pluginName, caseName, err := splitRef(ref)
	if err != nil {
		return ExitUserError, err
	}
	lister := rt.Lister(pluginName)
	if _, err := os.Stat(lister.PluginDir + "/plugin"); err != nil {
		return ExitUserError, fmt.Errorf("%w: 插件 %s 未安装", plugin.ErrUserInput, pluginName)
	}
	schema, err := lister.Schema(ctx)
	if err != nil {
		return ExitPluginErr, err
	}
	cases, err := lister.List(ctx, "", nil)
	if err != nil {
		return ExitPluginErr, err
	}
	c, ok := findCase(cases, caseName)
	if !ok {
		return ExitUserError, fmt.Errorf("%w: 插件 %s 没有场景 %q", plugin.ErrUserInput, pluginName, caseName)
	}

	fmt.Printf("%s/%s — %s\n\n", pluginName, c.ID, c.Title)
	if len(schema.Args) == 0 {
		fmt.Println("该场景没有可配置参数。")
		return ExitOK, nil
	}
	fmt.Println("参数：")
	for _, a := range schema.Args {
		fmt.Println(a.HelpLine())
	}
	return ExitOK, nil
}
