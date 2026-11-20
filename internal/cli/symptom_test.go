package cli_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Starting a plugin must hand the user two things: what is wrong, and how to get
// in. Without either, there is nothing to investigate.
func TestStartPrintsSymptomAndConnection(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"start", "fake"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "SYMPTOM") {
		t.Fatalf("no symptom section:\n%s", s)
	}
	if !strings.Contains(s, "CONNECT") {
		t.Fatalf("no connection section:\n%s", s)
	}
	if !strings.Contains(s, "fake-state") {
		t.Fatalf("connection details missing:\n%s", s)
	}
}

// The start output must not enumerate the injected faults: naming them is the
// answer to the exercise.
func TestStartDoesNotEnumerateFaults(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"start", "fake"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	for _, leaked := range []string{"示例场景一", "示例场景二"} {
		if strings.Contains(s, leaked) {
			t.Fatalf("start leaked a fault name (%q):\n%s", leaked, s)
		}
	}
}

func TestStartJSONCarriesSymptomAndConnect(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"start", "fake", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Instance string   `json:"instance"`
		Symptom  string   `json:"symptom"`
		Connect  []string `json:"connect"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	if got.Instance != "fake#1" || got.Symptom == "" || len(got.Connect) == 0 {
		t.Fatalf("%+v", got)
	}
}

// A scenario reference is not supported any more: a plugin injects all of its
// faults, so addressing one of them is a user mistake worth explaining.
func TestStartRejectsScenarioReference(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.Run(context.Background(), rt, []string{"start", "fake/env"})
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(err.Error(), "fake") || !strings.Contains(err.Error(), "all") {
		t.Fatalf("error should point at the plugin form: %v", err)
	}
}

func TestCheckRejectsScenarioReference(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	if code, err := cli.Run(context.Background(), rt, []string{"check", "fake/env"}); code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestStopRejectsScenarioReference(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	if code, err := cli.Run(context.Background(), rt, []string{"stop", "fake/env"}); code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

// Instance ids still work: they are how a user names a specific run.
func TestInstanceIDStillResolves(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	if code, err := cli.Run(context.Background(), rt, []string{"check", "fake#1"}); code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

// One plugin cannot have two live instances: the environment is shared, so a
// second start is a mistake, not a second environment.
func TestSecondStartOfSamePluginIsRefused(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)
	code, err := cli.Run(context.Background(), rt, []string{"start", "fake"})
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(err.Error(), "fake#1") {
		t.Fatalf("error must name the running instance: %v", err)
	}
}
