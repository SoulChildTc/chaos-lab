package cli

import (
	"context"
	"fmt"
	"os"
	"text/tabwriter"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

// ListEntry is one row of `list`: a case, addressed as plugin/case.
type ListEntry struct {
	Plugin string `json:"plugin"`
	Case   string `json:"case"`
	Title  string `json:"title"`
}

// RunList prints every case of every installed plugin. A plugin that fails to
// answer is reported on stderr and skipped, so one broken plugin does not hide
// the rest.
func RunList(ctx context.Context, rt *Runtime, asJSON bool) (int, error) {
	names, err := plugin.Discover(rt.PluginsDir)
	if err != nil {
		return ExitUserError, err
	}

	entries := make([]ListEntry, 0, len(names))
	for _, name := range names {
		cases, err := rt.Lister(name).List(ctx, "", nil)
		if err != nil {
			fmt.Fprintf(os.Stderr, "跳过插件 %s：%v\n", name, err)
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
		fmt.Println("没有可用的场景。用 chaoslab plugin install 安装插件。")
		return ExitOK, nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "场景\t说明")
	for _, e := range entries {
		fmt.Fprintf(w, "%s/%s\t%s\n", e.Plugin, e.Case, e.Title)
	}
	return ExitOK, w.Flush()
}
