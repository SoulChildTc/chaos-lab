package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/soulchild/chaos-lab/internal/runlog"
	"github.com/soulchild/chaos-lab/internal/state"
)

// runPath is where this instance's single run record lives. It is stored in the
// instance so later events update the same file instead of writing a second one.
func runPath(rt *Runtime, inst state.Instance) string {
	return filepath.Join(rt.Home, "runs",
		fmt.Sprintf("%s_%s.json", inst.StartedAt.Format("20060102T150405Z"), inst.Slug))
}

// writeRunRecord writes the record for an instance, or updates it when one
// already exists (a pass is recorded first, then a later stop updates it).
func writeRunRecord(rt *Runtime, inst state.Instance, result string, report []byte, released *bool) {
	path := inst.RunPath
	if path == "" {
		path = runPath(rt, inst)
	}
	if _, err := os.Stat(path); err == nil {
		_ = runlog.Update(path, func(r *runlog.Record) {
			if result != "" {
				r.Result = result
			}
			r.EndedAt = time.Now().UTC()
			r.DurationSec = int(r.EndedAt.Sub(r.StartedAt).Seconds())
			if len(report) > 0 {
				r.Report = report
			}
			if released != nil {
				r.ClaimsReleased = released
			}
		})
		return
	}
	end := time.Now().UTC()
	_ = runlog.Write(path, runlog.Record{
		Instance:       inst.ID,
		Plugin:         inst.Plugin,
		Case:           inst.Case,
		Args:           inst.Args,
		StartedAt:      inst.StartedAt,
		EndedAt:        end,
		DurationSec:    int(end.Sub(inst.StartedAt).Seconds()),
		Result:         result,
		Checks:         inst.Checks,
		Report:         report,
		ClaimsReleased: released,
	})
}

func writeRunDirEnsure(rt *Runtime) error {
	_, err := writeRunDir(rt)
	return err
}
