package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// The header must line up with the rows. The header is often the widest cell in
// a column ("PLUGIN" vs "k8s"), so it has to count towards the column width.
func TestListColumnsAlignWithHeader(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"list"})
	})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimRight(string(out), "\n"), "\n")
	if len(lines) < 3 {
		t.Fatalf("want header + rows, got %q", out)
	}

	// The FAULT column starts where the DESCRIPTION heading starts.
	offsets := map[int]bool{}
	for _, line := range lines {
		idx := strings.Index(line, "DESCRIPTION")
		if idx < 0 {
			idx = strings.Index(line, "访问站点返回 502")
		}
		if idx < 0 {
			idx = strings.Index(line, "Pod 一直 Pending")
		}
		if idx < 0 {
			continue
		}
		offsets[cli.DisplayWidth(line[:idx])] = true
	}
	if len(offsets) != 1 {
		t.Fatalf("description column misaligned, offsets=%v\n%s", offsets, out)
	}
}
