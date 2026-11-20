package cli_test

import (
	"context"
	"encoding/json"
	"runtime"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Release artefacts must be able to say what they are: a binary from a release
// page carries no git information, so the version is baked in at link time.
func TestVersionDefaultsToDev(t *testing.T) {
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), cli.NewRuntime(t.TempDir()), []string{"version"})
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "dev") {
		t.Fatalf("default version must be dev, got %q", out)
	}
}

func TestVersionJSON(t *testing.T) {
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), cli.NewRuntime(t.TempDir()), []string{"version", "--json"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Version string `json:"version"`
		Commit  string `json:"commit"`
		OS      string `json:"os"`
		Arch    string `json:"arch"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	if got.Version == "" || got.OS != runtime.GOOS || got.Arch != runtime.GOARCH {
		t.Fatalf("%+v", got)
	}
}

// -v is what people type first, and the release docs promise it.
func TestVersionShortFlag(t *testing.T) {
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), cli.NewRuntime(t.TempDir()), []string{"-v"})
	})
	if err != nil || !strings.Contains(string(out), "chaoslab") {
		t.Fatalf("out=%q err=%v", out, err)
	}
}
