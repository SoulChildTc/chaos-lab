package cli_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// `rm` is an alias for stop: same behaviour, shorter to type.
func TestRmIsAliasForStop(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.RunStart(context.Background(), rt, "fake", nil, true); code != 0 {
		t.Fatalf("start: %d %v", code, err)
	}
	code, err := cli.Run(context.Background(), rt, []string{"rm", "fake"})
	if code != 0 || err != nil {
		t.Fatalf("rm: code=%d err=%v", code, err)
	}
	f, _ := rt.Store.Read(context.Background())
	if f.Instances[0].Live {
		t.Fatalf("rm must release the instance: %+v", f.Instances[0])
	}
}

func TestRmSupportsAllAndJSON(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cli.RunStart(context.Background(), rt, "fake", nil, true)

	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"--json", "rm", "fake"})
	})
	if err != nil {
		t.Fatal(err)
	}
	var got struct {
		Live     bool `json:"live"`
		Released bool `json:"released"`
	}
	if err := json.Unmarshal(out, &got); err != nil {
		t.Fatalf("not JSON: %q (%v)", out, err)
	}
	if got.Live || !got.Released {
		t.Fatalf("%+v", got)
	}

	cli.RunStart(context.Background(), rt, "fake", nil, true)
	if code, err := cli.Run(context.Background(), rt, []string{"rm", "--all"}); code != 0 || err != nil {
		t.Fatalf("rm --all: code=%d err=%v", code, err)
	}
}

// rm must reject the same bad input stop does, not silently do nothing.
func TestRmRejectsMissingRef(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.Run(context.Background(), rt, []string{"rm"}); code != 1 || err == nil {
		t.Fatalf("code=%d err=%v", code, err)
	}
}
