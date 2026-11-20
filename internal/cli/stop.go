package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/soulchild/chaos-lab/internal/claims"
	"github.com/soulchild/chaos-lab/internal/plugin"
	"github.com/soulchild/chaos-lab/internal/runlog"
	"github.com/soulchild/chaos-lab/internal/state"
)

// StopOutput is the JSON shape of `stop --json`: what the plugin claimed to
// release, and what the framework measured afterwards.
type StopOutput struct {
	Instance       string           `json:"instance"`
	Live           bool             `json:"live"`
	Released       bool             `json:"released"`
	Phase          string           `json:"phase"`
	ClaimsReleased *bool            `json:"claims_released,omitempty"`
	Verdicts       []claims.Verdict `json:"verdicts"`
}

// RunStop asks the plugin to tear the environment down, then measures the
// claims itself. The framework never cleans up: it only reports whether the
// plugin did.
func RunStop(ctx context.Context, rt *Runtime, ref string, asJSON bool) (int, error) {
	f, err := rt.Store.Read(ctx)
	if err != nil {
		return ExitUserError, err
	}
	Converge(f)
	inst, err := Resolve(f.Instances, ref)
	if err != nil {
		return ExitUserError, err
	}

	pluginDir := rt.Lister(inst.Plugin).PluginDir
	timeout := defaultUpTimeout
	if cases, err := rt.Lister(inst.Plugin).List(ctx, inst.Slug, inst.Args); err == nil {
		if c, ok := findCase(cases, inst.Case); ok {
			timeout = parseTimeout(c.UpTimeout, defaultUpTimeout)
		}
	}
	downCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, code, runErr := pluginCall(downCtx, pluginDir, *inst, "down")

	var pluginMsg string
	if runErr != nil {
		if errors.Is(runErr, plugin.ErrTimeout) {
			pluginMsg = fmt.Sprintf("down 超过 %s 被终止", timeout)
		} else {
			pluginMsg = runErr.Error()
		}
	} else if code != 0 {
		pluginMsg = fmt.Sprintf("down 退出码 %d", code)
	}

	// Measure even when down failed: the user needs to know what is still there,
	// not just that the plugin errored. Unverifiable claims (unknown prefix) are
	// the plugin's responsibility and do not make the instance dirty.
	verdicts := claims.Verify(ctx, inst.Claims)
	residuals := claims.Residuals(verdicts)
	unchecked := claims.Unchecked(verdicts)
	clean := pluginMsg == "" && len(residuals) == 0

	released := clean
	var msg string
	switch {
	case !clean && pluginMsg != "":
		msg = pluginMsg
	case !clean:
		parts := make([]string, 0, len(residuals))
		for _, v := range residuals {
			parts = append(parts, fmt.Sprintf("%s（%s）", v.Claim, v.Detail))
		}
		msg = "down 报告成功，但仍有资源未释放：" + strings.Join(parts, ", ")
	case len(unchecked) > 0:
		parts := make([]string, 0, len(unchecked))
		for _, v := range unchecked {
			parts = append(parts, v.Claim)
		}
		msg = "以下 claim 框架无法复查，由插件负责：" + strings.Join(parts, ", ")
	}

	phase := state.PhaseStopped
	if !clean {
		phase = state.PhaseDirty
	}
	if err := rt.Store.Update(ctx, func(st *state.File) error {
		for i := range st.Instances {
			if st.Instances[i].ID != inst.ID {
				continue
			}
			st.Instances[i].Live = !clean
			st.Instances[i].PID = 0
			if clean {
				st.Instances[i].Phase = state.PhaseStopped
			} else {
				st.Instances[i].Phase = state.PhaseDirty
				m := msg
				st.Instances[i].Error = &m
			}
			if st.Instances[i].RunPath == "" {
				st.Instances[i].RunPath = runPath(rt, st.Instances[i])
			}
		}
		return nil
	}); err != nil {
		return ExitUserError, err
	}

	if clean {
		writeRunRecord(rt, rt.reload(ctx, *inst), resultOnStop(*inst), nil, &released)
	} else {
		writeRunRecord(rt, rt.reload(ctx, *inst), runlog.ResultDirty, nil, &released)
	}

	out := StopOutput{
		Instance:       inst.ID,
		Live:           !clean,
		Released:       released,
		Phase:          string(phase),
		ClaimsReleased: &released,
		Verdicts:       verdicts,
	}
	if asJSON {
		code := ExitOK
		if !clean {
			code = ExitPluginErr
		}
		return code, printJSON(out)
	}

	if !clean {
		if err := rt.Store.Update(ctx, func(st *state.File) error {
			for i := range st.Instances {
				if st.Instances[i].ID == inst.ID {
					m := msg
					st.Instances[i].Error = &m
				}
			}
			return nil
		}); err != nil {
			return ExitUserError, err
		}
		fmt.Printf("%s 未清理干净：%s\n", inst.ID, msg)
		fmt.Printf("排查后可重试：chaoslab stop %s\n", inst.ID)
		return ExitPluginErr, fmt.Errorf("%s 未清理干净：%s", inst.ID, msg)
	}

	fmt.Printf("%s 已清理\n", inst.ID)
	if len(unchecked) > 0 {
		parts := make([]string, 0, len(unchecked))
		for _, v := range unchecked {
			parts = append(parts, v.Claim)
		}
		fmt.Printf("注意：以下 claim 框架未复查，由插件保证：%s\n", strings.Join(parts, ", "))
	}
	return ExitOK, nil
}

// RunStopAll stops every live instance. A failure on one does not skip the
// others, and any failure makes the command exit non-zero.
func RunStopAll(ctx context.Context, rt *Runtime) (int, error) {
	f, err := rt.Store.Read(ctx)
	if err != nil {
		return ExitUserError, err
	}
	Converge(f)
	var ids []string
	for _, in := range f.Instances {
		if in.Live {
			ids = append(ids, in.ID)
		}
	}
	if len(ids) == 0 {
		fmt.Println("没有正在运行的实例")
		return ExitOK, nil
	}
	worst := ExitOK
	var firstErr error
	for _, id := range ids {
		code, err := RunStop(ctx, rt, id, false)
		if code != ExitOK && worst == ExitOK {
			worst = code
			firstErr = err
		}
	}
	return worst, firstErr
}

// resultOnStop picks the result for a clean stop: a record that already exists
// keeps its outcome (passed stays passed), and an instance that never passed is
// recorded as abandoned so an evaluation harness has a denominator.
func resultOnStop(inst state.Instance) string {
	if inst.Phase == state.PhasePassed {
		return ""
	}
	return runlog.ResultAbandoned
}
