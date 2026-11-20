package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

// ListEntry is one row of `list`: a fault a plugin can inject.
type ListEntry struct {
	Plugin   string `json:"plugin"`
	Fault    string `json:"fault"`
	Title    string `json:"title"`
}

// RunList prints every scenario every installed plugin provides. A filter limits
// it to one plugin. An unknown filter is an error, not an empty table.
func RunList(ctx context.Context, rt *Runtime, filter string, asJSON bool) (int, error) {
	names, err := pluginDiscover(rt)
	if err != nil {
		return ExitUserError, err
	}
	if filter != "" {
		found := false
		for _, n := range names {
			if n == filter {
				found = true
				break
			}
		}
		if !found {
			return ExitUserError, fmt.Errorf("plugin %s is not installed", filter)
		}
		names = []string{filter}
	}

	entries := make([]ListEntry, 0, len(names))
	for _, name := range names {
		catalog, err := rt.Lister(name).List(ctx, "", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping plugin %s: %v\n", name, err)
			continue
		}
		for _, sc := range catalog.Scenarios {
			entries = append(entries, ListEntry{Plugin: name, Fault: sc.ID, Title: sc.Title})
		}
	}

	if asJSON {
		return ExitOK, printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("No scenarios yet. Install a plugin first: chaoslab plugin install <dir>")
		return ExitOK, nil
	}

	nameWidth := 0
	for _, e := range entries {
		if w := displayWidth(e.Plugin); w > nameWidth {
			nameWidth = w
		}
	}
	scWidth := 0
	for _, e := range entries {
		if w := displayWidth(e.Fault); w > scWidth {
			scWidth = w
		}
	}

	fmt.Println(head(pad("PLUGIN", nameWidth+2) + pad("FAULT", scWidth+2) + "DESCRIPTION"))
	for _, e := range entries {
		fmt.Println(pad(e.Plugin, nameWidth+2) + pad(e.Fault, scWidth+2) + e.Title)
	}
	fmt.Println()
	fmt.Println(dim("These are the faults a plugin can inject. chaoslab start <plugin> injects all of them."))
	return ExitOK, nil
}

func pluginDiscover(rt *Runtime) ([]string, error) {
	return plugin.Discover(rt.PluginsDir)
}
