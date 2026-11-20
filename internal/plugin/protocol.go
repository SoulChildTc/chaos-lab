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

// Scenario is one fault a plugin can inject. It is a catalogue entry: the user
// does not address it directly, because the plugin decides which faults it
// injects when asked to bring its environment up.
type Scenario struct {
	ID    string `json:"id"`
	Title string `json:"title"`
}

// Catalog is what `list` returns for one instance: the faults this plugin
// provides, and the resources it will claim when started.
type Catalog struct {
	Scenarios    []Scenario `json:"scenarios"`
	Claims       []string   `json:"claims"`
	UpTimeout    string     `json:"up_timeout"`
	CheckTimeout string     `json:"check_timeout"`
	// Symptom is what the user is told before investigating. It must describe
	// what is observed, never which fault was injected.
	Symptom string `json:"symptom"`
	// Connect tells the user how to reach the environment: a kubeconfig path, a
	// host/port/user/password, whatever their stack needs.
	Connect []string `json:"connect"`
}

// Lister talks to one installed plugin. Every call is a fresh child process:
// the plugin holds no state between calls beyond what it writes to disk.
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

// List returns the plugin's catalogue for one instance: which faults it injects
// and which resources it will hold. slug and args are passed through so the
// plugin can render instance-specific claims; the framework has no template
// language of its own.
func (l *Lister) List(ctx context.Context, slug string, args map[string]any) (*Catalog, error) {
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
	var c Catalog
	if err := json.Unmarshal(out, &c); err != nil {
		return nil, fmt.Errorf("%w: list is not valid JSON: %v", ErrProtocol, err)
	}
	seen := map[string]bool{}
	for _, sc := range c.Scenarios {
		if !nameRe.MatchString(sc.ID) {
			return nil, fmt.Errorf("%w: scenario id %q is invalid", ErrProtocol, sc.ID)
		}
		if seen[sc.ID] {
			return nil, fmt.Errorf("%w: duplicate scenario id %q", ErrProtocol, sc.ID)
		}
		seen[sc.ID] = true
	}
	return &c, nil
}

// UpResult is what `up` returns once the environment is ready.
type UpResult struct {
	Symptom string   `json:"symptom"`
	Connect []string `json:"connect"`
}

// ParseUpResult reads the report a plugin prints on a successful up. An empty
// stdout is tolerated — a plugin may only talk on stderr — but malformed JSON is
// a protocol error, because silently dropping the symptom would leave the user
// with nothing to investigate.
func ParseUpResult(out []byte) (*UpResult, error) {
	if len(out) == 0 {
		return &UpResult{}, nil
	}
	var r UpResult
	if err := json.Unmarshal(out, &r); err != nil {
		return nil, fmt.Errorf("%w: up returned invalid JSON: %v", ErrProtocol, err)
	}
	return &r, nil
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
