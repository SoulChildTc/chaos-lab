package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// Command groups, in display order. Professional CLI help tells you which
// commands you will need first instead of dumping one alphabetical list.
type group struct {
	title string
	names []string
}

var groups = []group{
	{"Scenarios", []string{"list", "start", "check", "status", "stop", "rm"}},
	{"Plugins", []string{"plugin"}},
	{"Other", []string{"version"}},
}

// installHelp replaces cobra's default help. Two reasons it is hand-rendered:
// cobra's headings are English next to Chinese descriptions, and pflag pads by
// byte count, which misaligns any table containing CJK text.
func installHelp(root *cobra.Command) {
	var render func(c *cobra.Command) string
	render = func(c *cobra.Command) string {
		var b strings.Builder

		if c.Long != "" {
			b.WriteString(c.Long)
		} else {
			b.WriteString(c.Short)
		}
		b.WriteString("\n\n")

		b.WriteString("Usage:\n")
		if c.HasAvailableSubCommands() {
			path := c.CommandPath()
			if c.Parent() == nil {
				path = c.Name()
			}
			b.WriteString("  " + path + " <command> [flags]\n")
		} else {
			b.WriteString("  " + strings.TrimSuffix(c.UseLine(), " [flags]") + " [flags]\n")
		}

		b.WriteString(renderCommands(c))
		if local := c.LocalFlags(); local.HasAvailableFlags() {
			b.WriteString("\nFlags:\n")
			b.WriteString(flagLines(local))
		}
		if inherited := c.InheritedFlags(); inherited.HasAvailableFlags() {
			b.WriteString("\nGlobal Flags:\n")
			b.WriteString(flagLines(inherited))
		}

		if c.HasAvailableSubCommands() {
			b.WriteString("\nUse \"chaoslab <command> --help\" for more information about a command.\n")
		}

		return strings.TrimRight(b.String(), "\n") + "\n"
	}

	root.SetHelpFunc(func(c *cobra.Command, args []string) { fmt.Print(render(c)) })
	root.SetUsageFunc(func(c *cobra.Command) error {
		fmt.Print(render(c))
		return nil
	})
}

// renderCommands prints each group's commands, aligning names on the widest one
// across all groups so the list reads as a single table.
func renderCommands(c *cobra.Command) string {
	available := map[string]*cobra.Command{}
	var rest []*cobra.Command
	for _, sub := range c.Commands() {
		if !sub.IsAvailableCommand() || sub.Name() == "help" {
			continue
		}
		available[sub.Name()] = sub
	}
	grouped := map[string]bool{}
	for _, g := range groups {
		for _, n := range g.names {
			if _, ok := available[n]; ok {
				grouped[n] = true
			}
		}
	}
	for _, sub := range available {
		if !grouped[sub.Name()] {
			rest = append(rest, sub)
		}
	}
	sort.Slice(rest, func(i, j int) bool { return rest[i].Name() < rest[j].Name() })

	width := 0
	for _, sub := range available {
		if w := displayWidth(sub.Name()); w > width {
			width = w
		}
	}

	var b strings.Builder
	writeGroup := func(title string, subs []*cobra.Command) {
		if len(subs) == 0 {
			return
		}
		b.WriteString("\n" + title + ":\n")
		for _, sub := range subs {
			b.WriteString("  " + pad(sub.Name(), width+2) + sub.Short + "\n")
		}
	}

	for _, g := range groups {
		var subs []*cobra.Command
		for _, n := range g.names {
			if sub, ok := available[n]; ok {
				subs = append(subs, sub)
			}
		}
		writeGroup(g.title, subs)
	}
	// Grouping is for the root command. A subcommand's subcommands are one flat
	// list; calling a subcommand's children "Other" would be nonsense.
	if c.Parent() == nil {
		writeGroup("Other", rest)
	} else {
		writeGroup("Commands", rest)
	}
	return b.String()
}

// flagLines renders a flag set with the descriptions aligned on the terminal
// column. Flags without a shorthand are indented so every "--" starts in the
// same place, which is the convention users expect.
func flagLines(fs *pflag.FlagSet) string {
	type row struct{ flag, usage string }
	var rows []row
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		var name string
		if f.Shorthand != "" {
			name = "  -" + f.Shorthand + ", --" + f.Name
		} else {
			name = "      --" + f.Name
		}
		switch f.Value.Type() {
		case "string", "strings", "stringArray", "stringSlice", "stringToString", "int", "bool":
			// These are documented in their own usage strings; appending a type
			// placeholder would duplicate the convention for --json style flags.
			name += ""
		}
		usage := f.Usage
		if strings.HasPrefix(usage, "help for ") {
			usage = "help for this command"
		}
		rows = append(rows, row{name, usage})
	})
	if len(rows) == 0 {
		return ""
	}
	width := 0
	for _, r := range rows {
		if w := displayWidth(r.flag); w > width {
			width = w
		}
	}
	var b strings.Builder
	for _, r := range rows {
		b.WriteString(r.flag)
		b.WriteString(strings.Repeat(" ", width-displayWidth(r.flag)+2))
		b.WriteString(r.usage + "\n")
	}
	return b.String()
}

// pad pads s to width terminal cells.
func pad(s string, width int) string {
	if n := displayWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s + " "
}

// repeat is strings.Repeat with a guard against negative counts, which the
// alignment helpers can produce for an empty cell.
func repeat(s string, n int) string {
	if n <= 0 {
		return ""
	}
	return strings.Repeat(s, n)
}
