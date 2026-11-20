package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Every subcommand answers --help, including the ones whose flags are dynamic.
func TestHelpWorksOnEverySubcommand(t *testing.T) {
	rt, _ := newTestRuntime(t)
	for _, args := range [][]string{
		{"--help"},
		{"list", "--help"},
		{"check", "--help"},
		{"stop", "--help"},
		{"rm", "--help"},
		{"status", "--help"},
		{"plugin", "--help"},
		{"plugin", "install", "--help"},
		{"version", "--help"},
	} {
		out, err := captureStdout(t, func() (int, error) {
			return cli.Run(context.Background(), rt, args)
		})
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		if !strings.Contains(string(out), "chaoslab") {
			t.Fatalf("%v: help text missing, got %q", args, out)
		}
	}
}

// `start <ref> --help` must show the plugin's own parameter table, not a
// generic page: those flags only exist at runtime.
func TestStartHelpShowsPluginFlags(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"start", "fake/env", "--help"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "--port") || !strings.Contains(s, "--fail-down") {
		t.Fatalf("plugin flags missing: %q", s)
	}
}

// An unknown flag is a mistake, not something to ignore silently.
func TestUnknownFlagIsRejected(t *testing.T) {
	rt, _ := newTestRuntime(t)
	for _, args := range [][]string{
		{"list", "--nope"},
		{"check", "fake/env", "--nope"},
		{"status", "--nope"},
		{"stop", "fake/env", "--nope"},
		{"--nope"},
	} {
		if code, err := cli.Run(context.Background(), rt, args); code == 0 || err == nil {
			t.Fatalf("%v must fail, code=%d err=%v", args, code, err)
		}
	}
}

// list accepts a plugin name and only shows that plugin's cases.
func TestListFiltersByPlugin(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)

	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"list", "fake2"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "fake2/only") {
		t.Fatalf("filtered case missing: %q", s)
	}
	if strings.Contains(s, "fake/env") {
		t.Fatalf("filter leaked other plugins: %q", s)
	}
}

func TestListUnknownPluginIsUserError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.Run(context.Background(), rt, []string{"list", "nosuch"})
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

// A bare invocation prints help but must not look like success.
func TestBareInvocationIsNotSuccess(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "chaoslab") {
		t.Fatalf("help text missing: %q", out)
	}
	code, _ := cli.Run(context.Background(), rt, nil)
	if code == 0 {
		t.Fatal("bare invocation must not exit 0")
	}
}

// Too many positional arguments is a mistake worth reporting.
func TestExtraArgsAreRejected(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.Run(context.Background(), rt, []string{"check", "a", "b"}); code == 0 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if code, err := cli.Run(context.Background(), rt, []string{"status", "a", "b"}); code == 0 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
