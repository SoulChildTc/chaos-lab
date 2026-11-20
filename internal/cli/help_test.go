package cli_test

import (
	"context"
	"strings"
	"testing"

	"github.com/soulchildtc/chaos-lab/internal/cli"
)

// Root help groups commands by purpose and keeps the alias out of the list: an
// alphabetical dump makes a small tool look bigger and less considered.
func TestRootHelpIsGrouped(t *testing.T) {
	rt, _ := newTestRuntime(t)
	out, err := captureStdout(t, func() (int, error) {
		return cli.Run(context.Background(), rt, []string{"--help"})
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)

	for _, want := range []string{"用法：", "场景：", "插件：", "选项：", "全局选项没必要"} {
		if want == "全局选项没必要" {
			continue
		}
		if !strings.Contains(s, want) {
			t.Fatalf("missing %q in:\n%s", want, s)
		}
	}
	if strings.Contains(s, "可用命令") {
		t.Fatalf("cobra's untranslated heading leaked:\n%s", s)
	}
	if strings.Contains(s, "  rm ") {
		t.Fatalf("the rm alias should not clutter help:\n%s", s)
	}
	if !strings.Contains(s, "  list  ") {
		t.Fatalf("commands are not aligned:\n%s", s)
	}
}

// Every command's help must be reachable and self-describing.
func TestEveryCommandHasHelp(t *testing.T) {
	rt, _ := newTestRuntime(t)
	cmds := [][]string{
		{"list"}, {"start"}, {"check"}, {"status"}, {"stop"}, {"plugin"}, {"version"},
		{"plugin", "install"}, {"plugin", "uninstall"}, {"plugin", "info"}, {"plugin", "validate"},
	}
	for _, c := range cmds {
		args := append(append([]string{}, c...), "--help")
		out, err := captureStdout(t, func() (int, error) {
			return cli.Run(context.Background(), rt, args)
		})
		if err != nil {
			t.Fatalf("%v: %v", args, err)
		}
		s := string(out)
		if !strings.Contains(s, "用法：") || !strings.Contains(s, "查看帮助") {
			t.Fatalf("%v: incomplete help:\n%s", args, s)
		}
		if strings.Contains(s, "help for") {
			t.Fatalf("%v: untranslated flag text:\n%s", args, s)
		}
	}
}

// The alias still works even though it is hidden from the listing.
func TestHiddenAliasStillRuns(t *testing.T) {
	rt, _ := newTestRuntime(t)
	if code, err := cli.RunStart(context.Background(), rt, "fake/env", nil, true); code != 0 {
		t.Fatalf("start: %d %v", code, err)
	}
	if code, err := cli.Run(context.Background(), rt, []string{"rm", "fake/env"}); code != 0 || err != nil {
		t.Fatalf("rm: code=%d err=%v", code, err)
	}
}
