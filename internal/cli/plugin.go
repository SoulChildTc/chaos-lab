package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

// RunPlugin dispatches the `plugin` subcommands. sub is one of
// install|uninstall|info|validate.
func RunPlugin(ctx context.Context, rt *Runtime, sub string, rest []string, asJSON bool) (int, error) {
	switch sub {
	case "install":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: usage: chaoslab plugin install <git-url|dir>", plugin.ErrUserInput)
		}
		name, err := plugin.Install(ctx, rest[0], rt.PluginsDir)
		if err != nil {
			return ExitUserError, err
		}
		fmt.Printf("%s %s %s\n", glyphOK(), ok("installed"), bold(name))
		fmt.Println(dim("  next: chaoslab list"))
		return ExitOK, nil

	case "uninstall":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: usage: chaoslab plugin uninstall <name>", plugin.ErrUserInput)
		}
		f, err := rt.Store.Read(ctx)
		if err != nil {
			return ExitUserError, err
		}
		if err := plugin.Uninstall(rt.PluginsDir, rest[0], f.Instances); err != nil {
			return ExitUserError, err
		}
		fmt.Printf("%s %s %s\n", glyphOK(), ok("removed"), bold(rest[0]))
		return ExitOK, nil

	case "info":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: usage: chaoslab plugin info <name>", plugin.ErrUserInput)
		}
		lister := rt.Lister(rest[0])
		if _, err := os.Stat(lister.PluginDir + "/plugin"); err != nil {
			return ExitUserError, fmt.Errorf("%w: plugin %s is not installed", plugin.ErrUserInput, rest[0])
		}
		schema, err := lister.Schema(ctx)
		if err != nil {
			return ExitPluginErr, err
		}
		if asJSON {
			return ExitOK, printJSON(schema)
		}
		fmt.Printf("%s %s\n", schema.Name, schema.Version)
		if schema.Description != "" {
			fmt.Println(schema.Description)
		}
		if len(schema.Args) > 0 {
			fmt.Println("\nArguments:")
			for _, a := range schema.Args {
				fmt.Println(a.HelpLine())
			}
		}
		cases, err := lister.List(ctx, "", nil)
		if err == nil && len(cases) > 0 {
			fmt.Println("\nScenarios:")
			for _, c := range cases {
				fmt.Printf("  %s/%s  %s\n", schema.Name, c.ID, c.Title)
			}
		}
		return ExitOK, nil

	case "validate":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: usage: chaoslab plugin validate <dir>", plugin.ErrUserInput)
		}
		warnings, err := plugin.Validate(ctx, rest[0])
		if err != nil {
			return ExitPluginErr, err
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "warning: %s\n", w)
		}
		if len(warnings) == 0 {
			fmt.Printf("%s %s\n", glyphOK(), ok("protocol check passed"))
		}
		return ExitOK, nil

	default:
		return ExitUserError, fmt.Errorf("%w: unknown plugin subcommand %q (available: install|uninstall|info|validate)", plugin.ErrUserInput, sub)
	}
}
