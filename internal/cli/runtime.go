// Package cli implements every chaoslab subcommand. Each command returns an
// exit code; the caller maps that to the process exit status.
package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/soulchild/chaos-lab/internal/home"
	"github.com/soulchild/chaos-lab/internal/plugin"
	"github.com/soulchild/chaos-lab/internal/state"
)

// Exit codes. The numbers are part of the CLI contract (see the spec).
const (
	ExitOK        = 0
	ExitUserError = 1
	ExitNotPassed = 2
	ExitPluginErr = 3
)

// Runtime is everything a command needs. Constructing one does not touch disk
// beyond resolving the home directory.
type Runtime struct {
	Home       string
	Store      *state.Store
	PluginsDir string
}

// NewRuntime wires the paths under one home directory.
func NewRuntime(h string) *Runtime {
	return &Runtime{
		Home:       h,
		Store:      state.Open(filepath.Join(h, "state", "instances.json")),
		PluginsDir: filepath.Join(h, "plugins"),
	}
}

// NewRuntimeDefault builds a Runtime for the real ~/.lab (or CHAOSLAB_HOME).
func NewRuntimeDefault() (*Runtime, error) {
	h, err := home.Dir()
	if err != nil {
		return nil, err
	}
	return NewRuntime(h), nil
}

// pluginDir resolves an installed plugin's directory, rejecting names that could
// escape ~/.lab/plugins (a user-supplied name is joined into a path).
func (rt *Runtime) pluginDir(name string) (string, error) {
	if !pluginNameRe.MatchString(name) {
		return "", fmt.Errorf("%w: %q 不是合法的插件名", plugin.ErrUserInput, name)
	}
	dir := filepath.Join(rt.PluginsDir, name)
	if _, err := os.Stat(filepath.Join(dir, "plugin")); err != nil {
		return "", fmt.Errorf("%w: 插件 %s 未安装（chaoslab plugin install）", plugin.ErrUserInput, name)
	}
	return dir, nil
}

// Lister returns the protocol client for one installed plugin.
func (rt *Runtime) Lister(name string) *plugin.Lister {
	return &plugin.Lister{PluginDir: filepath.Join(rt.PluginsDir, name)}
}

var slugRe = regexp.MustCompile(`[^a-z0-9]+`)

// pluginNameRe matches the protocol's plugin-name rule; state and filesystem
// paths are derived from it, so it is enforced on input, not only in the plugin.
var pluginNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// Slug builds the identifier the plugin uses to name its resources. It must
// satisfy common resource naming rules (lowercase alnum and hyphen).
func Slug(pluginName, caseName string, seq int) string {
	raw := fmt.Sprintf("%s-%s-%d", pluginName, caseName, seq)
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(raw), "-"), "-")
}

// splitRef turns "plugin/case" into its parts.
func splitRef(ref string) (pluginName, caseName string, err error) {
	parts := strings.Split(ref, "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("%w: %q is not a <plugin>/<case> reference", plugin.ErrUserInput, ref)
	}
	return parts[0], parts[1], nil
}

// Resolve finds the instance a command refers to. ref may be a full instance ID
// ("plugin/case#2") or a case reference ("plugin/case") when that case has
// exactly one live instance.
func Resolve(instances []state.Instance, ref string) (*state.Instance, error) {
	if i := strings.LastIndex(ref, "#"); i > 0 {
		for n := range instances {
			if instances[n].ID == ref {
				return &instances[n], nil
			}
		}
		return nil, fmt.Errorf("%w: no instance %q", plugin.ErrUserInput, ref)
	}

	var matches []state.Instance
	for _, in := range instances {
		if in.Live && in.Plugin+"/"+in.Case == ref {
			matches = append(matches, in)
		}
	}
	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		return nil, fmt.Errorf("%w: %s is not running", plugin.ErrUserInput, ref)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("%w: %s has several live instances (%s); use one of those IDs",
			plugin.ErrUserInput, ref, strings.Join(ids, ", "))
	}
}

// Converge repairs records left behind by a framework process that was killed
// mid-start. It only observes the pid: no background daemon, no retry.
func Converge(f *state.File) {
	for i := range f.Instances {
		in := &f.Instances[i]
		if in.Phase != state.PhaseStarting || in.PID <= 0 {
			continue
		}
		if processAlive(in.PID) {
			continue
		}
		msg := "启动过程被中断（chaoslab 进程已不存在）"
		in.Error = &msg
		in.Phase = state.PhaseStopped
		// The plugin may have created resources before dying, so the instance
		// keeps owning them until a stop proves otherwise.
		in.Live = true
		in.PID = 0
	}
}

func pluginEnv(pluginName, pluginDir, instance, caseName string) []string {
	return []string{
		"LAB_INSTANCE=" + instance,
		"LAB_PLUGIN_DIR=" + pluginDir,
		"LAB_CASE=" + caseName,
	}
}

// reload re-reads one instance so a record written right after a store update
// sees the updated counter and run path.
func (rt *Runtime) reload(ctx context.Context, inst state.Instance) state.Instance {
	f, err := rt.Store.Read(ctx)
	if err != nil {
		return inst
	}
	for _, in := range f.Instances {
		if in.ID == inst.ID {
			return in
		}
	}
	return inst
}
