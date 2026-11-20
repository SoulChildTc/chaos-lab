package cli_test

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/soulchild/chaos-lab/internal/cli"
	"github.com/soulchild/chaos-lab/internal/state"
)

func TestStartRecoversOrphanedStartingRecord(t *testing.T) {
	rt, _ := newTestRuntime(t)
	dead := 999999
	if err := rt.Store.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{
			ID: "fake/env#9", Plugin: "fake", Case: "env", Slug: "fake-env-9",
			Phase: state.PhaseStarting, Live: true, PID: dead,
			Claims: []string{"dir:/tmp/whatever"},
		})
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, err := cli.RunStatus(context.Background(), rt, "", true)
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	in := f.Instances[0]
	if in.Phase != state.PhaseStopped || !in.Live || in.Error == nil {
		t.Fatalf("orphan not converged: %+v", in)
	}
}

func TestStartIsBlockedByOrphanOfSameCase(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if err := rt.Store.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{
			ID: "fake/env#9", Plugin: "fake", Case: "env", Slug: "fake-env-9",
			Phase: state.PhaseStarting, Live: true, PID: 999999,
		})
		f.Seq["fake/env"] = 9
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if _, err := filepath.Abs(rt.Home); err != nil {
		t.Fatal(err)
	}
}
