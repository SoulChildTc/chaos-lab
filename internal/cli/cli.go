package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func newRoot(rt *Runtime) *cobra.Command {
	root := &cobra.Command{
		Use:   "chaoslab",
		Short: "Local fault-drill lab",
		Long: `Local fault-drill lab. Bring up a broken environment, get the symptom only,
and track down the fault yourself. A plugin decides whether you fixed it.
Scenarios come from plugins, so install one before anything is runnable.`,
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
	root.PersistentFlags().Bool("json", false, "machine-readable output")
	// -v is what people type first.
	root.Flags().BoolP("version", "v", false, "print version information")

	root.CompletionOptions.DisableDefaultCmd = true
	installHelp(root)
	root.SetHelpCommand(&cobra.Command{
		Use:   "help [command]",
		Short: "Help about any command",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := root.Find(args)
			if err != nil || target == nil {
				return fail(ExitUserError, "unknown command: %v", args)
			}
			target.Help()
			return nil
		},
	})

	root.AddCommand(
		newListCmd(rt),
		newStartCmd(rt),
		newCheckCmd(rt),
		newStopCmd(rt, "stop", "Tear the environment down and verify nothing leaked"),
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
		Use:   "list [plugin]",
		Short: "List scenarios",
		Long:  "List scenarios from every installed plugin. Give a plugin name to list only that plugin's.",
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
		Use:   "check <plugin>/<case>[#n]",
		Short: "Decide whether the fault is fixed",
		Long: `Decide whether the scenario has been fixed. Exit code 0 means fixed, 2 means
not yet, 3 means the check itself failed.

A single check returns immediately; --watch retries every 3 seconds until it
passes or the scenario's check_timeout is exceeded.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return failWith(RunCheck(cmd.Context(), rt, args[0], watch, jsonFlag(cmd)))
		},
	}
	cmd.Flags().BoolVar(&watch, "watch", false, "poll until it passes or times out")
	return cmd
}

func newStopCmd(rt *Runtime, use, short string) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:   use + " (<plugin>/<case>[#n] | --all)",
		Short: short,
		Long: `Have the plugin destroy the environment, then measure for yourself whether the
resources it claimed are really gone. Anything left behind is marked dirty and
blocks restarting that scenario.`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if all {
				if len(args) > 0 {
					return fail(ExitUserError, "--all cannot be combined with an instance reference")
				}
				return failWith(RunStopAll(cmd.Context(), rt))
			}
			if len(args) != 1 {
				return fail(ExitUserError, "specify an instance, or use --all: chaoslab %s <plugin>/<case>", use)
			}
			return failWith(RunStop(cmd.Context(), rt, args[0], jsonFlag(cmd)))
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "stop every live instance")
	return cmd
}

// newStopAliasCmd applies --all the same way stop does, but stays out of the
// help listing: an alias is a convenience, not a second concept to learn.
func newStopAliasCmd(rt *Runtime) *cobra.Command {
	cmd := newStopCmd(rt, "rm", "Alias for stop")
	cmd.Hidden = true
	return cmd
}

func newStatusCmd(rt *Runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "status [<plugin>/<case>[#n]]",
		Short: "Show live instances",
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
		Short: "Print version and build information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return failWith(RunVersion(jsonFlag(cmd)))
		},
	}
}

func newPluginCmd(rt *Runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugin",
		Short: "Manage plugins",
		RunE: func(cmd *cobra.Command, args []string) error {
			return fail(ExitUserError, "a subcommand is required: install / uninstall / info / validate")
		},
	}
	cmd.AddCommand(
		&cobra.Command{
			Use:   "install <dir|git-url>",
			Short: "Install a plugin",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "install", args, jsonFlag(cmd)))
			},
		},
		&cobra.Command{
			Use:   "uninstall <name>",
			Short: "Remove a plugin (refused while it has live instances)",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "uninstall", args, false))
			},
		},
		&cobra.Command{
			Use:   "info <name>",
			Short: "Show a plugin's arguments and scenarios",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				return failWith(RunPlugin(cmd.Context(), rt, "info", args, jsonFlag(cmd)))
			},
		},
		&cobra.Command{
			Use:   "validate <dir>",
			Short: "Self-check a plugin without running up",
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
		Use:   "start <plugin>/<case> [--arg=value ...]",
		Short: "Bring an environment up and show only the symptom",
		Long: `Bring up one scenario and inject its fault. The output stops at the symptom and
does not tell you the cause.

Each scenario declares its own flags; add the scenario name to see them:
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
				return ExitUserError, fmt.Errorf("flag %q needs a value, write --%s=<value>", a, k)
			}
			kv[k] = v
		case strings.HasPrefix(a, "-"):
			return ExitUserError, fmt.Errorf("unknown flag %q", a)
		default:
			if ref != "" {
				return ExitUserError, fmt.Errorf("unexpected argument %q", a)
			}
			ref = a
		}
	}

	if ref == "" {
		if wantHelp {
			cmd.Help()
			return ExitOK, nil
		}
		return ExitUserError, fmt.Errorf("a scenario is required: chaoslab start <plugin>/<case>")
	}
	if wantHelp {
		return printStartHelp(cmd.Context(), rt, ref)
	}
	return RunStart(cmd.Context(), rt, ref, kv, asJSON)
}
