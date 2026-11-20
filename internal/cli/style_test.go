package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Colour escapes must never reach a pipe, a file, or a script.
func TestNoColorWhenNotATerminal(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)

	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"status"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(out), "\x1b[") {
		t.Fatalf("ANSI escapes leaked into non-terminal output: %q", out)
	}
}

func TestNoColorHonoursNO_COLOR(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	t.Setenv("FORCE_COLOR", "1")
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)

	out, _ := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"status"})
	})
	if strings.Contains(string(out), "\x1b[") {
		t.Fatalf("NO_COLOR ignored: %q", out)
	}
}

// Passed, failed and pending states must be visually distinct in text, not only
// by colour: a colour-blind user or a plain pipe still needs to tell them apart.
func TestStatesUseDistinctGlyphs(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)

	pass, _ := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"check", "fake"})
	})
	if !strings.Contains(string(pass), "✓") {
		t.Fatalf("pass has no glyph: %q", pass)
	}

	installSecondPlugin(t, rt)
	cli.RunStart(context.Background(), rt, "fake2", nil, true)
	overwritePlugin(t, rt, "fake2", "#!/bin/sh\ncase \"$1\" in\ncheck) echo '{\"passed\":false}' >&2; exit 1 ;;\nesac\n")
	pending, _ := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"check", "fake2"})
	})
	if !strings.Contains(string(pending), "·") {
		t.Fatalf("pending has no glyph: %q", pending)
	}
	if strings.Contains(string(pending), "✓") {
		t.Fatalf("pending must not reuse the pass glyph: %q", pending)
	}
}
