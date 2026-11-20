package plugin

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/soulchild/chaos-lab/internal/state"
)

var (
	ErrAlreadyInstalled = errors.New("plugin already installed")
	ErrInUse            = errors.New("plugin has live instances")
)

// knownClaimPrefixes are the prefixes the framework can verify itself after a
// stop. Anything else is allowed but unchecked, and reported as a warning.
var knownClaimPrefixes = []string{"port:", "docker:", "kind:", "dir:"}

// Install copies src into root/<name>. src is a directory or a git URL. The
// plugin is validated by calling its schema and list subcommands before it is
// moved into place, so a plugin that does not implement the protocol is
// rejected instead of installed broken.
func Install(ctx context.Context, src, root string) (string, error) {
	dir := src
	if !isGitURL(src) {
		abs, err := filepath.Abs(src)
		if err != nil {
			return "", err
		}
		dir = abs
	}
	if isGitURL(src) {
		tmp, err := os.MkdirTemp("", "chaoslab-clone-*")
		if err != nil {
			return "", err
		}
		defer os.RemoveAll(tmp)
		clone := exec.Command("git", "clone", "--depth", "1", src, tmp+"/repo")
		clone.Stderr = os.Stderr
		if err := clone.Run(); err != nil {
			return "", fmt.Errorf("git clone %s: %w", src, err)
		}
		dir = tmp + "/repo"
	}

	entry := filepath.Join(dir, "plugin")
	st, err := os.Stat(entry)
	if err != nil || st.IsDir() {
		return "", fmt.Errorf("%w: %s has no plugin file", ErrProtocol, src)
	}
	if st.Mode()&0o111 == 0 {
		if err := os.Chmod(entry, 0o755); err != nil {
			return "", err
		}
	}

	l := &Lister{PluginDir: dir}
	schema, err := l.Schema(ctx)
	if err != nil {
		return "", err
	}
	if _, err := l.List(ctx, "", nil); err != nil {
		return "", err
	}

	dest := filepath.Join(root, schema.Name)
	if _, err := os.Stat(dest); err == nil {
		return "", fmt.Errorf("%w: %s", ErrAlreadyInstalled, schema.Name)
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return "", err
	}
	if err := copyTree(dir, dest); err != nil {
		os.RemoveAll(dest)
		return "", err
	}
	return schema.Name, nil
}

// Uninstall removes an installed plugin. It refuses while any instance of that
// plugin is still live, because live instances need the plugin to clean up.
func Uninstall(root, name string, instances []state.Instance) error {
	// name is joined into a path, so it must be a plain plugin name. Without this
	// a name like ".." or "../../x" would delete outside root.
	if !nameRe.MatchString(name) {
		return fmt.Errorf("%w: %q 不是合法的插件名", ErrUserInput, name)
	}
	for _, in := range instances {
		if in.Plugin == name && in.Live {
			return fmt.Errorf("%w: %s still owns %s; run chaoslab stop first", ErrInUse, name, in.ID)
		}
	}
	dest := filepath.Join(root, name)
	if _, err := os.Stat(dest); err != nil {
		return fmt.Errorf("plugin %s is not installed", name)
	}
	return os.RemoveAll(dest)
}

// Validate checks a plugin directory without running up. It returns warnings
// (not errors) for claims the framework cannot verify itself.
func Validate(ctx context.Context, dir string) ([]string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	dir = abs
	entry := filepath.Join(dir, "plugin")
	st, err := os.Stat(entry)
	if err != nil || st.IsDir() {
		return nil, fmt.Errorf("%w: %s has no plugin file", ErrProtocol, dir)
	}
	if st.Mode()&0o111 == 0 {
		return nil, fmt.Errorf("%w: %s is not executable", ErrProtocol, entry)
	}
	l := &Lister{PluginDir: dir}
	if _, err := l.Schema(ctx); err != nil {
		return nil, err
	}
	cases, err := l.List(ctx, "", nil)
	if err != nil {
		return nil, err
	}
	var warnings []string
	for _, c := range cases {
		for _, cl := range c.Claims {
			if !hasKnownPrefix(cl) {
				warnings = append(warnings, fmt.Sprintf("case %s: claim %q uses an unknown prefix; the framework will not verify it after stop", c.ID, cl))
			}
		}
	}
	return warnings, nil
}

func hasKnownPrefix(claim string) bool {
	for _, p := range knownClaimPrefixes {
		if strings.HasPrefix(claim, p) {
			return true
		}
	}
	return false
}

func isGitURL(s string) bool {
	return strings.HasPrefix(s, "git@") || strings.HasPrefix(s, "git://") ||
		strings.HasPrefix(s, "http://") || strings.HasPrefix(s, "https://") ||
		strings.HasPrefix(s, "ssh://")
}

func copyTree(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			return os.Symlink(link, target)
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, info.Mode().Perm())
	})
}
