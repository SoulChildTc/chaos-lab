package cli

import (
	"context"
	"encoding/json"
	"os"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// pluginCall runs one lifecycle subcommand of a plugin for an instance and
// returns its stdout. Args go in a temp file so types and quoting survive.
func pluginCall(ctx context.Context, pluginDir string, inst state.Instance, action string) ([]byte, int, error) {
	argsJSON, err := json.Marshal(inst.Args)
	if err != nil {
		return nil, -1, err
	}
	argsFile, err := os.CreateTemp("", "chaoslab-args-*.json")
	if err != nil {
		return nil, -1, err
	}
	defer os.Remove(argsFile.Name())
	if _, err := argsFile.Write(argsJSON); err != nil {
		argsFile.Close()
		return nil, -1, err
	}
	argsFile.Close()

	return plugin.Run(ctx, pluginDir, pluginEnv(inst.Plugin, pluginDir, inst.Slug), pluginDir+"/plugin",
		action, "--instance="+inst.Slug, "--args=@"+argsFile.Name())
}
