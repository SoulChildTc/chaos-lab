package cli

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/soulchildtc/chaos-lab/internal/home"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
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
		return "", fmt.Errorf("%w: %q is not a valid plugin name", plugin.ErrUserInput, name)
	}
	dir := filepath.Join(rt.PluginsDir, name)
	if _, err := os.Stat(filepath.Join(dir, "plugin")); err != nil {
		return "", fmt.Errorf("%w: plugin %s is not installed (chaoslab plugin install)", plugin.ErrUserInput, name)
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
func Slug(pluginName string, seq int) string {
	raw := fmt.Sprintf("%s-%d", pluginName, seq)
	return strings.Trim(slugRe.ReplaceAllString(strings.ToLower(raw), "-"), "-")
}

// parsePluginRef accepts "plugin" or "plugin#3". A scenario name is not a valid
// reference: the plugin decides which faults it injects, so there is no
// per-fault addressing on the command line.
func parsePluginRef(ref string) (pluginName, instanceID string, err error) {
	if i := strings.Index(ref, "#"); i >= 0 {
		pluginName = ref[:i]
		if !pluginNameRe.MatchString(pluginName) {
			return "", "", fmt.Errorf("%w: %q is not a plugin instance", plugin.ErrUserInput, ref)
		}
		return pluginName, ref, nil
	}
	if strings.Contains(ref, "/") {
		return "", "", fmt.Errorf("%w: %q addresses a scenario; a plugin injects all of its faults at once, so use %s",
			plugin.ErrUserInput, ref, strings.SplitN(ref, "/", 2)[0])
	}
	if !pluginNameRe.MatchString(ref) {
		return "", "", fmt.Errorf("%w: %q is not a plugin name", plugin.ErrUserInput, ref)
	}
	return ref, "", nil
}

// Resolve finds the live instance a command refers to. ref is "plugin" (exactly
// one live instance, or none) or the full instance id "plugin#3".
func Resolve(instances []state.Instance, ref string) (*state.Instance, error) {
	pluginName, instanceID, err := parsePluginRef(ref)
	if err != nil {
		return nil, err
	}

	var matches []state.Instance
	for i := range instances {
		in := &instances[i]
		if !in.Live || in.Plugin != pluginName {
			continue
		}
		if instanceID != "" && in.ID != instanceID {
			continue
		}
		matches = append(matches, *in)
	}

	switch len(matches) {
	case 1:
		return &matches[0], nil
	case 0:
		if instanceID != "" {
			return nil, fmt.Errorf("%w: no live instance %q", plugin.ErrUserInput, instanceID)
		}
		return nil, fmt.Errorf("%w: %s is not running", plugin.ErrUserInput, pluginName)
	default:
		ids := make([]string, len(matches))
		for i, m := range matches {
			ids[i] = m.ID
		}
		return nil, fmt.Errorf("%w: %s has several live instances (%s); use one of those IDs",
			plugin.ErrUserInput, pluginName, strings.Join(ids, ", "))
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
		msg := "start was interrupted (the chaoslab process is gone)"
		in.Error = &msg
		in.Phase = state.PhaseStopped
		// The plugin may have created resources before dying, so the instance
		// keeps owning them until a stop proves otherwise.
		in.Live = true
		in.PID = 0
	}
}

func pluginEnv(pluginName, pluginDir, instanceID string) []string {
	return []string{
		"LAB_INSTANCE=" + instanceID,
		"LAB_PLUGIN_DIR=" + pluginDir,
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
