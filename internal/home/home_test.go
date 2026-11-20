package home_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/home"
)

func TestDirUsesEnvOverride(t *testing.T) {
	t.Setenv("CHAOSLAB_HOME", "/tmp/lab-test-home")
	d, err := home.Dir()
	if err != nil || d != "/tmp/lab-test-home" {
		t.Fatalf("d=%q err=%v", d, err)
	}
}

func TestDirFallsBackToHomeDotLab(t *testing.T) {
	t.Setenv("CHAOSLAB_HOME", "")
	uh, _ := os.UserHomeDir()
	d, err := home.Dir()
	if err != nil || d != filepath.Join(uh, ".lab") {
		t.Fatalf("d=%q err=%v", d, err)
	}
}

func TestEnsureCreatesNestedDirs(t *testing.T) {
	root := t.TempDir()
	t.Setenv("CHAOSLAB_HOME", root)
	p, err := home.Ensure("state", "deep")
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if p != filepath.Join(root, "state", "deep") {
		t.Fatalf("p=%q", p)
	}
	if st, err := os.Stat(p); err != nil || !st.IsDir() {
		t.Fatalf("not created: %v", err)
	}
}
