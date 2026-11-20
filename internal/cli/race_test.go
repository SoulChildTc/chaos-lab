package cli_test

import (
	"context"
	"sync"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
	"github.com/soulchildtc/chaos-lab/internal/state"
)

// Two different cases claiming the same port must not both be admitted. The
// claim is only known after the plugin answers list, so the reservation and the
// conflict check have to be one atomic step; a check split across two lock
// sections sees the other instance with empty claims and lets both through.
func TestConcurrentStartsCannotBothClaimSamePort(t *testing.T) {
	rt, _ := newTestRuntime(t)

	var wg sync.WaitGroup
	start := make(chan struct{})
	codes := make([]int, 2)
	refs := []string{"fake", "fake"}
	for i := range refs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			<-start
			codes[i], _ = cli.RunStart(context.Background(), rt, refs[i], map[string]string{"port": "18555"}, true)
		}(i)
	}
	close(start)
	wg.Wait()

	ok := 0
	for _, c := range codes {
		if c == 0 {
			ok++
		}
	}
	if ok != 1 {
		t.Fatalf("want exactly one start to win, codes=%v", codes)
	}

	f, err := rt.Store.Read(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	live := 0
	for _, in := range f.Instances {
		if in.Live {
			live++
		}
	}
	if live != 1 {
		t.Fatalf("want exactly one live instance, got %d: %+v", live, f.Instances)
	}
	if len(f.Instances) != 1 {
		t.Fatalf("loser must leave no record: %+v", f.Instances)
	}
	_ = state.PhaseRunning
}
