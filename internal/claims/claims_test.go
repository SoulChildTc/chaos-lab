package claims_test

import (
	"testing"

	"github.com/soulchild/chaos-lab/internal/claims"
	"github.com/soulchild/chaos-lab/internal/state"
)

func TestConflictFindsFirstSharedClaim(t *testing.T) {
	c, ok := claims.Conflict([]string{"port:8080", "dir:/x"}, []string{"kind:a", "port:8080"})
	if !ok || c != "port:8080" {
		t.Fatalf("c=%q ok=%v", c, ok)
	}
}

func TestConflictIsOrderIndependent(t *testing.T) {
	a, _ := claims.Conflict([]string{"dir:/x", "port:8080"}, []string{"port:8080"})
	b, _ := claims.Conflict([]string{"port:8080"}, []string{"dir:/x", "port:8080"})
	if a != b {
		t.Fatalf("a=%q b=%q", a, b)
	}
}

func TestConflictEmptyIsNoConflict(t *testing.T) {
	if _, ok := claims.Conflict(nil, []string{"port:1"}); ok {
		t.Fatal("empty claims must not conflict")
	}
}

func TestConflictsWithSkipsNonLive(t *testing.T) {
	live := []state.Instance{
		{ID: "a", Claims: []string{"port:1"}, Live: false},
		{ID: "b", Claims: []string{"port:2"}, Live: true},
	}
	id, c, found := claims.ConflictsWith([]string{"port:2"}, live)
	if !found || id != "b" || c != "port:2" {
		t.Fatalf("id=%q c=%q found=%v", id, c, found)
	}
	if _, _, found := claims.ConflictsWith([]string{"port:1"}, live); found {
		t.Fatal("non-live instance must not conflict")
	}
}

func TestConflictsWithReportsFirstClaimInNewOrder(t *testing.T) {
	live := []state.Instance{{ID: "x", Claims: []string{"dir:/shared", "port:5"}, Live: true}}
	_, c, _ := claims.ConflictsWith([]string{"dir:/shared", "port:5"}, live)
	if c != "dir:/shared" {
		t.Fatalf("want first claim of the new instance, got %q", c)
	}
}
