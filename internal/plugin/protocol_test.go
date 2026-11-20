package plugin_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
)

func writePlugin(t *testing.T, body string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "plugin"), []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestSchemaParsesArgs(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
[ "$1" = schema ] || exit 9
echo '{"name":"fake","version":"0.1.0","args":[{"name":"fail-down","type":"bool","default":false,"help":"h"}]}'
`)
	s, err := (&plugin.Lister{PluginDir: dir}).Schema(context.Background())
	if err != nil || s.Name != "fake" || len(s.Args) != 1 || s.Args[0].Type != "bool" {
		t.Fatalf("s=%+v err=%v", s, err)
	}
}

func TestSchemaRejectsUnknownArgType(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
echo '{"name":"fake","args":[{"name":"x","type":"float"}]}'
`)
	_, err := (&plugin.Lister{PluginDir: dir}).Schema(context.Background())
	if !errors.Is(err, plugin.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestSchemaRejectsBadName(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
echo '{"name":"Bad_Name","args":[]}'
`)
	_, err := (&plugin.Lister{PluginDir: dir}).Schema(context.Background())
	if !errors.Is(err, plugin.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestListRejectsNonJSON(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
echo 'not json'
`)
	_, err := (&plugin.Lister{PluginDir: dir}).List(context.Background(), "", nil)
	if !errors.Is(err, plugin.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestListPassesInstanceAndArgsFile(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
shift
for a in "$@"; do
  case "$a" in
    --instance=*) inst="${a#--instance=}";;
    --args=@*) file="${a#--args=@}";;
  esac
done
has=missing
grep -q '"a":"b"' "$file" && has=present
printf '{"cases":[{"id":"c1","title":"%s|%s"}]}' "$inst" "$has"
`)
	cases, err := (&plugin.Lister{PluginDir: dir}).List(context.Background(), "slug-1", map[string]any{"a": "b"})
	if err != nil || len(cases) != 1 {
		t.Fatalf("cases=%+v err=%v", cases, err)
	}
	if cases[0].Title != "slug-1|present" {
		t.Fatalf("title=%q", cases[0].Title)
	}
}

func TestListOmitsArgsFileWhenNoArgs(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
for a in "$@"; do
  case "$a" in --args=*) echo '{"cases":[{"id":"bad_args_passed","title":""}]}'; exit 0;; esac
done
echo '{"cases":[{"id":"ok","title":"t"}]}'
`)
	cases, err := (&plugin.Lister{PluginDir: dir}).List(context.Background(), "", nil)
	if err != nil || len(cases) != 1 || cases[0].ID != "ok" {
		t.Fatalf("cases=%+v err=%v", cases, err)
	}
}

func TestListRejectsBadCaseID(t *testing.T) {
	dir := writePlugin(t, `#!/bin/sh
echo '{"cases":[{"id":"Bad_ID","title":"t"}]}'
`)
	_, err := (&plugin.Lister{PluginDir: dir}).List(context.Background(), "", nil)
	if !errors.Is(err, plugin.ErrProtocol) {
		t.Fatalf("want ErrProtocol, got %v", err)
	}
}

func TestDiscoverFindsOnlyExecutablePluginDirs(t *testing.T) {
	root := t.TempDir()
	good := filepath.Join(root, "good")
	os.MkdirAll(good, 0o755)
	os.WriteFile(filepath.Join(good, "plugin"), []byte("#!/bin/sh\n"), 0o755)

	notExec := filepath.Join(root, "notexec")
	os.MkdirAll(notExec, 0o755)
	os.WriteFile(filepath.Join(notExec, "plugin"), []byte("x"), 0o644)

	os.MkdirAll(filepath.Join(root, "empty"), 0o755)

	got, err := plugin.Discover(root)
	if err != nil || len(got) != 1 || got[0] != "good" {
		t.Fatalf("got=%v err=%v", got, err)
	}
}

func TestDiscoverMissingRootIsEmpty(t *testing.T) {
	got, err := plugin.Discover(filepath.Join(t.TempDir(), "nope"))
	if err != nil || len(got) != 0 {
		t.Fatalf("got=%v err=%v", got, err)
	}
}
