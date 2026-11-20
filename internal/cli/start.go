package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/soulchildtc/chaos-lab/internal/claims"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/runlog"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

const defaultUpTimeout = 5 * time.Minute

// RunStart brings one plugin's environment up. The plugin decides which faults
// it injects and how they share resources; the framework only records the
// instance, checks for claim collisions and relays the symptom and connection
// details to the user.
func RunStart(ctx context.Context, rt *Runtime, ref string, kv map[string]string, asJSON bool) (int, error) {
	pluginName, _, err := parsePluginRef(ref)
	if err != nil {
		return ExitUserError, err
	}

	pluginDir, err := rt.pluginDir(pluginName)
	if err != nil {
		return ExitUserError, err
	}
	lister := &plugin.Lister{PluginDir: pluginDir}
	schema, err := lister.Schema(ctx)
	if err != nil {
		return ExitPluginErr, err
	}
	args, err := plugin.Merge(schema, kv)
	if err != nil {
		return ExitUserError, err
	}

	var inst state.Instance
	err = rt.Store.Update(ctx, func(f *state.File) error {
		Converge(f)

		for _, existing := range f.Instances {
			if existing.Live && existing.Plugin == pluginName {
				return fmt.Errorf("%w: %s is already running as %s (up %s); stop it first: chaoslab stop %s",
					plugin.ErrUserInput, pluginName, existing.ID, shortDuration(time.Since(existing.StartedAt)), existing.ID)
			}
		}

		seq := f.Seq[pluginName] + 1
		slug := Slug(pluginName, seq)

		// The catalogue tells us which resources this instance will hold. Claims
		// are rendered by the plugin for this exact instance and args.
		catalog, listErr := lister.List(ctx, slug, args)
		if listErr != nil {
			return listErr
		}
		if id, claim, clash := claims.ConflictsWith(catalog.Claims, f.Instances); clash {
			return fmt.Errorf("%w: %s already holds %s", plugin.ErrUserInput, id, claim)
		}

		inst = state.Instance{
			ID:        fmt.Sprintf("%s#%d", pluginName, seq),
			Plugin:    pluginName,
			Slug:      slug,
			Args:      args,
			Claims:    catalog.Claims,
			Phase:     state.PhaseStarting,
			Live:      true,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
		}
		inst.RunPath = runPath(rt, inst)
		f.Seq[pluginName] = seq
		f.Instances = append(f.Instances, inst)
		return nil
	})
	if err != nil {
		return codeForStartError(err), err
	}

	timeout := parseTimeout(caseTimeout(ctx, lister, args), defaultUpTimeout)
	upCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	stdout, code, runErr := pluginCall(upCtx, pluginDir, inst, "up")
	if runErr != nil || code != 0 {
		msg := describeFailure(runErr, code)
		if runErr != nil {
			msg = runErr.Error()
		}
		stateErr := rt.Store.Update(ctx, func(f *state.File) error {
			for i := range f.Instances {
				if f.Instances[i].ID != inst.ID {
					continue
				}
				f.Instances[i].Phase = state.PhaseStopped
				// up may have created resources before failing, so the instance
				// keeps owning them until a stop proves otherwise.
				f.Instances[i].Live = true
				f.Instances[i].PID = 0
				f.Instances[i].Error = &msg
			}
			return nil
		})
		writeRunRecord(rt, rt.reload(ctx, inst), runlog.ResultStartFailed, nil, nil, nil)
		if stateErr != nil {
			return ExitPluginErr, fmt.Errorf("%s failed to start: %s (and the state write failed: %v)", inst.ID, msg, stateErr)
		}
		return ExitPluginErr, fmt.Errorf("%s failed to start: %s", inst.ID, msg)
	}

	if err := rt.Store.Update(ctx, func(f *state.File) error {
		for i := range f.Instances {
			if f.Instances[i].ID != inst.ID {
				continue
			}
			f.Instances[i].Phase = state.PhaseRunning
			f.Instances[i].PID = 0
		}
		return nil
	}); err != nil {
		// The environment is up but unrecorded; say so instead of pretending.
		return ExitPluginErr, fmt.Errorf("%s is up, but recording its state failed: %w", inst.ID, err)
	}
	inst.Phase = state.PhaseRunning

	// The symptom and connection details come from the plugin. Without them the
	// user has nothing to investigate and nowhere to connect.
	result, err := plugin.ParseUpResult(stdout)
	if err != nil {
		return ExitPluginErr, err
	}

	if asJSON {
		return ExitOK, printJSON(struct {
			Instance string   `json:"instance"`
			Slug     string   `json:"slug"`
			Phase    string   `json:"phase"`
			Symptom  string   `json:"symptom,omitempty"`
			Connect  []string `json:"connect,omitempty"`
		}{inst.ID, inst.Slug, string(state.PhaseRunning), result.Symptom, result.Connect})
	}

	fmt.Printf("%s %s %s\n", glyphOK(), ok("ready"), dim(inst.ID))
	if result.Symptom != "" {
		fmt.Printf("\n%s\n", head("SYMPTOM"))
		for _, line := range wrap(result.Symptom, 74) {
			fmt.Println("  " + line)
		}
	}
	if len(result.Connect) > 0 {
		fmt.Printf("\n%s\n", head("CONNECT"))
		for _, c := range result.Connect {
			fmt.Println("  " + c)
		}
	}
	fmt.Printf("\n%s\n", dim("when you think it is fixed: chaoslab check "+inst.ID))
	return ExitOK, nil
}

// codeForStartError separates a refused start (user can fix it) from a plugin
// that failed to answer the protocol.
func codeForStartError(err error) int {
	switch {
	case errors.Is(err, plugin.ErrUserInput):
		return ExitUserError
	default:
		return ExitPluginErr
	}
}

func parseTimeout(s string, fallback time.Duration) time.Duration {
	if s == "" {
		return fallback
	}
	d, err := time.ParseDuration(s)
	if err != nil || d <= 0 {
		return fallback
	}
	return d
}

// caseTimeout asks the plugin how long its environment may take to come up.
// A failure here falls back to the default rather than blocking the start.
func caseTimeout(ctx context.Context, lister *plugin.Lister, args map[string]any) string {
	c, err := lister.List(ctx, "", args)
	if err != nil {
		return ""
	}
	return c.UpTimeout
}

func describeFailure(err error, code int) string {
	if err != nil {
		return err.Error()
	}
	return "plugin exited " + strconv.Itoa(code)
}

func printJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	fmt.Println(string(b))
	return nil
}

func shortDuration(d time.Duration) string {
	d = d.Round(time.Second)
	if d < time.Minute {
		return d.String()
	}
	return d.Round(10 * time.Second).String()
}
