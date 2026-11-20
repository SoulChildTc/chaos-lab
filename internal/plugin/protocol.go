package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"

)

// ErrProtocol marks a plugin that answered but not according to the protocol.
// Callers map it to exit code 3, never to a user-input error.
var ErrProtocol = errors.New("plugin protocol error")

var nameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

type Schema struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Description string `json:"description"`
	Args        []Arg  `json:"args"`
}

type Arg struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Default any    `json:"default"`
	Help    string `json:"help"`
}

type Case struct {
	ID           string   `json:"id"`
	Title        string   `json:"title"`
	Claims       []string `json:"claims"`
	UpTimeout    string   `json:"up_timeout"`
	CheckTimeout string   `json:"check_timeout"`
}

// Lister talks to one installed plugin. Every call is a fresh child process:
// the plugin holds no state between calls.
type Lister struct {
	PluginDir string
}

func (l *Lister) entry() string { return filepath.Join(l.PluginDir, "plugin") }

func (l *Lister) run(ctx context.Context, args ...string) ([]byte, error) {
	out, code, err := Run(ctx, l.PluginDir, nil, l.entry(), args...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		return nil, fmt.Errorf("%w: %s %s exited %d", ErrProtocol, filepath.Base(l.PluginDir), args[0], code)
	}
	return out, nil
}

func (l *Lister) Schema(ctx context.Context) (*Schema, error) {
	out, err := l.run(ctx, "schema", "--json")
	if err != nil {
		return nil, err
	}
	var s Schema
	if err := json.Unmarshal(out, &s); err != nil {
		return nil, fmt.Errorf("%w: schema is not valid JSON: %v", ErrProtocol, err)
	}
	if !nameRe.MatchString(s.Name) {
		return nil, fmt.Errorf("%w: schema name %q is not a valid plugin name", ErrProtocol, s.Name)
	}
	for _, a := range s.Args {
		switch a.Type {
		case "string", "int", "bool":
		default:
			return nil, fmt.Errorf("%w: arg %q has type %q (want string|int|bool)", ErrProtocol, a.Name, a.Type)
		}
	}
	return &s, nil
}

// List returns the plugin's cases. slug and args are optional; when args is
// non-empty it is written to a temp file and passed as --args=@file so the
// plugin can render instance-specific claims without the framework knowing any
// template language.
func (l *Lister) List(ctx context.Context, slug string, args map[string]any) ([]Case, error) {
	argv := []string{"list", "--json"}
	if slug != "" {
		argv = append(argv, "--instance="+slug)
	}
	if len(args) > 0 {
		f, err := os.CreateTemp("", "chaoslab-args-*.json")
		if err != nil {
			return nil, err
		}
		defer os.Remove(f.Name())
		if err := json.NewEncoder(f).Encode(args); err != nil {
			f.Close()
			return nil, err
		}
		f.Close()
		argv = append(argv, "--args=@"+f.Name())
	}

	out, err := l.run(ctx, argv...)
	if err != nil {
		return nil, err
	}
	var payload struct {
		Cases []Case `json:"cases"`
	}
	if err := json.Unmarshal(out, &payload); err != nil {
		return nil, fmt.Errorf("%w: list is not valid JSON: %v", ErrProtocol, err)
	}
	seen := map[string]bool{}
	for _, c := range payload.Cases {
		if !nameRe.MatchString(c.ID) {
			return nil, fmt.Errorf("%w: case id %q is invalid", ErrProtocol, c.ID)
		}
		if seen[c.ID] {
			return nil, fmt.Errorf("%w: duplicate case id %q", ErrProtocol, c.ID)
		}
		seen[c.ID] = true
	}
	return payload.Cases, nil
}

// Discover lists subdirectories of root that contain an executable plugin file.
// A missing root is not an error: no plugins installed yet.
func Discover(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		st, err := os.Stat(filepath.Join(root, e.Name(), "plugin"))
		if err != nil || st.IsDir() || st.Mode()&0o111 == 0 {
			continue
		}
		names = append(names, e.Name())
	}
	return names, nil
}
