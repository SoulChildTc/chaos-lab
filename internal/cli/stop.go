package cli

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/soulchildtc/chaos-lab/internal/claims"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/runlog"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// StopOutput is the JSON shape of `stop --json`: what the plugin claimed to
// release, what the framework measured afterwards, and why it failed.
type StopOutput struct {
	Instance   string           `json:"instance"`
	Live       bool             `json:"live"`
	Released   bool             `json:"released"`
	Phase      string           `json:"phase"`
	Message    string           `json:"message,omitempty"`
	Verdicts   []claims.Verdict `json:"verdicts"`
	Unverified []string         `json:"unverified,omitempty"`
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

	pluginDir, err := rt.pluginDir(inst.Plugin)
	if err != nil {
		return ExitUserError, err
	}
	lister := &plugin.Lister{PluginDir: pluginDir}
	timeout := parseTimeout(caseTimeout(ctx, lister, inst.Args), defaultUpTimeout)

	downCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	_, code, runErr := pluginCall(downCtx, pluginDir, *inst, "down")

	var pluginMsg string
	if runErr != nil {
		if errors.Is(runErr, plugin.ErrTimeout) {
			pluginMsg = fmt.Sprintf("down exceeded %s and was killed", timeout)
		} else {
			pluginMsg = runErr.Error()
		}
	} else if code != 0 {
		pluginMsg = fmt.Sprintf("down exited %d", code)
	}

	// Measure even when down failed: the user needs to know what is still there,
	// not just that the plugin errored.
	verdicts := claims.Verify(ctx, inst.Claims)
	residuals := claims.Residuals(verdicts)
	unchecked := claims.Unchecked(verdicts)
	clean := pluginMsg == "" && len(residuals) == 0

	// claims_released is only true when every claim was actually judged and found
	// gone. An unverifiable claim leaves it false: "clean" would be a guess.
	var released *bool
	allJudged := len(unchecked) == 0
	if clean && allJudged {
		t := true
		released = &t
	} else {
		f := false
		released = &f
	}

	var msg string
	switch {
	case pluginMsg != "":
		msg = pluginMsg
	case len(residuals) > 0:
		parts := make([]string, 0, len(residuals))
		for _, v := range residuals {
			parts = append(parts, fmt.Sprintf("%s（%s）", v.Claim, v.Detail))
		}
		msg = "down reported success, but these are still held: " + strings.Join(parts, ", ")
	case len(unchecked) > 0:
		parts := make([]string, 0, len(unchecked))
		for _, v := range unchecked {
			parts = append(parts, v.Claim)
		}
		msg = "the framework cannot verify these claims; the plugin vouches for them: " + strings.Join(parts, ", ")
	}

	unverified := make([]string, 0, len(unchecked))
	for _, v := range unchecked {
		unverified = append(unverified, v.Claim)
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
				if len(unverified) > 0 {
					m := msg
					st.Instances[i].Error = &m
				}
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
		return ExitPluginErr, fmt.Errorf("%s: recording the cleanup result failed: %w", inst.ID, err)
	}

	reloaded := rt.reload(ctx, *inst)
	if clean {
		writeRunRecord(rt, reloaded, resultOnStop(rt, reloaded), nil, released, unverified)
	} else {
		writeRunRecord(rt, reloaded, runlog.ResultDirty, nil, released, unverified)
	}

	out := StopOutput{
		Instance:   inst.ID,
		Live:       !clean,
		Released:   clean && allJudged,
		Phase:      string(phase),
		Message:    msg,
		Verdicts:   verdicts,
		Unverified: unverified,
	}

	if !clean {
		if asJSON {
			return ExitPluginErr, printJSON(out)
		}
		fmt.Printf("%s %s %s\n", glyphFail(), bad("not clean"), dim(inst.ID))
		for _, line := range wrap(msg, 72) {
			fmt.Println("  " + line)
		}
		fmt.Println(dim("  retry after investigating: chaoslab rm " + inst.ID))
		return ExitPluginErr, fmt.Errorf("%s is not clean: %s", inst.ID, msg)
	}

	if asJSON {
		return ExitOK, printJSON(out)
	}
	fmt.Printf("%s %s %s\n", glyphOK(), ok("clean"), dim(inst.ID))
	if len(unverified) > 0 {
		fmt.Println(warn("  note: the framework did not verify these claims; the plugin vouches for them: ") + strings.Join(unverified, ", "))
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
		fmt.Println("No live instances.")
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

// resultOnStop picks the result for a clean stop. A record that already has a
// terminal outcome keeps it: a failed start stays start_failed and a watch
// timeout stays timeout. Only an instance that never reached a verdict becomes
// abandoned, so an evaluation harness still gets a denominator.
func resultOnStop(rt *Runtime, inst state.Instance) string {
	if inst.Phase == state.PhasePassed {
		return ""
	}
	if inst.RunPath != "" {
		switch runlog.ExistingResult(inst.RunPath) {
		case runlog.ResultStartFailed, runlog.ResultTimeout, runlog.ResultDirty, runlog.ResultPassed:
			return ""
		}
	}
	return runlog.ResultAbandoned
}
