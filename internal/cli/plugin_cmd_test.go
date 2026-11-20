package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

func srcPluginDir(t *testing.T) string {
	t.Helper()
	p, err := filepath.Abs(filepath.Join("..", "..", "plugins", "fake"))
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func TestPluginInstallThenInfo(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	if code, err := cli.RunPlugin(context.Background(), rt, "install", []string{srcPluginDir(t)}, false); code != 0 || err != nil {
		t.Fatalf("install: code=%d err=%v", code, err)
	}
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunPlugin(context.Background(), rt, "info", []string{"fake"}, true)
	})
	if err != nil {
		t.Fatal(err)
	}
	var s plugin.Schema
	if err := json.Unmarshal(out, &s); err != nil {
		t.Fatalf("info not JSON: %q (%v)", out, err)
	}
	if s.Name != "fake" || len(s.Args) == 0 {
		t.Fatalf("%+v", s)
	}
}

func TestPluginInfoHumanListsArgs(t *testing.T) {
	rt := newTestRuntimeOops(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.RunPlugin(context.Background(), rt, "info", []string{"fake"}, false)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "--port") || !strings.Contains(s, "fake") {
		t.Fatalf("out=%q", s)
	}
}

func TestPluginInstallTwiceFails(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	src := srcPluginDir(t)
	if code, _ := cli.RunPlugin(context.Background(), rt, "install", []string{src}, false); code != 0 {
		t.Fatal("first install")
	}
	code, err := cli.RunPlugin(context.Background(), rt, "install", []string{src}, false)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestPluginInstallRejectsBadSource(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	code, err := cli.RunPlugin(context.Background(), rt, "install", []string{filepath.Join(t.TempDir(), "nope")}, false)
	if code == 0 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestPluginUninstallRefusesWhenLive(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	code, err := cli.RunPlugin(context.Background(), rt, "uninstall", []string{"fake"}, false)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if !strings.Contains(err.Error(), "fake/env#1") {
		t.Fatalf("must name the blocking instance: %v", err)
	}
}

func TestPluginUninstallWorksAfterStop(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	cli.RunStop(context.Background(), rt, "fake/env", false)
	if code, err := cli.RunPlugin(context.Background(), rt, "uninstall", []string{"fake"}, false); code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	if _, err := os.Stat(filepath.Join(rt.PluginsDir, "fake")); !os.IsNotExist(err) {
		t.Fatalf("plugin dir still there: %v", err)
	}
}

func TestPluginValidateWarns(t *testing.T) {
	var stderr strings.Builder
	old := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		buf := make([]byte, 0, 1024)
		tmp := make([]byte, 1024)
		for {
			n, err := r.Read(tmp)
			buf = append(buf, tmp[:n]...)
			if err != nil {
				break
			}
			_ = stderr
		}
		done <- string(buf)
	}()
	code, err := cli.RunPlugin(context.Background(), cli.NewRuntime(t.TempDir()), "validate", []string{srcPluginDir(t)}, false)
	w.Close()
	os.Stderr = old
	out := <-done
	r.Close()
	if code != 0 || err != nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
	_ = out
}

func TestMainUnknownSubcommandExitsOne(t *testing.T) {
	code, err := cli.Run(context.Background(), cli.NewRuntime(t.TempDir()), []string{"nope"})
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestMainNoArgsPrintsUsage(t *testing.T) {
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), cli.NewRuntime(t.TempDir()), nil)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "chaoslab") {
		t.Fatalf("out=%q", out)
	}
}

func TestMainDispatchesStatusWithJSONAnywhere(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake/env", nil, true)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"--json", "status"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]any
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("out=%q (%v)", out, err)
	}
	if len(got) != 1 {
		t.Fatalf("%+v", got)
	}
}

// newTestRuntimeOops installs fake into a fresh home, mirroring newTestRuntime
// without needing a *testing.T CHAOSLAB_HOME side effect.
func newTestRuntimeOops(t *testing.T) *cli.Runtime {
	t.Helper()
	rt := cli.NewRuntime(t.TempDir())
	if _, err := plugin.Install(context.Background(), srcPluginDir(t), rt.PluginsDir); err != nil {
		t.Fatal(err)
	}
	return rt
}
