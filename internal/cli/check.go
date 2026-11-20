package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/runlog"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

const (
	// perCheckTimeout bounds one plugin check call. A check that hangs must not
	// hang the CLI; it is a plugin bug and is reported as one.
	perCheckTimeout = 2 * time.Minute
	// watchInterval is the pause between checks when --watch is used.
	watchInterval = 3 * time.Second
	// defaultCheckWindow is how long --watch keeps trying before giving up.
	defaultCheckWindow = 10 * time.Minute
)

// checkTimeoutForTest lets a test shrink the per-call bound instead of waiting
// two minutes. Zero means the production value.
var checkTimeoutForTest time.Duration

// SetCheckTimeoutForTest overrides the per-call timeout. Tests only; zero
// restores the default.
func SetCheckTimeoutForTest(d time.Duration) { checkTimeoutForTest = d }

func perCallTimeout() time.Duration {
	if checkTimeoutForTest > 0 {
		return checkTimeoutForTest
	}
	return perCheckTimeout
}

// CheckOutput is the JSON shape of `check --json`. It mirrors the plugin's
// report so a harness never has to parse human text.
type CheckOutput struct {
	Passed  bool            `json:"passed"`
	Message string          `json:"message,omitempty"`
	Report  json.RawMessage `json:"report,omitempty"`
}

// RunCheck asks the plugin whether the environment is fixed. Exit codes: 0
// passed, 2 not yet fixed, 3 the check itself failed.
func RunCheck(ctx context.Context, rt *Runtime, ref string, watch bool, asJSON bool) (int, error) {
	f, err := rt.Store.Read(ctx)
	if err != nil {
		return ExitUserError, err
	}
	Converge(f)
	inst, err := Resolve(f.Instances, ref)
	if err != nil {
		return ExitUserError, err
	}
	if !inst.Live {
		return ExitUserError, fmt.Errorf("%w: %s is already stopped", plugin.ErrUserInput, inst.ID)
	}

	window := defaultCheckWindow
	if cases, err := rt.Lister(inst.Plugin).List(ctx, inst.Slug, inst.Args); err == nil {
		if c, ok := findCase(cases, inst.Case); ok {
			window = parseTimeout(c.CheckTimeout, defaultCheckWindow)
		}
	}

	deadline := time.Now().Add(window)
	for {
		out, code, err := runCheckOnce(ctx, rt, *inst)
		switch code {
		case ExitOK:
			if err := rt.markPassed(ctx, *inst); err != nil {
				return ExitUserError, err
			}
			writeRunRecord(rt, rt.reload(ctx, *inst), runlog.ResultPassed, out.Report, nil, nil)
			if asJSON {
				return ExitOK, printJSON(out)
			}
			printPass(inst.ID, out.Report)
			return ExitOK, nil
		case ExitNotPassed:
			if !watch {
				if asJSON {
					return ExitNotPassed, printJSON(out)
				}
				printNotFixed(inst.ID, out.Message)
				return ExitNotPassed, nil
			}
			fmt.Fprintf(os.Stderr, "%s retrying in %s\n", dim(inst.ID), dim(watchInterval.String()))
		default:
			// The check itself is broken: not an attempt, no record.
			return ExitPluginErr, err
		}

		if !watch {
			return ExitNotPassed, nil
		}
		if time.Now().After(deadline) {
			writeRunRecord(rt, rt.reload(ctx, *inst), runlog.ResultTimeout, nil, nil, nil)
			return ExitNotPassed, fmt.Errorf("%s is still not fixed after %s", inst.ID, window)
		}
		select {
		case <-ctx.Done():
			return ExitNotPassed, ctx.Err()
		case <-time.After(watchInterval):
		}
	}
}

func (rt *Runtime) markPassed(ctx context.Context, inst state.Instance) error {
	return rt.Store.Update(ctx, func(f *state.File) error {
		for i := range f.Instances {
			if f.Instances[i].ID != inst.ID {
				continue
			}
			f.Instances[i].Phase = state.PhasePassed
			f.Instances[i].Checks++
			now := time.Now().UTC()
			f.Instances[i].LastCheck = &now
			if f.Instances[i].RunPath == "" {
				f.Instances[i].RunPath = runPath(rt, f.Instances[i])
			}
			return nil
		}
		return nil
	})
}

func (rt *Runtime) markAttempt(ctx context.Context, inst state.Instance) {
	_ = rt.Store.Update(ctx, func(f *state.File) error {
		for i := range f.Instances {
			if f.Instances[i].ID != inst.ID {
				continue
			}
			f.Instances[i].Checks++
			now := time.Now().UTC()
			f.Instances[i].LastCheck = &now
		}
		return nil
	})
}

// runCheckOnce performs exactly one plugin check call.
func runCheckOnce(ctx context.Context, rt *Runtime, inst state.Instance) (CheckOutput, int, error) {
	pluginDir := rt.Lister(inst.Plugin).PluginDir
	checkCtx, cancel := context.WithTimeout(ctx, perCallTimeout())
	defer cancel()

	stdout, code, err := pluginCall(checkCtx, pluginDir, inst, "check")
	if err != nil {
		if errors.Is(err, plugin.ErrTimeout) {
			return CheckOutput{Message: "check timed out"}, ExitPluginErr,
				fmt.Errorf("%s: check did not return within %s, killed", inst.ID, perCallTimeout())
		}
		return CheckOutput{Message: err.Error()}, ExitPluginErr, err
	}

	switch code {
	case 0:
		var report struct {
			Passed    bool            `json:"passed"`
			RootCause string          `json:"root_cause"`
			Clues     []string        `json:"clues"`
			Message   string          `json:"message"`
			Raw       json.RawMessage `json:"-"`
		}
		if err := json.Unmarshal(stdout, &report); err != nil {
			return CheckOutput{Message: "check output is not valid JSON"}, ExitPluginErr,
				fmt.Errorf("%w: %s: check exited 0 but stdout is not JSON: %q", plugin.ErrProtocol, inst.ID, truncate(stdout))
		}
		if !report.Passed {
			return CheckOutput{Passed: false, Message: "check exited 0 but reported passed=false"}, ExitPluginErr,
				fmt.Errorf("%w: %s: check exited 0 but reported passed=false", plugin.ErrProtocol, inst.ID)
		}
		return CheckOutput{Passed: true, Report: json.RawMessage(stdout)}, ExitOK, nil
	case 1:
		rt.markAttempt(ctx, inst)
		return CheckOutput{Passed: false, Message: "not fixed yet"}, ExitNotPassed, nil
	case 2:
		return CheckOutput{Message: "the check itself failed"}, ExitPluginErr,
			fmt.Errorf("%s: check failed (exit 2), the environment may be unreachable", inst.ID)
	default:
		return CheckOutput{Message: fmt.Sprintf("check exited %d", code)}, ExitPluginErr,
			fmt.Errorf("%w: %s: check exited %d", plugin.ErrProtocol, inst.ID, code)
	}
}

func truncate(b []byte) string {
	const max = 200
	if len(b) <= max {
		return string(b)
	}
	return string(b[:max]) + "..."
}
