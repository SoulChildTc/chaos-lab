package cli

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

// ListEntry is one row of `list`: a case, addressed as plugin/case.
type ListEntry struct {
	Plugin string `json:"plugin"`
	Case   string `json:"case"`
	Title  string `json:"title"`
}

// RunList prints the cases of installed plugins. An empty filter means every
// plugin; a filter naming an unknown or broken plugin is an error, not an empty
// table. A plugin that fails to answer is reported on stderr and skipped, so one
// broken plugin does not hide the rest.
func RunList(ctx context.Context, rt *Runtime, filter string, asJSON bool) (int, error) {
	names, err := plugin.Discover(rt.PluginsDir)
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
			return ExitUserError, fmt.Errorf("%w: plugin %s is not installed", plugin.ErrUserInput, filter)
		}
		names = []string{filter}
	}

	entries := make([]ListEntry, 0, len(names))
	for _, name := range names {
		cases, err := rt.Lister(name).List(ctx, "", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipping plugin %s: %v\n", name, err)
			continue
		}
		for _, c := range cases {
			entries = append(entries, ListEntry{Plugin: name, Case: c.ID, Title: c.Title})
		}
	}

	if asJSON {
		return ExitOK, printJSON(entries)
	}
	if len(entries) == 0 {
		fmt.Println("No scenarios yet. Install a plugin first: chaoslab plugin install <dir>")
		return ExitOK, nil
	}
	rows := make([][]string, 0, len(entries))
	nameWidth := 0
	for _, e := range entries {
		name := e.Plugin + "/" + e.Case
		if w := displayWidth(name); w > nameWidth {
			nameWidth = w
		}
		rows = append(rows, []string{name, e.Title})
	}
	fmt.Println(head("SCENARIO" + strings.Repeat(" ", nameWidth-displayWidth("SCENARIO")+2) + "DESCRIPTION"))
	for _, r := range rows {
		fmt.Println(r[0] + strings.Repeat(" ", nameWidth-displayWidth(r[0])+2) + r[1])
	}
	return ExitOK, nil
}
