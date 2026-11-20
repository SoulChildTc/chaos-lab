package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/soulchild/chaos-lab/internal/cli"
	"github.com/soulchild/chaos-lab/internal/runlog"
	"github.com/soulchild/chaos-lab/internal/state"
)

// overwritePlugin replaces an installed plugin's entry so a test can simulate a
// plugin that hangs, lies, or returns garbage.
func overwritePlugin(t *testing.T, rt *cli.Runtime, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(rt.PluginsDir, name, "plugin"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
}

func runRecords(t *testing.T, home string) []runlog.Record {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(home, "runs", "*.json"))
	if err != nil {
		t.Fatal(err)
	}
	out := make([]runlog.Record, 0, len(files))
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		var r runlog.Record
		if err := json.Unmarshal(b, &r); err != nil {
			t.Fatalf("%s: %v", p, err)
		}
		out = append(out, r)
	}
	return out
}

func TestCheckPassWritesRunRecord(t *testing.T) {
	rt, home := newTestRuntime(t)
	if code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true); code != 0 {
		t.Fatalf("start: %d %v", code, err)
	}
	code, err := cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultPassed || len(recs[0].Report) == 0 {
		t.Fatalf("%+v", recs)
	}
	if recs[0].Instance != "fake/env#1" || recs[0].Checks != 1 {
		t.Fatalf("%+v", recs[0])
	}
}

func TestCheckPassMarksInstancePassedButLive(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	f, _ := rt.Store.Read(context.Background())
	in := f.Instances[0]
	if in.Phase != state.PhasePassed || !in.Live {
		t.Fatalf("a passed instance still owns its environment: %+v", in)
	}
}

func TestCheckNotFixedReturnsTwo(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"broken": "true"}, true)
	code, err := cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if code != 2 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Phase != state.PhaseRunning {
		t.Fatalf("%+v", f.Instances[0])
	}
	if f.Instances[0].Checks != 1 {
		t.Fatalf("a not-fixed attempt must be counted: %+v", f.Instances[0])
	}
}

func TestCheckNotFixedWritesNoRecord(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"broken": "true"}, true)
	cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if recs := runRecords(t, home); len(recs) != 0 {
		t.Fatalf("a failed attempt is not a final result: %+v", recs)
	}
}

func TestCheckProtocolErrorReturnsThree(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	overwritePlugin(t, rt, "fake", "#!/bin/sh\ncase \"$1\" in check) exit 0;; esac\nexit 1\n")
	code, _ := cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if code != 3 {
		t.Fatalf("want 3 for exit 0 with no report, got %d", code)
	}
}

func TestCheckBrokenExitCodeTwoIsPluginError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	overwritePlugin(t, rt, "fake", "#!/bin/sh\ncase \"$1\" in check) echo 'cluster unreachable' >&2; exit 2;; esac\nexit 1\n")
	code, _ := cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Checks != 0 {
		t.Fatalf("a broken check must not count as an attempt: %+v", f.Instances[0])
	}
}

func TestCheckHangIsKilled(t *testing.T) {
	if testing.Short() {
		t.Skip("waits out the per-call timeout")
	}
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	overwritePlugin(t, rt, "fake", "#!/bin/sh\ncase \"$1\" in check) sleep 600;; esac\nexit 1\n")

	// Shrink the per-call limit so the test does not wait two minutes. The
	// production default is perCheckTimeout.
	cli.SetCheckTimeoutForTest(2 * time.Second)
	t.Cleanup(func() { cli.SetCheckTimeoutForTest(0) })

	start := time.Now()
	code, err := cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if code != 3 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if elapsed := time.Since(start); elapsed > 30*time.Second {
		t.Fatalf("check was not bounded: %v", elapsed)
	}
}

func TestCheckJSONReportMatchesExitCode(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Passed bool `json:"passed"`
		Report struct {
			RootCause string   `json:"root_cause"`
			Clues     []string `json:"clues"`
		} `json:"report"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out, err)
	}
	if !got.Passed || got.Report.RootCause == "" {
		t.Fatalf("%+v", got)
	}
}

func TestCheckSecondPassUpdatesSameRecord(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	if recs := runRecords(t, home); len(recs) != 1 {
		t.Fatalf("want one record for one instance, got %d", len(recs))
	}
}

func TestCheckWatchTimesOutAndRecordsTimeout(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"broken": "true", "check-timeout": "2s"}, true)
	code, _ := cli.RunCheck(context.Background(), rt, "fake/env", true, true)
	if code != 2 {
		t.Fatalf("want 2, got %d", code)
	}
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultTimeout {
		t.Fatalf("%+v", recs)
	}
}

func TestCheckWatchStopsAsSoonAsItPasses(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"check-timeout": "60s"}, true)
	code, err := cli.RunCheck(context.Background(), rt, "fake/env", true, true)
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultPassed {
		t.Fatalf("%+v", recs)
	}
}

func TestCheckUnknownRefIsUserError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunCheck(context.Background(), rt, "fake/nothing", false, true)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "fake/nothing") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestCheckDoesNotPassWhenWatchCancelled(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"broken": "true", "check-timeout": "60s"}, true)
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	code, _ := cli.RunCheck(ctx, rt, "fake/env", true, true)
	if code == 0 {
		t.Fatal("cancelled watch must not report success")
	}
}
