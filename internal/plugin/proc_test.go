package plugin_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
	"strings"
	"testing"
	"time"

	"github.com/soulchild/chaos-lab/internal/plugin"
)

func TestRunCapturesStdoutAndPassesStderr(t *testing.T) {
	out, code, err := plugin.Run(context.Background(), t.TempDir(), nil, "sh", "-c", "echo hello; echo warn >&2; exit 0")
	if err != nil || code != 0 || strings.TrimSpace(string(out)) != "hello" {
		t.Fatalf("out=%q code=%d err=%v", out, code, err)
	}
}

func TestRunReturnsExitCodeWithoutError(t *testing.T) {
	_, code, err := plugin.Run(context.Background(), t.TempDir(), nil, "sh", "-c", "exit 1")
	if err != nil || code != 1 {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestRunKillsProcessGroupOnTimeout(t *testing.T) {
	dir := t.TempDir()
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	start := time.Now()
	// The child spawns its own background sleep; a whole-group kill must take it too.
	_, _, err := plugin.Run(ctx, dir, nil, "sh", "-c", "sleep 30 & echo $! > child.pid; wait")
	if !errors.Is(err, plugin.ErrTimeout) {
		t.Fatalf("want ErrTimeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 8*time.Second {
		t.Fatalf("timeout not enforced: %v", elapsed)
	}
	// The grandchild must be dead, else timeout leaves orphans.
	b, readErr := os.ReadFile(filepath.Join(dir, "child.pid"))
	if readErr == nil {
		pid, _ := strconv.Atoi(strings.TrimSpace(string(b)))
		if pid > 0 {
			time.Sleep(500 * time.Millisecond)
			if syscall.Kill(pid, 0) == nil {
				_ = syscall.Kill(pid, syscall.SIGKILL)
				t.Fatalf("grandchild %d survived the process-group kill", pid)
			}
		}
	}
}

func TestRunSetsWorkingDirectoryAndEnv(t *testing.T) {
	dir := t.TempDir()
	want, err := filepath.EvalSymlinks(dir)
	if err != nil {
		t.Fatal(err)
	}
	out, _, err := plugin.Run(context.Background(), dir, []string{"LAB_X=42"}, "sh", "-c", "pwd; echo $LAB_X")
	lines := strings.Fields(strings.TrimSpace(string(out)))
	if err != nil || len(lines) != 2 || lines[0] != want || lines[1] != "42" {
		t.Fatalf("out=%q want dir %q err=%v", out, want, err)
	}
}
