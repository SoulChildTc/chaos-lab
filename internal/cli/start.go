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

// RunStart creates one instance of a case: resolve the plugin, merge args,
// reserve a sequence number, ask the plugin which resources it will claim,
// reject conflicts, then let the plugin bring the environment up.
//
// The reservation, the claim query and the conflict check all happen inside one
// lock section. They cannot be split: a claim is only known after the plugin
// answers list, and another start must not slip in between reserving and
// claiming, or two instances would both hold the same port.
func RunStart(ctx context.Context, rt *Runtime, ref string, kv map[string]string, asJSON bool) (int, error) {
	pluginName, caseName, err := splitRef(ref)
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
	var conflict error
	err = rt.Store.Update(ctx, func(f *state.File) error {
		Converge(f)

		for _, existing := range f.Instances {
			if existing.Live && existing.Plugin == pluginName && existing.Case == caseName {
				return fmt.Errorf("%w: %s 正在运行（实例 %s，已运行 %s）；先 chaoslab stop %s",
					plugin.ErrUserInput, ref, existing.ID, shortDuration(time.Since(existing.StartedAt)), existing.ID)
			}
		}

		seq := f.Seq[pluginName+"/"+caseName] + 1
		slug := Slug(pluginName, caseName, seq)

		// The plugin renders claims for this exact instance and args. Unknown
		// prefixes are compared too, so the check stays honest even for claims
		// the framework cannot verify itself.
		cases, listErr := lister.List(ctx, slug, args)
		if listErr != nil {
			return listErr
		}
		theCase, found := findCase(cases, caseName)
		if !found {
			return fmt.Errorf("%w: 插件 %s 没有场景 %q", plugin.ErrUserInput, pluginName, caseName)
		}
		if id, claim, clash := claims.ConflictsWith(theCase.Claims, f.Instances); clash {
			conflict = fmt.Errorf("%w: %s 已经占用 %s", plugin.ErrUserInput, id, claim)
			return conflict
		}

		inst = state.Instance{
			ID:        fmt.Sprintf("%s/%s#%d", pluginName, caseName, seq),
			Plugin:    pluginName,
			Case:      caseName,
			Slug:      slug,
			Args:      args,
			Claims:    theCase.Claims,
			Phase:     state.PhaseStarting,
			Live:      true,
			PID:       os.Getpid(),
			StartedAt: time.Now().UTC(),
		}
		inst.RunPath = runPath(rt, inst)
		f.Seq[pluginName+"/"+caseName] = seq
		f.Instances = append(f.Instances, inst)
		return nil
	})
	if err != nil {
		return codeForStartError(err), err
	}

	timeout := parseTimeout(caseTimeout(ctx, lister, caseName, args, true), defaultUpTimeout)
	upCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, code, runErr := pluginCall(upCtx, pluginDir, inst, "up")
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
			return ExitPluginErr, fmt.Errorf("%s 启动失败：%s（且状态写入失败：%v）", inst.ID, msg, stateErr)
		}
		return ExitPluginErr, fmt.Errorf("%s 启动失败：%s", inst.ID, msg)
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
		return ExitPluginErr, fmt.Errorf("%s 环境已启动，但状态写入失败：%w", inst.ID, err)
	}

	inst.Phase = state.PhaseRunning
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

// codeForStartError separates a refused start (user can fix it) from a plugin
// that failed to answer the protocol.
func codeForStartError(err error) int {
	switch {
	case errors.Is(err, plugin.ErrUserInput):
		return ExitUserError
	case errors.Is(err, plugin.ErrProtocol):
		return ExitPluginErr
	default:
		return ExitPluginErr
	}
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

// caseTimeout asks the plugin for this case's timeout. Failures fall back to the
// default rather than blocking an action the user already asked for.
func caseTimeout(ctx context.Context, lister *plugin.Lister, caseName string, args map[string]any, up bool) string {
	cases, err := lister.List(ctx, "", args)
	if err != nil {
		return ""
	}
	c, ok := findCase(cases, caseName)
	if !ok {
		return ""
	}
	if up {
		return c.UpTimeout
	}
	return c.CheckTimeout
}

func describeFailure(err error, code int) string {
	if err != nil {
		return err.Error()
	}
	return "插件退出码 " + strconv.Itoa(code)
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
