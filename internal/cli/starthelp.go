package cli

import (
	"context"
	"fmt"
)

// printStartHelp answers `start <plugin> --help` with the plugin's own parameter
// table and the faults it will inject. These flags are declared at runtime, so
// cobra cannot generate them.
func printStartHelp(ctx context.Context, rt *Runtime, ref string) (int, error) {
	pluginName, _, err := parsePluginRef(ref)
	if err != nil {
		return ExitUserError, err
	}
	if _, err := rt.pluginDir(pluginName); err != nil {
		return ExitUserError, err
	}
	lister := rt.Lister(pluginName)
	schema, err := lister.Schema(ctx)
	if err != nil {
		return ExitPluginErr, err
	}
	catalog, err := lister.List(ctx, "", nil)
	if err != nil {
		return ExitPluginErr, err
	}

	fmt.Printf("%s", pluginName)
	if schema.Description != "" {
		fmt.Printf(" — %s", schema.Description)
	}
	fmt.Printf("\n\nUsage: chaoslab start %s [flags]\n", pluginName)

	if len(catalog.Scenarios) > 0 {
		fmt.Printf("\nStarting this plugin injects all %d of its faults:\n", len(catalog.Scenarios))
		width := 0
		for _, sc := range catalog.Scenarios {
			if w := displayWidth(sc.ID); w > width {
				width = w
			}
		}
		for _, sc := range catalog.Scenarios {
			fmt.Printf("  %s%s\n", pad(sc.ID, width+2), sc.Title)
		}
		fmt.Println()
		fmt.Println(dim("The symptom you are shown is the entry point; which faults are"))
		fmt.Println(dim("behind it is what you are meant to work out."))
	}

	if len(schema.Args) == 0 {
		fmt.Println("\nThis plugin takes no flags.")
		return ExitOK, nil
	}
	fmt.Println("\nFlags:")
	width := 0
	for _, a := range schema.Args {
		if w := displayWidth(a.FlagName()); w > width {
			width = w
		}
	}
	for _, a := range schema.Args {
		name := a.FlagName()
		fmt.Println("  " + name + repeat(" ", width-displayWidth(name)+2) + a.HelpSummary())
	}
	return ExitOK, nil
}
