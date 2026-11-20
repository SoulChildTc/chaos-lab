package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/runlog"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// A stop whose claims cannot be judged must not claim they were released: that
// would make "no residue" a guess. It must say so, in both modes.
func TestStopUnverifiableClaimsAreNotReportedReleased(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	if err := rt.Store.Update(context.Background(), func(f *state.File) error {
		f.Instances[0].Claims = []string{"remote:jenkins"}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStop(context.Background(), rt, "fake", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), `"claims_released":true`) {
		t.Fatalf("unverifiable claims must not be reported released: %s", out)
	}
	if !strings.Contains(string(out), "remote:jenkins") {
		t.Fatalf("output must name the unverified claim: %s", out)
	}

	recs := runRecords(t, home)
	if len(recs) != 1 {
		t.Fatalf("%+v", recs)
	}
	if recs[0].ClaimsReleased != nil && *recs[0].ClaimsReleased {
		t.Fatalf("run record must not claim released: %+v", recs[0])
	}
	if len(recs[0].Unverified) != 1 || recs[0].Unverified[0] != "remote:jenkins" {
		t.Fatalf("run record must carry the unverified claims: %+v", recs[0])
	}
	_ = runlog.ResultDirty
}

// A plugin that failed to tear down must say why, even in JSON mode.
func TestStopJSONCarriesTheFailureMessage(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"fail-down": "true"}, true)
	out, _ := captureStdout(t, func() (int, error) {
		return cli.RunStop(context.Background(), rt, "fake", true)
	})
	if !strings.Contains(string(out), `"message"`) || !strings.Contains(string(out), "down") {
		t.Fatalf("JSON stop must explain the failure: %s", out)
	}
}

// stop must not erase the outcome of a failed start or a watch timeout.
func TestStopKeepsStartFailedResult(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"fail-up": "true"}, true)
	cli.RunStop(context.Background(), rt, "fake", false)
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultStartFailed {
		t.Fatalf("start_failed must survive stop: %+v", recs)
	}
}

func TestStopKeepsTimeoutResult(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"broken": "true", "check-timeout": "1s"}, true)
	cli.RunCheck(context.Background(), rt, "fake", true, true)
	cli.RunStop(context.Background(), rt, "fake", false)
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultTimeout {
		t.Fatalf("timeout must survive stop: %+v", recs)
	}
}

// A watch timeout must record the attempts it actually made.
func TestWatchTimeoutRecordsCheckCount(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"broken": "true", "check-timeout": "1s"}, true)
	cli.RunCheck(context.Background(), rt, "fake", true, true)
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Checks < 1 {
		t.Fatalf("checks must be recorded: %+v", recs)
	}
}

// An orphaned starting record must not block uninstall.
func TestUninstallConvergesOrphanRecord(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if err := rt.Store.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{
			ID: "fake/env#9", Plugin: "fake", Slug: "fake-9",
			Phase: state.PhaseStarting, Live: true, PID: 999999,
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, err := cli.RunPlugin(context.Background(), rt, "uninstall", []string{"fake"}, false)
	if code != 1 || err == nil {
		t.Fatalf("an orphan still owns resources, so uninstall must refuse: code=%d err=%v", code, err)
	}
	if !strings.Contains(err.Error(), "fake/env#9") {
		t.Fatalf("must name the orphan: %v", err)
	}
}
