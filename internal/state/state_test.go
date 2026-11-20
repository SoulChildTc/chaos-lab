package state_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/soulchild/chaos-lab/internal/state"
)

func TestUpdateAllocatesSeqAndPersists(t *testing.T) {
	st := state.Open(filepath.Join(t.TempDir(), "instances.json"))
	err := st.Update(context.Background(), func(f *state.File) error {
		if f.Seq == nil {
			f.Seq = map[string]int{}
		}
		f.Seq["k8s/c1"]++
		f.Instances = append(f.Instances, state.Instance{ID: "k8s/c1#1", Live: true})
		return nil
	})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	f, err := st.Read(context.Background())
	if err != nil || f.Seq["k8s/c1"] != 1 || len(f.Instances) != 1 {
		t.Fatalf("f=%+v err=%v", f, err)
	}
}

func TestReadMissingFileIsEmpty(t *testing.T) {
	st := state.Open(filepath.Join(t.TempDir(), "instances.json"))
	f, err := st.Read(context.Background())
	if err != nil || len(f.Instances) != 0 {
		t.Fatalf("f=%+v err=%v", f, err)
	}
}

func TestReadCorruptFileReportsPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "instances.json")
	if err := os.WriteFile(p, []byte("{oops"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := state.Open(p).Read(context.Background())
	if err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("want error naming %s, got %v", p, err)
	}
}

func TestUpdateIsSerializedAcrossConcurrentCallers(t *testing.T) {
	st := state.Open(filepath.Join(t.TempDir(), "instances.json"))
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_ = st.Update(context.Background(), func(f *state.File) error {
				if f.Seq == nil {
					f.Seq = map[string]int{}
				}
				f.Seq["k8s/c1"]++
				return nil
			})
		}()
	}
	wg.Wait()
	f, _ := st.Read(context.Background())
	if f.Seq["k8s/c1"] != 20 {
		t.Fatalf("lost update: seq=%d", f.Seq["k8s/c1"])
	}
}

func TestUpdateRollsBackOnFnError(t *testing.T) {
	st := state.Open(filepath.Join(t.TempDir(), "instances.json"))
	_ = st.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{ID: "a"})
		return nil
	})
	err := st.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{ID: "b"})
		return errors.New("boom")
	})
	if err == nil {
		t.Fatal("want error")
	}
	f, _ := st.Read(context.Background())
	if len(f.Instances) != 1 || f.Instances[0].ID != "a" {
		t.Fatalf("partial write persisted: %+v", f.Instances)
	}
}

func TestUpdateCreatesParentDir(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nested", "deep", "instances.json")
	st := state.Open(p)
	if err := st.Update(context.Background(), func(f *state.File) error { return nil }); err != nil {
		t.Fatalf("err=%v", err)
	}
	if _, err := os.Stat(p); err != nil {
		t.Fatalf("state file not written: %v", err)
	}
}

func TestUpdateLeavesNoTempFiles(t *testing.T) {
	dir := t.TempDir()
	st := state.Open(filepath.Join(dir, "instances.json"))
	if err := st.Update(context.Background(), func(f *state.File) error { return nil }); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("temp file left behind: %s", e.Name())
		}
	}
}

func TestInstanceTimesRoundTripInJSON(t *testing.T) {
	p := filepath.Join(t.TempDir(), "instances.json")
	st := state.Open(p)
	now := time.Now().UTC().Truncate(time.Second)
	_ = st.Update(context.Background(), func(f *state.File) error {
		f.Instances = append(f.Instances, state.Instance{ID: "x", StartedAt: now, LastCheck: &now})
		return nil
	})
	f, _ := st.Read(context.Background())
	if !f.Instances[0].StartedAt.Equal(now) || f.Instances[0].LastCheck == nil || !f.Instances[0].LastCheck.Equal(now) {
		t.Fatalf("%+v", f.Instances[0])
	}
}
