package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

func TestStatusJSONListsOnlyLive(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)
	if code, _ := cli.RunStart(context.Background(), rt, "fake", nil, true); code != 0 {
		t.Fatal("start fake")
	}
	if code, _ := cli.RunStart(context.Background(), rt, "fake2", nil, true); code != 0 {
		t.Fatal("start fake2")
	}
	if code, _ := cli.RunStop(context.Background(), rt, "fake", false); code != 0 {
		t.Fatal("stop fake")
	}

	out, err := captureStdout(t, func() (int, error) { return cli.RunStatus(context.Background(), rt, "", true) })
	if err != nil {
		t.Fatal(err)
	}
	var got []state.Instance
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out, err)
	}
	if len(got) != 1 || got[0].Plugin != "fake2" {
		t.Fatalf("%+v", got)
	}
}

func TestStatusJSONIncludesDirty(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", map[string]string{"fail-down": "true"}, true)
	cli.RunStop(context.Background(), rt, "fake", false)

	out, _ := captureStdout(t, func() (int, error) { return cli.RunStatus(context.Background(), rt, "", true) })
	var got []state.Instance
	json.Unmarshal(out, &got)
	if len(got) != 1 || got[0].Phase != state.PhaseDirty || !got[0].Live {
		t.Fatalf("%+v", got)
	}
}

func TestStatusJSONEmptyIsArray(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) { return cli.RunStatus(context.Background(), rt, "", true) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("empty status must be [], got %q", out)
	}
}

func TestStatusFilterByInstanceID(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	cli.RunStart(context.Background(), rt, "fake", nil, true)

	out, _ := captureStdout(t, func() (int, error) {
		return cli.RunStatus(context.Background(), rt, "fake#1", true)
	})
	var got []state.Instance
	json.Unmarshal(out, &got)
	if len(got) != 1 || got[0].ID != "fake#1" {
		t.Fatalf("%+v", got)
	}
}

func TestStatusUnknownRefIsUserError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStatus(context.Background(), rt, "fake/nope", true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestStatusHumanOutputMentionsIDs(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	out, err := captureStdout(t, func() (int, error) { return cli.RunStatus(context.Background(), rt, "", false) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "fake#1") || !strings.Contains(string(out), "running") {
		t.Fatalf("human output=%q", out)
	}
}
