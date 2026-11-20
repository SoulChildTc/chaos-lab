package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Passing must tell the user so, and hand over the root cause and the clues.
// Silence after a real fix is the worst possible feedback: the user cannot tell
// a pass from a broken command.
func TestCheckHumanOutputShowsPassAndRootCause(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)

	out, err := captureStdout(t, func() (int, error) {
		return cli.RunCheck(context.Background(), rt, "fake/env", false, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "fixed") {
		t.Fatalf("must say it passed, got %q", s)
	}
	if !strings.Contains(strings.ToLower(s), "root cause") {
		t.Fatalf("must show the root cause, got %q", s)
	}
	if !strings.Contains(strings.ToLower(s), "clues") {
		t.Fatalf("must show the clues, got %q", s)
	}
}

// A not-yet-fixed check must still say something in human mode.
func TestCheckHumanOutputSaysNotFixed(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", map[string]string{"broken": "true"}, true)

	out, _ := captureStdout(t, func() (int, error) {
		return cli.RunCheck(context.Background(), rt, "fake/env", false, false)
	})
	if s := string(out); !strings.Contains(s, "not fixed") {
		t.Fatalf("must say it is not fixed yet, got %q", s)
	}
}

// A check that itself failed must say why, in human mode.
func TestCheckHumanOutputShowsPluginError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	overwritePlugin(t, rt, "fake", "#!/bin/sh\ncase \"$1\" in check) echo 'cluster unreachable' >&2; exit 2;; esac\nexit 1\n")

	out, err := captureStdout(t, func() (int, error) {
		return cli.RunCheck(context.Background(), rt, "fake/env", false, false)
	})
	if err == nil {
		t.Fatal("want an error")
	}
	_ = out
}
