package cli

import (
	"context"
	"fmt"
	"strings"
)

// printStartHelp answers `start <ref> --help` with the plugin's own parameter
// table. These flags are declared at runtime, so cobra cannot generate them.
func printStartHelp(ctx context.Context, rt *Runtime, ref string) (int, error) {
	pluginName, caseName, err := splitRef(ref)
	if err != nil {
		return ExitUserError, err
	}
	pluginDir, err := rt.pluginDir(pluginName)
	if err != nil {
		return ExitUserError, err
	}
	lister := rt.Lister(pluginName)
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
		return ExitUserError, fmt.Errorf("plugin %s has no scenario %q; see chaoslab list %s", pluginName, caseName, pluginName)
	}
	_ = pluginDir

	fmt.Printf("%s/%s — %s\n\n", pluginName, c.ID, c.Title)
	fmt.Printf("Usage: chaoslab start %s/%s [flags]\n\n", pluginName, c.ID)
	if len(schema.Args) == 0 {
		fmt.Println("This scenario takes no flags.")
		return ExitOK, nil
	}
	fmt.Println("Flags:")
	width := 0
	for _, a := range schema.Args {
		if w := displayWidth(a.FlagName()); w > width {
			width = w
		}
	}
	for _, a := range schema.Args {
		name := a.FlagName()
		fmt.Println("  " + name + strings.Repeat(" ", width-displayWidth(name)+2) + a.HelpSummary())
	}
	return ExitOK, nil
}
