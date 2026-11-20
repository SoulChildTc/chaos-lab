package cli

import (
	"context"
	"fmt"
	"os"

	"github.com/soulchild/chaos-lab/internal/plugin"
	"github.com/soulchild/chaos-lab/internal/state"
)

// RunPlugin dispatches the `plugin` subcommands. sub is one of
// install|uninstall|info|validate.
func RunPlugin(ctx context.Context, rt *Runtime, sub string, rest []string, asJSON bool) (int, error) {
	switch sub {
	case "install":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab plugin install <git-url|dir>", plugin.ErrUserInput)
		}
		name, err := plugin.Install(ctx, rest[0], rt.PluginsDir)
		if err != nil {
			return ExitUserError, err
		}
		fmt.Printf("已安装插件 %s\n", name)
		fmt.Printf("场景：chaoslab list\n")
		return ExitOK, nil

	case "uninstall":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab plugin uninstall <name>", plugin.ErrUserInput)
		}
		f, err := rt.Store.Read(ctx)
		if err != nil {
			return ExitUserError, err
		}
		if err := plugin.Uninstall(rt.PluginsDir, rest[0], f.Instances); err != nil {
			return ExitUserError, err
		}
		fmt.Printf("已卸载插件 %s\n", rest[0])
		return ExitOK, nil

	case "info":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab plugin info <name>", plugin.ErrUserInput)
		}
		lister := rt.Lister(rest[0])
		if _, err := os.Stat(lister.PluginDir + "/plugin"); err != nil {
			return ExitUserError, fmt.Errorf("%w: 插件 %s 未安装", plugin.ErrUserInput, rest[0])
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
			fmt.Println("\n参数：")
			for _, a := range schema.Args {
				fmt.Println(a.HelpLine())
			}
		}
		cases, err := lister.List(ctx, "", nil)
		if err == nil && len(cases) > 0 {
			fmt.Println("\n场景：")
			for _, c := range cases {
				fmt.Printf("  %s/%s  %s\n", schema.Name, c.ID, c.Title)
			}
		}
		return ExitOK, nil

	case "validate":
		if len(rest) != 1 {
			return ExitUserError, fmt.Errorf("%w: 用法 chaoslab plugin validate <dir>", plugin.ErrUserInput)
		}
		warnings, err := plugin.Validate(ctx, rest[0])
		if err != nil {
			return ExitPluginErr, err
		}
		for _, w := range warnings {
			fmt.Fprintf(os.Stderr, "警告：%s\n", w)
		}
		if len(warnings) == 0 {
			fmt.Println("协议检查通过")
		}
		return ExitOK, nil

	default:
		return ExitUserError, fmt.Errorf("%w: 未知的 plugin 子命令 %q（可用：install|uninstall|info|validate）", plugin.ErrUserInput, sub)
	}
}

var _ = state.PhaseDirty
