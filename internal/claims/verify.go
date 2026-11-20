package claims

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// Verdict is the outcome of verifying one claim. Checked=false means the
// framework cannot judge it: an unknown prefix, or a missing tool. Unchecked
// must never be reported as released.
type Verdict struct {
	Claim    string
	Checked  bool
	Released bool
	Detail   string
}

// Verify measures each claim against the real machine. It only observes; it
// never cleans anything up. Cleanup belongs to the plugin's down action.
func Verify(ctx context.Context, cs []string) []Verdict {
	out := make([]Verdict, 0, len(cs))
	for _, c := range cs {
		out = append(out, verifyOne(ctx, c))
	}
	return out
}

// AllReleased is true only when every claim was actually checked and released.
func AllReleased(vs []Verdict) bool {
	if len(vs) == 0 {
		return true
	}
	for _, v := range vs {
		if !v.Checked || !v.Released {
			return false
		}
	}
	return true
}

// Residuals are the claims proven to still be held.
func Residuals(vs []Verdict) []Verdict {
	var out []Verdict
	for _, v := range vs {
		if v.Checked && !v.Released {
			out = append(out, v)
		}
	}
	return out
}

// Unchecked are the claims the framework could not judge.
func Unchecked(vs []Verdict) []Verdict {
	var out []Verdict
	for _, v := range vs {
		if !v.Checked {
			out = append(out, v)
		}
	}
	return out
}

func verifyOne(ctx context.Context, c string) Verdict {
	switch {
	case strings.HasPrefix(c, "port:"):
		port, err := strconv.Atoi(strings.TrimPrefix(c, "port:"))
		if err != nil {
			return Verdict{Claim: c, Detail: "port is not a number"}
		}
		conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", port), 300*time.Millisecond)
		if err != nil {
			return Verdict{Claim: c, Checked: true, Released: true, Detail: "port is free"}
		}
		conn.Close()
		return Verdict{Claim: c, Checked: true, Released: false, Detail: "port is still listening"}

	case strings.HasPrefix(c, "dir:"):
		p := strings.TrimPrefix(c, "dir:")
		if _, err := os.Stat(p); os.IsNotExist(err) {
			return Verdict{Claim: c, Checked: true, Released: true, Detail: "path is gone"}
		} else if err != nil {
			return Verdict{Claim: c, Detail: err.Error()}
		}
		return Verdict{Claim: c, Checked: true, Released: false, Detail: "path still exists"}

	case strings.HasPrefix(c, "kind:"):
		name := strings.TrimPrefix(c, "kind:")
		out, ok := runMaybe(ctx, "kind", "get", "clusters")
		if !ok {
			return Verdict{Claim: c, Detail: "kind is not installed; cannot verify"}
		}
		for _, line := range strings.Split(string(out), "\n") {
			if strings.TrimSpace(line) == name {
				return Verdict{Claim: c, Checked: true, Released: false, Detail: "kind cluster still exists"}
			}
		}
		return Verdict{Claim: c, Checked: true, Released: true, Detail: "kind cluster is gone"}

	case strings.HasPrefix(c, "docker:"):
		rest := strings.TrimPrefix(c, "docker:")
		if !hasBinary("docker") {
			return Verdict{Claim: c, Detail: "docker is not installed; cannot verify"}
		}
		name := rest
		if i := strings.Index(rest, ":"); i >= 0 {
			name = rest[i+1:]
		}
		if name == "" {
			return Verdict{Claim: c, Detail: "docker claim has no name"}
		}
		out, err := exec.CommandContext(ctx, "docker", "inspect", name).CombinedOutput()
		if err == nil {
			return Verdict{Claim: c, Checked: true, Released: false, Detail: "docker object still exists"}
		}
		// Only a definitive "no such object" means the object is gone. A dead
		// daemon or a permission error says nothing about the object, so it must
		// stay unchecked rather than be reported as released.
		if isDockerMissingObject(string(out)) {
			return Verdict{Claim: c, Checked: true, Released: true, Detail: "docker object is gone"}
		}
		return Verdict{Claim: c, Detail: "cannot judge docker: " + firstLine(string(out))}
	}
	return Verdict{Claim: c, Detail: "unknown claim prefix; the plugin is responsible for this one"}
}

func hasBinary(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

// runMaybe runs a command, returning ok=false when the binary is missing so the
// claim is reported as unchecked rather than as released.
func runMaybe(ctx context.Context, name string, args ...string) ([]byte, bool) {
	if !hasBinary(name) {
		return nil, false
	}
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return nil, false
	}
	return out, true
}

// isDockerMissingObject recognises the only error that means the object is
// really gone. Anything else (daemon down, permission, bad socket) says nothing
// about the object and must stay unchecked.
func isDockerMissingObject(out string) bool {
	lower := strings.ToLower(out)
	return strings.Contains(lower, "no such object") ||
		strings.Contains(lower, "no such container") ||
		strings.Contains(lower, "no such volume") ||
		strings.Contains(lower, "no such network")
}

func firstLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	if len(s) > 160 {
		s = s[:160] + "..."
	}
	return s
}
