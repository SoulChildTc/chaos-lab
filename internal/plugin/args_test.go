package plugin_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/soulchild/chaos-lab/internal/plugin"
)

func TestMergeFillsDefaultsAndConverts(t *testing.T) {
	s := &plugin.Schema{Args: []plugin.Arg{
		{Name: "image", Type: "string", Default: "kindest/node:v1.31.0"},
		{Name: "replicas", Type: "int", Default: float64(1)},
		{Name: "fail-down", Type: "bool", Default: false},
	}}
	got, err := plugin.Merge(s, map[string]string{"replicas": "3", "fail-down": "true"})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if got["image"] != "kindest/node:v1.31.0" || got["replicas"] != 3 || got["fail-down"] != true {
		t.Fatalf("got=%v", got)
	}
}

func TestMergeRejectsUnknownKey(t *testing.T) {
	s := &plugin.Schema{Args: []plugin.Arg{{Name: "n", Type: "int"}}}
	_, err := plugin.Merge(s, map[string]string{"oops": "1"})
	if !errors.Is(err, plugin.ErrUserInput) || !strings.Contains(err.Error(), "oops") {
		t.Fatalf("want ErrUserInput naming the arg, got %v", err)
	}
}

func TestMergeRejectsBadType(t *testing.T) {
	s := &plugin.Schema{Args: []plugin.Arg{{Name: "n", Type: "int"}}}
	if _, err := plugin.Merge(s, map[string]string{"n": "abc"}); !errors.Is(err, plugin.ErrUserInput) {
		t.Fatalf("want ErrUserInput, got %v", err)
	}
}

func TestMergeLeavesOptionalArgAbsent(t *testing.T) {
	s := &plugin.Schema{Args: []plugin.Arg{{Name: "kubeconfig", Type: "string"}}}
	got, err := plugin.Merge(s, nil)
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if _, ok := got["kubeconfig"]; ok {
		t.Fatalf("optional arg without default must stay absent, got %v", got)
	}
}

func TestMergeAllowsExplicitEmptyString(t *testing.T) {
	s := &plugin.Schema{Args: []plugin.Arg{{Name: "kubeconfig", Type: "string"}}}
	got, err := plugin.Merge(s, map[string]string{"kubeconfig": ""})
	if err != nil {
		t.Fatalf("err=%v", err)
	}
	if v, ok := got["kubeconfig"]; !ok || v != "" {
		t.Fatalf("explicit empty string must be kept, got %v", got)
	}
}

func TestArgHelpLineShowsDefault(t *testing.T) {
	a := plugin.Arg{Name: "node-image", Type: "string", Default: "kindest/node:v1.31.0", Help: "节点镜像"}
	if got := a.HelpLine(); !strings.Contains(got, "--node-image") || !strings.Contains(got, "kindest/node:v1.31.0") || !strings.Contains(got, "节点镜像") {
		t.Fatalf("got=%q", got)
	}
}

func TestArgHelpLineOmitsDefaultWhenAbsent(t *testing.T) {
	a := plugin.Arg{Name: "kubeconfig", Type: "string", Help: "复用集群"}
	if got := a.HelpLine(); strings.Contains(got, "default") {
		t.Fatalf("got=%q", got)
	}
}
