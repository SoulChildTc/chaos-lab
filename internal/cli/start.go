package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"time"

	"github.com/soulchild/chaos-lab/internal/claims"
	"github.com/soulchild/chaos-lab/internal/plugin"
	"github.com/soulchild/chaos-lab/internal/runlog"
	"github.com/soulchild/chaos-lab/internal/state"
)

const defaultUpTimeout = 5 * time.Minute

// RunStart creates one instance of a case: resolve the plugin, merge args,
// allocate a sequence number, ask the plugin which resources it will claim,
// reject conflicts, then let the plugin bring the environment up.
func RunStart(ctx context.Context, rt *Runtime, ref string, kv map[string]string, asJSON bool) (int, error) {
	pluginName, caseName, err := splitRef(ref)
	if err != nil {
		return ExitUserError, err
	}

	lister := rt.Lister(pluginName)
	if _, statErr := os.Stat(lister.PluginDir + "/plugin"); statErr != nil {
		return ExitUserError, fmt.Errorf("%w: 插件 %s 未安装（chaoslab plugin install）", plugin.ErrUserInput, pluginName)
	}
	schema, err := lister.Schema(ctx)
	if err != nil {
		return ExitPluginErr, err
	}
	args, err := plugin.Merge(schema, kv)
	if err != nil {
		return ExitUserError, err
	}

	// The plugin decides what this case will hold. It needs the instance slug to
	// do that, so the sequence number is allocated first.
	var inst state.Instance
	err = rt.Store.Update(ctx, func(f *state.File) error {
		Converge(f)

		for _, existing := range f.Instances {
			if existing.Live && existing.Plugin == pluginName && existing.Case == caseName {
				return fmt.Errorf("%w: %s is already running as %s（已运行 %s）；先 chaoslab stop %s",
					plugin.ErrUserInput, ref, existing.ID, shortDuration(time.Since(existing.StartedAt)), existing.ID)
			}
		}

		seq := f.Seq[pluginName+"/"+caseName] + 1
		slug := Slug(pluginName, caseName, seq)
		inst = state.Instance{
			ID:        fmt.Sprintf("%s/%s#%d", pluginName, caseName, seq),
			Plugin:    pluginName,
			Case:      caseName,
			Slug:      slug,
			Args:      args,
			Phase:     state.PhaseStarting,
			Live:      true,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
		}
		f.Seq[pluginName+"/"+caseName] = seq
		f.Instances = append(f.Instances, inst)
		return nil
	})
	if err != nil {
		return ExitUserError, err
	}

	// Claims are only known once the slug exists. A conflict rolls the record back
	// so a refused start leaves no trace.
	cases, err := lister.List(ctx, inst.Slug, args)
	if err != nil {
		rt.dropInstance(ctx, inst.ID)
		return ExitPluginErr, err
	}
	theCase, ok := findCase(cases, caseName)
	if !ok {
		rt.dropInstance(ctx, inst.ID)
		return ExitUserError, fmt.Errorf("%w: plugin %s has no case %q", plugin.ErrUserInput, pluginName, caseName)
	}
	if id, claim, found := claims.ConflictsWith(theCase.Claims, mustRead(ctx, rt)); found {
		rt.dropInstance(ctx, inst.ID)
		return ExitUserError, fmt.Errorf("%w: %s already holds %s", plugin.ErrUserInput, id, claim)
	}

	if err := rt.Store.Update(ctx, func(f *state.File) error {
		for i := range f.Instances {
			if f.Instances[i].ID == inst.ID {
				f.Instances[i].Claims = theCase.Claims
				f.Instances[i].RunPath = runPath(rt, inst)
			}
		}
		return nil
	}); err != nil {
		rt.dropInstance(ctx, inst.ID)
		return ExitUserError, err
	}
	inst.Claims = theCase.Claims

	timeout := parseTimeout(theCase.UpTimeout, defaultUpTimeout)
	upCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	pluginDir := rt.Lister(pluginName).PluginDir
	_, code, runErr := pluginCall(upCtx, pluginDir, inst, "up")
	if runErr != nil || code != 0 {
		msg := describeFailure(runErr, code)
		code := ExitPluginErr
		if errors.Is(runErr, context.DeadlineExceeded) || errors.Is(runErr, plugin.ErrTimeout) {
			msg = fmt.Sprintf("环境启动超过 %s 被终止", timeout)
		}
		_ = rt.Store.Update(ctx, func(f *state.File) error {
			for i := range f.Instances {
				if f.Instances[i].ID == inst.ID {
					f.Instances[i].Phase = state.PhaseStopped
					f.Instances[i].Live = true
					f.Instances[i].PID = 0
					f.Instances[i].Error = &msg
				}
			}
			return nil
		})
		writeRunRecord(rt, inst, runlog.ResultStartFailed, nil, nil)
		return code, fmt.Errorf("%s 启动失败：%s", inst.ID, msg)
	}

	if err := rt.Store.Update(ctx, func(f *state.File) error {
		for i := range f.Instances {
			if f.Instances[i].ID == inst.ID {
				f.Instances[i].Phase = state.PhaseRunning
				f.Instances[i].PID = 0
			}
		}
		return nil
	}); err != nil {
		return ExitUserError, err
	}

	if asJSON {
		return ExitOK, printJSON(struct {
			Instance string `json:"instance"`
			Slug     string `json:"slug"`
			Phase    string `json:"phase"`
		}{inst.ID, inst.Slug, string(state.PhaseRunning)})
	}
	fmt.Printf("%s 已就绪（slug %s）\n", inst.ID, inst.Slug)
	fmt.Printf("修好后运行：chaoslab check %s\n", inst.ID)
	return ExitOK, nil
}

func (rt *Runtime) dropInstance(ctx context.Context, id string) {
	_ = rt.Store.Update(ctx, func(f *state.File) error {
		out := f.Instances[:0]
		for _, in := range f.Instances {
			if in.ID != id {
				out = append(out, in)
			}
		}
		f.Instances = out
		return nil
	})
}

func mustRead(ctx context.Context, rt *Runtime) []state.Instance {
	f, err := rt.Store.Read(ctx)
	if err != nil {
		return nil
	}
	return f.Instances
}

func findCase(cases []plugin.Case, id string) (plugin.Case, bool) {
	for _, c := range cases {
		if c.ID == id {
			return c, true
		}
	}
	return plugin.Case{}, false
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

func describeFailure(err error, code int) string {
	if err != nil {
		return err.Error()
	}
	return "plugin exited with code " + strconv.Itoa(code)
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
