package cli_test

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// newTestRuntime installs plugins/fake into a throwaway CHAOSLAB_HOME and
// returns a Runtime wired to it.
func newTestRuntime(t *testing.T) (*cli.Runtime, string) {
	t.Helper()
	h := t.TempDir()
	t.Setenv("CHAOSLAB_HOME", h)
	if err := installFake(t, h); err != nil {
		t.Fatalf("install fake: %v", err)
	}
	return cli.NewRuntime(h), h
}

func installFake(t *testing.T, home string) error {
	t.Helper()
	src, err := filepath.Abs(filepath.Join("..", "..", "plugins", "fake"))
	if err != nil {
		return err
	}
	_, err = plugin.Install(context.Background(), src, filepath.Join(home, "plugins"))
	return err
}

func TestSlugIsLowercaseAndHyphenated(t *testing.T) {
	if got := cli.Slug("k8s", 1); got != "k8s-1" {
		t.Fatalf("got=%q", got)
	}
	if got := cli.Slug("My_Plugin", 7); got != "my-plugin-7" {
		t.Fatalf("got=%q", got)
	}
}

func TestStartCreatesRunningInstance(t *testing.T) {
	rt, home := newTestRuntime(t)
	code, err := cli.RunStart(context.Background(), rt, "fake", nil, true)
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if len(f.Instances) != 1 {
		t.Fatalf("instances=%+v", f.Instances)
	}
	in := f.Instances[0]
	if in.Phase != state.PhaseRunning || !in.Live || in.ID != "fake#1" || in.Slug != "fake-1" {
		t.Fatalf("%+v", in)
	}
	if len(in.Claims) != 1 || in.Claims[0][:4] != "dir:" {
		t.Fatalf("claims=%v", in.Claims)
	}
	if _, err := os.Stat(filepath.Join(home, "fake-state", "fake-1")); err != nil {
		t.Fatalf("plugin up did not create its env: %v", err)
	}
}

func TestStartJSONNamesTheInstance(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStart(context.Background(), rt, "fake", nil, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Instance string `json:"instance"`
		Slug     string `json:"slug"`
		Phase    string `json:"phase"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout is not JSON: %q (%v)", out, err)
	}
	if got.Instance != "fake#1" || got.Slug != "fake-1" || got.Phase != "running" {
		t.Fatalf("%+v", got)
	}
}

func TestStartRejectsSecondInstanceOfSameCase(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.RunStart(context.Background(), rt, "fake", nil, true); code != 0 {
		t.Fatalf("first start: code=%d err=%v", code, err)
	}
	code, err := cli.RunStart(context.Background(), rt, "fake", nil, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("fake#1")) {
		t.Fatalf("error must name the running instance: %v", err)
	}
}

func TestStartRejectsClaimConflict(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)
	if code, err := cli.RunStart(context.Background(), rt, "fake", map[string]string{"port": "18080"}, true); code != 0 {
		t.Fatalf("first start: code=%d err=%v", code, err)
	}
	// A second plugin claiming the same port must be refused, naming both sides.
	overwritePlugin(t, rt, "fake2", `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake2","args":[]}' ;;
list)   echo '{"scenarios":[],"claims":["port:18080"]}' ;;
esac
`)
	code, err := cli.RunStart(context.Background(), rt, "fake2", nil, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("port:18080")) {
		t.Fatalf("error must name the claim: %v", err)
	}
	if !bytes.Contains([]byte(err.Error()), []byte("fake#1")) {
		t.Fatalf("error must name the conflicting instance: %v", err)
	}
}

func TestStartConflictLeavesNoRecord(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"port": "18081"}, true)
	overwritePlugin(t, rt, "fake2", `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake2","args":[]}' ;;
list)   echo '{"scenarios":[],"claims":["port:18081"]}' ;;
esac
`)
	if code, _ := cli.RunStart(context.Background(), rt, "fake2", nil, true); code != 1 {
		t.Fatalf("want conflict, got code=%d", code)
	}
	f, _ := rt.Store.Read(context.Background())
	if len(f.Instances) != 1 {
		t.Fatalf("conflicting start left a record: %+v", f.Instances)
	}
}

func TestStartFailureKeepsLiveRecord(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStart(context.Background(), rt, "fake", map[string]string{"fail-up": "true"}, true)
	if code != 3 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if len(f.Instances) != 1 {
		t.Fatalf("%+v", f.Instances)
	}
	in := f.Instances[0]
	if !in.Live || in.Phase != state.PhaseStopped || in.Error == nil {
		t.Fatalf("%+v", in)
	}
}

func TestStartRejectsUnknownArg(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStart(context.Background(), rt, "fake", map[string]string{"nope": "1"}, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if len(f.Instances) != 0 {
		t.Fatalf("invalid args must not create a record: %+v", f.Instances)
	}
}

func TestStartRejectsUnknownCaseOrPlugin(t *testing.T) {
	rt, _ := newTestRuntime(t)
	for _, ref := range []string{"fake/nosuch", "nosuch/env", "justoneword"} {
		if code, err := cli.RunStart(context.Background(), rt, ref, nil, true); code != 1 || err == nil {
			t.Fatalf("ref=%q code=%d err=%v", ref, code, err)
		}
	}
}

func TestStartWritesRunningPhaseOnlyAfterUpSucceeds(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Phase != state.PhaseRunning {
		t.Fatalf("%+v", f.Instances[0])
	}
	if f.Seq["fake"] != 1 {
		t.Fatalf("seq=%v", f.Seq)
	}
}
