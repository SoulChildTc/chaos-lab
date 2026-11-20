package plugin_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

const fakePluginBody = `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake","version":"0.1.0","args":[]}' ;;
list)   echo '{"cases":[{"id":"env","title":"t"}]}' ;;
*) exit 9 ;;
esac
`

func writePluginDir(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestInstallCopiesPluginIntoRoot(t *testing.T) {
	root := t.TempDir()
	name, err := plugin.Install(context.Background(), writePluginDir(t, fakePluginBody), root)
	if err != nil || name != "fake" {
		t.Fatalf("name=%q err=%v", name, err)
	}
	if _, err := os.Stat(filepath.Join(root, "fake", "plugin")); err != nil {
		t.Fatalf("plugin not installed: %v", err)
	}
}

func TestInstallMakesPluginExecutable(t *testing.T) {
	src := t.TempDir()
	if err := os.WriteFile(filepath.Join(src, "plugin"), []byte(fakePluginBody), 0o644); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), src, root); err != nil {
		t.Fatalf("err=%v", err)
	}
	st, _ := os.Stat(filepath.Join(root, "fake", "plugin"))
	if st.Mode()&0o111 == 0 {
		t.Fatalf("installed plugin is not executable: %v", st.Mode())
	}
}

func TestInstallRejectsSecondInstall(t *testing.T) {
	src := writePluginDir(t, fakePluginBody)
	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), src, root); err != nil {
		t.Fatalf("first install: %v", err)
	}
	if _, err := plugin.Install(context.Background(), src, root); !errors.Is(err, plugin.ErrAlreadyInstalled) {
		t.Fatalf("want ErrAlreadyInstalled, got %v", err)
	}
}

func TestInstallRejectsFailingSchema(t *testing.T) {
	src := writePluginDir(t, "#!/bin/sh\nexit 1\n")
	if _, err := plugin.Install(context.Background(), src, t.TempDir()); err == nil {
		t.Fatal("want error for failing schema")
	}
}

func TestInstallRejectsMissingPluginFile(t *testing.T) {
	if _, err := plugin.Install(context.Background(), t.TempDir(), t.TempDir()); err == nil {
		t.Fatal("want error for directory without plugin")
	}
}

func TestInstallLeavesNothingBehindOnFailure(t *testing.T) {
	root := t.TempDir()
	src := writePluginDir(t, "#!/bin/sh\nexit 1\n")
	_, _ = plugin.Install(context.Background(), src, root)
	entries, _ := os.ReadDir(root)
	if len(entries) != 0 {
		t.Fatalf("half-installed: %v", entries)
	}
}

func TestUninstallRemovesPlugin(t *testing.T) {
	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), writePluginDir(t, fakePluginBody), root); err != nil {
		t.Fatal(err)
	}
	if err := plugin.Uninstall(root, "fake", nil); err != nil {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "fake")); !os.IsNotExist(err) {
		t.Fatalf("plugin still present: %v", err)
	}
}

func TestUninstallRefusesLiveInstances(t *testing.T) {
	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), writePluginDir(t, fakePluginBody), root); err != nil {
		t.Fatal(err)
	}
	err := plugin.Uninstall(root, "fake", []state.Instance{{ID: "fake/env#1", Plugin: "fake", Live: true}})
	if !errors.Is(err, plugin.ErrInUse) {
		t.Fatalf("want ErrInUse, got %v", err)
	}
}

func TestUninstallAllowsNonLiveInstances(t *testing.T) {
	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), writePluginDir(t, fakePluginBody), root); err != nil {
		t.Fatal(err)
	}
	err := plugin.Uninstall(root, "fake", []state.Instance{{ID: "fake/env#1", Plugin: "fake", Live: false}})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
}

func TestUninstallMissingPluginErrors(t *testing.T) {
	if err := plugin.Uninstall(t.TempDir(), "nope", nil); err == nil {
		t.Fatal("want error")
	}
}

func TestValidateWarnsUnknownClaimPrefix(t *testing.T) {
	src := writePluginDir(t, `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake","args":[]}' ;;
list)   echo '{"scenarios":[{"id":"c","title":"t"}],"claims":["remote:jenkins"]}' ;;
esac
`)
	warn, err := plugin.Validate(context.Background(), src)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if len(warn) != 1 || !strings.Contains(warn[0], "remote:") {
		t.Fatalf("warn=%v", warn)
	}
}

func TestValidateAcceptsKnownPrefixes(t *testing.T) {
	src := writePluginDir(t, `#!/bin/sh
case "$1" in
schema) echo '{"name":"fake","args":[]}' ;;
list)   echo '{"scenarios":[{"id":"c","title":"t"}],"claims":["port:1","dir:/x","kind:y","docker:network:z"]}' ;;
esac
`)
	warn, err := plugin.Validate(context.Background(), src)
	if err != nil || len(warn) != 0 {
		t.Fatalf("warn=%v err=%v", warn, err)
	}
}

func TestValidateFailsOnBrokenProtocol(t *testing.T) {
	src := writePluginDir(t, "#!/bin/sh\necho nope\n")
	if _, err := plugin.Validate(context.Background(), src); !errors.Is(err, plugin.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestInstallAcceptsRelativeSourcePath(t *testing.T) {
	// A relative path must work: the plugin child runs with its own directory as
	// cwd, so an unresolved relative entry path would be a fork/exec error.
	parent := t.TempDir()
	src := filepath.Join(parent, "fakeplug")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "plugin"), []byte(fakePluginBody), 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	root := t.TempDir()
	if _, err := plugin.Install(context.Background(), "fakeplug", root); err != nil {
		t.Fatalf("relative install failed: %v", err)
	}
}

func TestValidateAcceptsRelativePath(t *testing.T) {
	parent := t.TempDir()
	src := filepath.Join(parent, "fakeplug")
	if err := os.MkdirAll(src, 0o755); err != nil {
		t.Fatal(err)
	}
	body := "#!/bin/sh\ncase \"$1\" in schema) echo '{\"name\":\"fake\",\"args\":[]}' ;; list) echo '{\"cases\":[{\"id\":\"c\",\"title\":\"t\"}]}' ;; esac\n"
	if err := os.WriteFile(filepath.Join(src, "plugin"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	wd, _ := os.Getwd()
	if err := os.Chdir(parent); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chdir(wd) })

	if _, err := plugin.Validate(context.Background(), "fakeplug"); err != nil {
		t.Fatalf("relative validate failed: %v", err)
	}
}
