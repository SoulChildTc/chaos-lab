// Package claims detects resource collisions between running instances and
// verifies that a stopped instance really released what it declared.
//
// The framework only compares strings: the plugin renders its own claims for a
// given instance and args, so conditional claims (reuse an existing cluster vs
// create one) stay the plugin's business.
package claims

import "github.com/soulchildtc/chaos-lab/internal/state"

// Conflict returns the first claim present in both sets.
func Conflict(a, b []string) (string, bool) {
	if len(a) == 0 || len(b) == 0 {
		return "", false
	}
	set := make(map[string]struct{}, len(b))
	for _, c := range b {
		set[c] = struct{}{}
	}
	for _, c := range a {
		if _, ok := set[c]; ok {
			return c, true
		}
	}
	return "", false
}

// ConflictsWith reports the first claim of want that a live instance already
// holds, naming that instance. Only Live instances count: a stopped one no
// longer owns its resources.
func ConflictsWith(want []string, instances []state.Instance) (instID, claim string, found bool) {
	for _, c := range want {
		for _, in := range instances {
			if !in.Live {
				continue
			}
			for _, held := range in.Claims {
				if held == c {
					return in.ID, c, true
				}
			}
		}
	}
	return "", "", false
}
