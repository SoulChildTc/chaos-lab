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

// installSecondPlugin adds a second plugin so list has to walk more than one.
func installSecondPlugin(t *testing.T, rt *cli.Runtime) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "plugin")
	body := `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake2","args":[]}' ;;
list)   echo '{"scenarios":[{"id":"only","title":"第二个插件的故障"}],"claims":[]}' ;;
esac
`
	if err := os.WriteFile(src, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := plugin.Install(context.Background(), filepath.Dir(src), rt.PluginsDir); err != nil {
		t.Fatal(err)
	}
}

func TestListUnionAcrossPlugins(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)

	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, "", true) })
	if err != nil {
		t.Fatal(err)
	}
	var got []cli.ListEntry
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out, err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 faults (2 fake + 1 fake2), got %+v", got)
	}
	var sawSecond bool
	for _, e := range got {
		if e.Plugin == "fake2" && e.Scenario == "only" {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Fatalf("second plugin missing: %+v", got)
	}
}

func TestListEmptySaysSo(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, "", false) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "No scenarios") {
		t.Fatalf("out=%q", out)
	}
}

func TestListJSONEmptyIsArray(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, "", true) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("out=%q", out)
	}
}

func TestListReportsBrokenPluginWithoutDroppingOthers(t *testing.T) {
	rt, _ := newTestRuntime(t)
	brokenDir := filepath.Join(rt.PluginsDir, "broken")
	if err := os.MkdirAll(brokenDir, 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(brokenDir, "plugin"), []byte("#!/bin/sh\nexit 1\n"), 0o755)

	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, "", true) })
	if err != nil {
		t.Fatal(err)
	}
	var got []cli.ListEntry
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q", out)
	}
	if len(got) != 2 {
		t.Fatalf("healthy plugin faults lost: %+v", got)
	}
}

func TestListHumanOutputShowsFaults(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, "", false) })
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "PLUGIN") || !strings.Contains(s, "env") {
		t.Fatalf("out=%q", s)
	}
}

func TestListFiltersByPlugin(t *testing.T) {
	rt, _ := newTestRuntime(t)
	installSecondPlugin(t, rt)

	out, err := captureStdout(t, func() (int, error) {
		return cli.RunList(context.Background(), rt, "fake2", false)
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "only") {
		t.Fatalf("filtered fault missing: %q", s)
	}
	if strings.Contains(s, "\tenv") || strings.Contains(s, "  env ") {
		t.Fatalf("filter leaked other plugins: %q", s)
	}
}

func TestListUnknownPluginIsUserError(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunList(context.Background(), rt, "nosuch", false)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
