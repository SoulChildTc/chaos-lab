package cli_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulchild/chaos-lab/internal/cli"
	"github.com/soulchild/chaos-lab/internal/plugin"
)

// installSecondPlugin adds a second plugin so list has to walk more than one.
func installSecondPlugin(t *testing.T, rt *cli.Runtime) {
	t.Helper()
	src := filepath.Join(t.TempDir(), "plugin")
	body := "#!/bin/sh\ncase \"$1\" in\nschema) echo '{\"name\":\"fake2\",\"args\":[]}' ;;\nlist) echo '{\"cases\":[{\"id\":\"only\",\"title\":\"第二个插件\"}]}' ;;\nesac\n"
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

	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, true) })
	if err != nil {
		t.Fatal(err)
	}
	var got []cli.ListEntry
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q (%v)", out, err)
	}
	if len(got) != 3 {
		t.Fatalf("want 3 cases (2 fake + 1 fake2), got %+v", got)
	}
	var sawSecond bool
	for _, e := range got {
		if e.Plugin == "fake2" && e.Case == "only" {
			sawSecond = true
		}
	}
	if !sawSecond {
		t.Fatalf("second plugin missing: %+v", got)
	}
}

func TestListEmptySaysSo(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, false) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "没有") {
		t.Fatalf("out=%q", out)
	}
}

func TestListJSONEmptyIsArray(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, true) })
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(string(out)) != "[]" {
		t.Fatalf("out=%q", out)
	}
}

func TestListReportsBrokenPluginWithoutDroppingOthers(t *testing.T) {
	rt, _ := newTestRuntime(t)
	// A plugin that installs fine but is broken now: list must still show the
	// healthy ones, and say which plugin failed.
	brokenDir := filepath.Join(rt.PluginsDir, "broken")
	os.MkdirAll(brokenDir, 0o755)
	os.WriteFile(filepath.Join(brokenDir, "plugin"), []byte("#!/bin/sh\nexit 1\n"), 0o755)

	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, true) })
	if err != nil {
		t.Fatal(err)
	}
	var got []cli.ListEntry
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("stdout not JSON: %q", out)
	}
	if len(got) != 2 {
		t.Fatalf("healthy plugin cases lost: %+v", got)
	}
}

func TestListHumanOutputShowsRefs(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) { return cli.RunList(context.Background(), rt, false) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), "fake/env") {
		t.Fatalf("out=%q", out)
	}
}
