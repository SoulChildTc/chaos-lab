package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// exitError carries a code out of a cobra RunE. Cobra has no notion of exit
// codes, but every chaoslab command has one and scripts depend on them.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}

func (e *exitError) Unwrap() error { return e.err }

// fail returns an error that makes the process exit with code.
func fail(code int, format string, args ...any) error {
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

// failWith keeps an existing error so errors.Is still works, and attaches a code.
func failWith(code int, err error) error {
	if err == nil {
		err = errors.New("")
	}
	return &exitError{code: code, err: err}
}

// Run builds the command tree, executes it, and returns the process exit code
// plus the message to print on stderr.
func Run(ctx context.Context, rt *Runtime, argv []string) (int, error) {
	root := newRoot(rt)
	root.SetArgs(argv)
	err := root.ExecuteContext(ctx)

	var x *exitError
	if errors.As(err, &x) {
		if x.err == nil || x.err.Error() == "" {
			return x.code, nil
		}
		return x.code, x.err
	}
	if err != nil {
		return ExitUserError, err
	}
	return ExitOK, nil
}

func newRoot(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "chaoslab",
		Short: "本地故障演练环境",
		Long: `本地故障演练环境。起好带故障的环境，只告诉你现象；你排查修复，
改对了由插件定义的规则判定。场景来自插件，装一个才有东西可跑。`,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			if v, _ := cmd.Flags().GetBool("version"); v {
				return failWith(RunVersion(jsonFlag(cmd)))
			}
			// Bare invocation is a mistake, not a success: a script that forgot
			// its subcommand must not read as "went fine".
			cmd.Help()
			return fail(ExitUserError, "")
		},
	}
	root.PersistentFlags().Bool("json", false, "机器可读输出")
	root.PersistentFlags().Lookup("json").Usage = "机器可读输出"
	// -v is what people type first.
	root.Flags().BoolP("version", "v", false, "版本与构建信息")

	root.CompletionOptions.DisableDefaultCmd = true
	installHelp(root)
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [命令]",
		Short: "查看命令帮助",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := root.Find(args)
			if err != nil || target == nil {
				return fail(ExitUserError, "没有这个命令：%v", args)
			}
			target.Help()
			return nil
		},
	})

	root.AddCommand(
		newListCmd(rt),
		newStartCmd(rt),
		newCheckCmd(rt),
		newStopCmd(rt, "stop", "清理环境并实测残留"),
		newStopAliasCmd(rt),
		newStatusCmd(rt),
		newPluginCmd(rt),
		newVersionCmd(),
	)
	return root
}

func jsonFlag(cmd *cobra.Command) bool {
	v, _ := cmd.Flags().GetBool("json")
	return v
}

func newListCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list [插件]",
		Short: "列出场景",
		Long:  "列出所有已装插件的场景。给出插件名则只列该插件的。",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			filter := ""
			if len(args) == 1 {
				filter = args[0]
			}
			return failWith(RunList(cmd.Context(), rt, filter, jsonFlag(cmd)))
		},
	}
}

func newCheckCmd(rt *Runtime) *cobra.Command {
	var watch bool
	cmd := &cobra.Command{
		Use:   "check <插件>/<场景>[#序号]",
		Short: "判定是否修好",
		Long: `判定场景是否已修复。退出码 0 修好，2 还没修好，3 检查本身出错。

单次判定直接返回；--watch 每 3 秒重试，直到通过或超过该场景的 check_timeout。`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return failWith(RunCheck(cmd.Context(), rt, args[0], watch, jsonFlag(cmd)))
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "轮询到通过或超时")
	return cmd
}

func newStopCmd(rt *Runtime, use, short string) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   use + " (<插件>/<场景>[#序号] | --all)",
		Short: short,
		Long:  "让插件销毁环境，然后框架自己实测插件声明的资源是否真的释放了。没清干净会标为 dirty 并拦住重试。",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				if len(args) > 0 {
					return fail(ExitUserError, "--all 不能和实例引用一起用")
				}
				return failWith(RunStopAll(cmd.Context(), rt))
			}
			if len(args) != 1 {
				return fail(ExitUserError, "需要指定实例，或用 --all：chaoslab %s <插件>/<场景>", use)
			}
			return failWith(RunStop(cmd.Context(), rt, args[0], jsonFlag(cmd)))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "停掉所有存活实例")
	return cmd
}

// newStopAlias applies --all the same way stop does, but stays out of the help
// listing: an alias is a convenience, not a second concept to learn.
func newStopAliasCmd(rt *Runtime) *cobra.Command {
	cmd := newStopCmd(rt, "rm", "清理环境并实测残留")
	cmd.Hidden = true
	return cmd
}

func newStatusCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status [<插件>/<场景>[#序号]]",
		Short: "查看存活实例",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			ref := ""
			if len(args) == 1 {
				ref = args[0]
			}
			return failWith(RunStatus(cmd.Context(), rt, ref, jsonFlag(cmd)))
		},
	}
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "版本与构建信息",
		RunE: func(cmd *cobra.Command, args []string) error {
			return failWith(RunVersion(jsonFlag(cmd)))
		},
	}
}

func newPluginCmd(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "管理插件",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fail(ExitUserError, "需要子命令：install / uninstall / info / validate")
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "install <目录|git-url>",
			Short: "装插件",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "install", args, jsonFlag(cmd)))
			},
		},
		&cobra.Command{
			Use:   "uninstall <名字>",
			Short: "卸插件，有存活实例时拒绝",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "uninstall", args, false))
			},
		},
		&cobra.Command{
			Use:   "info <名字>",
			Short: "看插件支持的参数和场景",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "info", args, jsonFlag(cmd)))
			},
		},
		&cobra.Command{
			Use:   "validate <目录>",
			Short: "写插件时自检，不执行 up",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "validate", args, false))
			},
		},
	)
	return cmd
}

// newStartCmd disables cobra's flag parsing on purpose: the flags a scenario
// accepts are declared by the plugin at runtime, so --node-image etc. cannot be
// registered ahead of time. The subcommand parses its own argv, and --help is
// answered with the plugin's real parameter table.
func newStartCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "start <插件>/<场景> [--参数=值 ...]",
		Short: "起环境，只给现象",
		Long: `起一个场景的环境并注入故障，输出停在现象那一行，不告诉你原因。

每个场景支持的参数由插件声明，加上场景名再 --help 查看：
  chaoslab start k8s/pod-pending --help`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return failWith(runStartArgs(cmd, rt, args))
		},
	}
}

// runStartArgs parses `start <ref> [--arg=value ...] [--json] [--help]`.
func runStartArgs(cmd *cobra.Command, rt *Runtime, args []string) (int, error) {
	var ref string
	kv := map[string]string{}
	asJSON := false
	wantHelp := false

	for _, a := range args {
		switch {
		case a == "--help" || a == "-h":
			wantHelp = true
		case a == "--json":
			asJSON = true
		case strings.HasPrefix(a, "--"):
			k, v, ok := strings.Cut(strings.TrimPrefix(a, "--"), "=")
			if !ok {
				return ExitUserError, fmt.Errorf("参数 %q 缺少值，写成 --%s=<值>", a, k)
			}
			kv[k] = v
		case strings.HasPrefix(a, "-"):
			return ExitUserError, fmt.Errorf("未知选项 %q", a)
		default:
			if ref != "" {
				return ExitUserError, fmt.Errorf("多余的参数 %q", a)
			}
			ref = a
		}
	}

	if ref == "" {
		if wantHelp {
			// `start --help` with no scenario: show the command's own help
			// rather than complaining about a missing argument.
			cmd.Help()
			return ExitOK, nil
		}
		return ExitUserError, errors.New("需要指定场景：chaoslab start <插件>/<场景>")
	}
	if wantHelp {
		return printStartHelp(cmd.Context(), rt, ref)
	}
	return RunStart(cmd.Context(), rt, ref, kv, asJSON)
}
