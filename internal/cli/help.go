package cli

import (
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

// installHelp replaces cobra's default help with a compact, localised one.
// Cobra's templates say "Usage:", "Flags:", "Available Commands:" next to
// Chinese descriptions, and pflag pads with byte counts, which misaligns any
// table containing CJK text. Rendering from the flag set gives exact control.
func installHelp(root *cobra.Command) {
	var render func(c *cobra.Command) string
	render = func(c *cobra.Command) string {
		var b strings.Builder

		if c.Long != "" {
			b.WriteString(c.Long)
		} else {
			b.WriteString(c.Short)
		}
		b.WriteString("\n\n用法：")
		switch {
		case c.HasAvailableSubCommands():
			if c.Parent() == nil {
				b.WriteString("chaoslab [命令]")
			} else {
				b.WriteString(c.CommandPath() + " [命令]")
			}
		default:
			b.WriteString(strings.TrimSuffix(c.UseLine(), " [flags]"))
		}
		b.WriteString("\n")

		var subs []*cobra.Command
		for _, sub := range c.Commands() {
			if sub.IsAvailableCommand() && sub.Name() != "help" {
				subs = append(subs, sub)
			}
		}
		sort.Slice(subs, func(i, j int) bool { return subs[i].Name() < subs[j].Name() })
		if len(subs) > 0 {
			b.WriteString("\n可用命令：\n")
			for _, sub := range subs {
				b.WriteString("  " + pad(sub.Name(), 12) + sub.Short + "\n")
			}
		}

		if local := c.LocalFlags(); local.HasAvailableFlags() {
			b.WriteString("\n选项：\n")
			b.WriteString(flagLines(local, false))
		}
		if inherited := c.InheritedFlags(); inherited.HasAvailableFlags() {
			b.WriteString("\n全局选项：\n")
			b.WriteString(flagLines(inherited, true))
		}

		return strings.TrimRight(b.String(), "\n") + "\n"
	}

	root.SetHelpFunc(func(c *cobra.Command, args []string) { fmt.Print(render(c)) })
	root.SetUsageFunc(func(c *cobra.Command) error {
		fmt.Print(render(c))
		return nil
	})
}

// flagLines renders a flag set as "  <flag>   <usage>" with the descriptions
// aligned on the terminal column, not the byte count.
func flagLines(fs *pflag.FlagSet, withDefault bool) string {
	type row struct{ flag, usage string }
	var rows []row
	fs.VisitAll(func(f *pflag.Flag) {
		if f.Hidden {
			return
		}
		name := "    --" + f.Name
		if f.Shorthand != "" {
			name = "  -" + f.Shorthand + ", --" + f.Name
		}
		switch f.Value.Type() {
		case "string":
			name += " <值>"
		case "strings", "stringArray", "stringSlice", "stringToString":
			name += " <值>"
		}
		usage := f.Usage
		if strings.HasPrefix(usage, "help for ") {
			usage = "查看帮助"
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
	_ = withDefault
	return b.String()
}

// pad pads s to width terminal cells.
func pad(s string, width int) string {
	if n := displayWidth(s); n < width {
		return s + strings.Repeat(" ", width-n)
	}
	return s + " "
}
