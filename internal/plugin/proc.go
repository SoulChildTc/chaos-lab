package plugin

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

// ErrTimeout is returned when the child outlived its deadline and was killed.
var ErrTimeout = errors.New("plugin call timed out")

// termGrace is how long a killed process group gets before SIGKILL.
const termGrace = 5 * time.Second

// Run executes name with args once in dir. Stderr goes straight to os.Stderr so
// the user sees plugin progress live; stdout is returned to the caller as
// protocol data. A cancelled or expired ctx kills the whole process group,
// because plugins spawn children (kind, docker) that must not outlive the call.
func Run(ctx context.Context, dir string, env []string, name string, args ...string) ([]byte, int, error) {
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	cmd.Stdin = nil // inherited as /dev/null by exec
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		return nil, -1, err
	}

	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()

	var err error
	select {
	case err = <-done:
	case <-ctx.Done():
		killGroup(cmd.Process.Pid)
		select {
		case <-done:
		case <-time.After(termGrace + time.Second):
		}
		return stdout.Bytes(), -1, ErrTimeout
	}

	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return stdout.Bytes(), exitErr.ExitCode(), nil
	}
	if err != nil {
		return stdout.Bytes(), -1, err
	}
	return stdout.Bytes(), 0, nil
}

func killGroup(pid int) {
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	go func() {
		time.Sleep(termGrace)
		_ = syscall.Kill(-pid, syscall.SIGKILL)
	}()
}
