package cli_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulchild/chaos-lab/internal/cli"
	"github.com/soulchild/chaos-lab/internal/runlog"
	"github.com/soulchild/chaos-lab/internal/state"
)

func TestStopReleasesAndMarksNotLive(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true); code != 0 {
		t.Fatalf("start: %d %v", code, err)
	}
	code, err := cli.RunStop(context.Background(), rt, "fake/env", true)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	in := f.Instances[0]
	if in.Live || in.Phase != state.PhaseStopped {
		t.Fatalf("%+v", in)
	}
}

func TestStopWithFailDownMarksDirty(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"fail-down": "true"}, true)
	code, _ := cli.RunStop(context.Background(), rt, "fake/env", true)
	if code != 3 {
		t.Fatalf("want 3, got %d", code)
	}
	f, _ := rt.Store.Read(context.Background())
	in := f.Instances[0]
	if !in.Live || in.Phase != state.PhaseDirty || in.Error == nil {
		t.Fatalf("%+v", in)
	}
}

func TestStopWithResidualClaimsMarksDirty(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"leak-dir": "true"}, true)
	code, _ := cli.RunStop(context.Background(), rt, "fake/env", true)
	if code != 3 {
		t.Fatalf("a residual must be a plugin-level failure, got %d", code)
	}
	f, _ := rt.Store.Read(context.Background())
	in := f.Instances[0]
	if !in.Live || in.Phase != state.PhaseDirty {
		t.Fatalf("residual dir must keep live: %+v", in)
	}
}

func TestStopReportsWhichClaimLeaked(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"leak-dir": "true"}, true)
	_, err := cli.RunStop(context.Background(), rt, "fake/env", false)
	if err == nil || !strings.Contains(err.Error(), "dir:") {
		t.Fatalf("error must name the leaked claim, got %v", err)
	}
}

func TestStopUncheckedClaimIsReportedNotFatal(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	// A claim the framework cannot verify must not block the stop, but must be
	// surfaced, otherwise "clean" is a guess.
	rt.Store.Update(context.Background(), func(f *state.File) error {
		f.Instances[0].Claims = []string{"remote:jenkins"}
		return nil
	})
	code, err := cli.RunStop(context.Background(), rt, "fake/env", true)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Live {
		t.Fatalf("%+v", f.Instances[0])
	}
}

func TestStopThenStartSameCaseSucceeds(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)
	code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	last := f.Instances[len(f.Instances)-1]
	if last.ID != "fake/env#2" {
		t.Fatalf("seq must not be reused: %+v", last)
	}
	cli.RunStop(context.Background(), rt, "fake/env", true)
}

func TestStopWritesAbandonedRecordWhenNeverPassed(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)
	recs := runRecords(t, home)
	if len(recs) != 1 {
		t.Fatalf("%+v", recs)
	}
	if recs[0].Result != runlog.ResultAbandoned || recs[0].ClaimsReleased == nil || !*recs[0].ClaimsReleased {
		t.Fatalf("%+v", recs[0])
	}
}

func TestStopAfterPassKeepsPassedResult(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunCheck(context.Background(), rt, "fake/env", false, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)
	recs := runRecords(t, home)
	if len(recs) != 1 {
		t.Fatalf("stop must update the pass record, not add one: %+v", recs)
	}
	if recs[0].Result != runlog.ResultPassed || recs[0].ClaimsReleased == nil || !*recs[0].ClaimsReleased {
		t.Fatalf("%+v", recs[0])
	}
	if len(recs[0].Report) == 0 {
		t.Fatal("stop must not drop the root cause report")
	}
}

func TestStopDirtyRunRecordSurvives(t *testing.T) {
	rt, home := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"fail-down": "true"}, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)
	recs := runRecords(t, home)
	if len(recs) != 1 || recs[0].Result != runlog.ResultDirty {
		t.Fatalf("%+v", recs)
	}
	if recs[0].ClaimsReleased == nil || *recs[0].ClaimsReleased {
		t.Fatalf("dirty must record claims not released: %+v", recs[0])
	}
}

func TestStopDirtyBlocksRestart(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"fail-down": "true"}, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)
	code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "fake/env#1") {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestStopRetryAfterDirtyCanSucceed(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"fail-down": "true"}, true)
	cli.RunStop(context.Background(), rt, "fake/env", true)

	// The user fixes the cause (here: the plugin stops failing and actually
	// removes the leftover directory) and retries.
	overwritePlugin(t, rt, "fake", "#!/bin/sh\nrm -rf \"$CHAOSLAB_HOME\"/fake-state/*\nexit 0\n")
	code, err := cli.RunStop(context.Background(), rt, "fake/env", true)
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Live || f.Instances[0].Phase == state.PhaseDirty {
		t.Fatalf("%+v", f.Instances[0])
	}
}

func TestStopJSONNamesReleasedClaims(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStop(context.Background(), rt, "fake/env", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, `"released":true`) || !strings.Contains(s, `"live":false`) {
		t.Fatalf("stop --json must report the verdict: %s", s)
	}
}

func TestStopAmbiguousRefRequiresInstanceID(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, _ := cli.RunStart(context.Background(), rt, "fake/env", nil, true); code != 0 {
		t.Fatal("start")
	}
	// Forge a second live instance of the same case to exercise the ambiguity.
	if err := rt.Store.Update(context.Background(), func(f *state.File) error {
		dup := f.Instances[0]
		dup.ID = "fake/env#2"
		dup.Slug = "fake-env-2"
		// Its own claim: a real second instance would own a different directory.
		dup.Claims = []string{"dir:" + filepath.Join(rt.Home, "fake-state", "fake-env-2")}
		f.Instances = append(f.Instances, dup)
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, err := cli.RunStop(context.Background(), rt, "fake/env", true)
	if code != 1 || err == nil || !strings.Contains(err.Error(), "fake/env#1") || !strings.Contains(err.Error(), "fake/env#2") {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if code2, err2 := cli.RunStop(context.Background(), rt, "fake/env#2", true); code2 != 0 || err2 != nil {
		t.Fatalf("explicit id must work: %d %v", code2, err2)
	}
}

func TestStopUnknownRefIsUserError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStop(context.Background(), rt, "fake/env", true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestStopLeavesOtherInstancesAlone(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunStart(context.Background(), rt, "fake/other", nil, true)
	if code, _ := cli.RunStop(context.Background(), rt, "fake/env", true); code != 0 {
		t.Fatal("stop failed")
	}
	f, _ := rt.Store.Read(context.Background())
	live := 0
	for _, in := range f.Instances {
		if in.Live {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("want exactly one remaining live instance, got %d", live)
	}
	_ = os.RemoveAll(filepath.Join(rt.Home, "fake-state"))
}
