package cli_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// A plugin name is joined into a filesystem path, so it must never escape
// ~/.lab/plugins.
func TestPluginNameCannotEscapePluginsDir(t *testing.T) {
	rt := cli.NewRuntime(t.TempDir())
	for _, name := range []string{"../../etc", "..", "a/b", "A_B", ""} {
		if code, err := cli.RunPlugin(context.Background(), rt, "uninstall", []string{name}, false); code != 1 || err == nil {
			t.Fatalf("uninstall %q must be refused, code=%d err=%v", name, code, err)
		}
		if code, err := cli.RunPlugin(context.Background(), rt, "info", []string{name}, false); code != 1 || err == nil {
			t.Fatalf("info %q must be refused, code=%d err=%v", name, code, err)
		}
	}
}

func TestStartRejectsPluginNameWithSlashEscape(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStart(context.Background(), rt, "../../etc/passwd", nil, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

func TestStartRejectsCaseWithSlash(t *testing.T) {
	rt, _ := newTestRuntime(t)
	code, err := cli.RunStart(context.Background(), rt, "fake/no/such", nil, true)
	if code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}

// Uninstall must not reach outside the plugins directory even if a plugin dir
// is a symlink.
func TestUninstallOnlyRemovesInsidePluginsDir(t *testing.T) {
	root := t.TempDir()
	victim := filepath.Join(root, "victim")
	if err := os.MkdirAll(victim, 0o755); err != nil {
		t.Fatal(err)
	}
	pluginsDir := filepath.Join(root, "plugins")
	if err := os.MkdirAll(pluginsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	err := plugin.Uninstall(pluginsDir, "victim", nil)
	if err == nil {
		t.Fatal("uninstalling a name that is not installed must fail")
	}
	if _, statErr := os.Stat(victim); statErr != nil {
		t.Fatalf("victim directory was removed: %v", statErr)
	}
}

func TestResolveRefusesNonLiveForStop(t *testing.T) {
	instances := []state.Instance{
		{ID: "fake/env#1", Plugin: "fake", Case: "env", Live: false, Phase: state.PhaseStopped},
	}
	_, err := cli.Resolve(instances, "fake/env")
	if !errors.Is(err, plugin.ErrUserInput) {
		t.Fatalf("want ErrUserInput, got %v", err)
	}
	if !strings.Contains(err.Error(), "fake/env") {
		t.Fatalf("error must name the ref: %v", err)
	}
}
