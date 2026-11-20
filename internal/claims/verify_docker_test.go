package claims_test

import (
	"context"
	"os/exec"
	"testing"

	"github.com/soulchild/chaos-lab/internal/claims"
)

// A docker daemon that is down (or a permission error) must not be reported as
// "the object is gone". Only a definitive "no such object" counts as released.
func TestVerifyDockerDaemonDownIsUnchecked(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	// Point docker at a socket that cannot exist: every inspect fails, but not
	// with "no such object".
	t.Setenv("DOCKER_HOST", "unix:///nonexistent/chaoslab-test.sock")
	vs := claims.Verify(context.Background(), []string{"docker:container:definitely-not-there"})
	if len(vs) != 1 {
		t.Fatalf("%+v", vs)
	}
	if vs[0].Released {
		t.Fatalf("an unreachable daemon must not report released: %+v", vs[0])
	}
	if vs[0].Checked {
		t.Fatalf("an unreachable daemon must be unchecked: %+v", vs[0])
	}
	if claims.AllReleased(vs) {
		t.Fatal("unchecked must never count as released")
	}
}

// A missing docker object must still be released: down succeeded.
func TestVerifyDockerMissingObjectIsReleased(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		t.Skip("docker not installed")
	}
	vs := claims.Verify(context.Background(), []string{"docker:container:chaoslab-definitely-absent-xyz"})
	if len(vs) != 1 || !vs[0].Checked || !vs[0].Released {
		t.Fatalf("%+v", vs)
	}
}
