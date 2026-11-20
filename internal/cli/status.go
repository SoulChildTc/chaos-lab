package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"text/tabwriter"
	"time"

	"github.com/soulchild/chaos-lab/internal/plugin"
	"github.com/soulchild/chaos-lab/internal/state"
)

// RunStatus prints live instances, converging any record orphaned by a killed
// framework process first.
func RunStatus(ctx context.Context, rt *Runtime, ref string, asJSON bool) (int, error) {
	var out []state.Instance
	if err := rt.Store.Update(ctx, func(f *state.File) error {
		Converge(f)
		for _, in := range f.Instances {
			if !in.Live {
				continue
			}
			if ref != "" && in.ID != ref && in.Plugin+"/"+in.Case != ref {
				continue
			}
			out = append(out, in)
		}
		return nil
	}); err != nil {
		return ExitUserError, err
	}

	if ref != "" && len(out) == 0 {
		return ExitUserError, fmt.Errorf("%w: no live instance matches %s", plugin.ErrUserInput, ref)
	}

	if asJSON {
		if out == nil {
			out = []state.Instance{}
		}
		return ExitOK, printJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("没有正在运行的实例")
		return ExitOK, nil
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ID\tPHASE\tLIVE\t已运行\t检查\t错误")
	for _, in := range out {
		errText := "-"
		if in.Error != nil {
			errText = *in.Error
		}
		fmt.Fprintf(w, "%s\t%s\t%t\t%s\t%d\t%s\n", in.ID, in.Phase, in.Live, shortDuration(time.Since(in.StartedAt)), in.Checks, errText)
	}
	return ExitOK, w.Flush()
}

var _ = json.Marshal
