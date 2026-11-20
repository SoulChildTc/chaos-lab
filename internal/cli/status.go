package cli

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/soulchildtc/chaos-lab/internal/plugin"
	"github.com/soulchildtc/chaos-lab/internal/state"
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
			if ref != "" && in.ID != ref && in.Plugin+"/"+in.Case != ref && in.Plugin != ref {
				continue
			}
			out = append(out, in)
		}
		return nil
	}); err != nil {
		return ExitUserError, err
	}

	if ref != "" && len(out) == 0 {
		return ExitUserError, fmt.Errorf("%w: 没有匹配 %s 的存活实例", plugin.ErrUserInput, ref)
	}

	if asJSON {
		if out == nil {
			out = []state.Instance{}
		}
		return ExitOK, printJSON(out)
	}
	if len(out) == 0 {
		fmt.Println("没有正在运行的实例。用 chaoslab list 看看有哪些场景。")
		return ExitOK, nil
	}

	rows := make([][]string, 0, len(out))
	for _, in := range out {
		errText := "-"
		if in.Error != nil {
			errText = *in.Error
		}
		rows = append(rows, []string{
			in.ID,
			phaseLabel(in.Phase),
			yesNo(in.Live),
			shortDuration(time.Since(in.StartedAt)),
			fmt.Sprintf("%d", in.Checks),
			errText,
		})
	}

	header := []string{"ID", "阶段", "占用", "已运行", "检查", "错误"}
	fmt.Println(renderTable(header, rows))
	return ExitOK, nil
}

// phaseLabel translates the state machine for humans. The raw value stays in
// --json, which scripts depend on.
func phaseLabel(p state.Phase) string {
	switch p {
	case state.PhaseStarting:
		return "启动中"
	case state.PhaseRunning:
		return "运行中"
	case state.PhasePassed:
		return "已通过"
	case state.PhaseStopped:
		return "已停止"
	case state.PhaseDirty:
		return "未清理"
	}
	return string(p)
}

func yesNo(b bool) string {
	if b {
		return "是"
	}
	return "否"
}

// renderTable pads every column to the widest cell, counting display width
// rather than bytes. A CJK character occupies two terminal cells but three
// bytes, so byte-based padding (which text/tabwriter does) misaligns any table
// with Chinese headers.
func renderTable(header []string, rows [][]string) string {
	widths := make([]int, len(header))
	for i, h := range header {
		widths[i] = displayWidth(h)
	}
	for _, r := range rows {
		for i, c := range r {
			if w := displayWidth(c); w > widths[i] {
				widths[i] = w
			}
		}
	}

	var b strings.Builder
	writeRow := func(cells []string) {
		for i, c := range cells {
			b.WriteString(c)
			if i < len(cells)-1 {
				b.WriteString(strings.Repeat(" ", widths[i]-displayWidth(c)+2))
			}
		}
		b.WriteString("\n")
	}
	writeRow(header)
	for _, r := range rows {
		writeRow(r)
	}
	return strings.TrimRight(b.String(), "\n")
}

// displayWidth counts terminal cells: East Asian wide characters take two.
func displayWidth(s string) int {
	w := 0
	for _, r := range s {
		if isWide(r) {
			w += 2
		} else {
			w++
		}
	}
	return w
}

// isWide covers the CJK ranges chaoslab actually prints.
func isWide(r rune) bool {
	switch {
	case r >= 0x1100 && r <= 0x115F, // Hangul Jamo
		r >= 0x2E80 && r <= 0xA4CF, // CJK radicals .. Yi
		r >= 0xAC00 && r <= 0xD7A3, // Hangul syllables
		r >= 0xF900 && r <= 0xFAFF, // CJK compatibility ideographs
		r >= 0xFE30 && r <= 0xFE6F, // CJK compatibility forms
		r >= 0xFF00 && r <= 0xFF60, // fullwidth forms
		r >= 0xFFE0 && r <= 0xFFE6:
		return true
	}
	return false
}

var _ = os.Stdout

// DisplayWidth exposes the cell-width count so tests can assert alignment.
func DisplayWidth(s string) int { return displayWidth(s) }
