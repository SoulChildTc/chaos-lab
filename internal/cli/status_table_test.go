package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Chinese headers are two cells wide but three bytes long. A byte-based table
// (text/tabwriter) misaligns every row; the columns must line up visually.
func TestStatusTableAlignsWithChineseHeaders(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunStart(context.Background(), rt, "fake/other", nil, true)

	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStatus(context.Background(), rt, "", false)
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + 2 rows, got %q", out)
	}

	// Every row must start the phase column at the same display offset.
	positions := map[int]bool{}
	for _, line := range lines[1:] {
		idx := strings.Index(line, "running")
		if idx < 0 {
			t.Fatalf("row without phase: %q", line)
		}
		positions[cli.DisplayWidth(line[:idx])] = true
	}
	if len(positions) != 1 {
		t.Fatalf("phase column misaligned, offsets=%v\n%s", positions, out)
	}
}

func TestStatusEmptySuggestsList(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStatus(context.Background(), rt, "", false)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "chaoslab list") {
		t.Fatalf("empty status must point at list: %q", out)
	}
}

// status must accept a whole plugin name, since a plugin can own several cases.
func TestStatusFiltersByPlugin(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)

	out, err := captureStdout(t, func() (int, error) {
		return cli.RunStatus(context.Background(), rt, "fake", true)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "fake/env#1") {
		t.Fatalf("plugin filter must match its instances: %q", out)
	}
}
