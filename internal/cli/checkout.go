package cli

import (
	"encoding/json"
	"fmt"
	"strings"
)

// printPass hands over what the user earned: pass, root cause, then the clues
// that would have led there fastest.
func printPass(id string, report json.RawMessage) {
	var r struct {
		RootCause string   `json:"root_cause"`
		Clues     []string `json:"clues"`
	}
	if len(report) > 0 {
		_ = json.Unmarshal(report, &r)
	}

	fmt.Printf("%s %s %s\n\n", glyphOK(), ok("通过"), dim(id))
	if r.RootCause != "" {
		fmt.Println(head("根因"))
		for _, line := range wrap(r.RootCause, 74) {
			fmt.Println("  " + line)
		}
	}
	if len(r.Clues) > 0 {
		fmt.Println()
		fmt.Println(head("关键线索"))
		for _, c := range r.Clues {
			fmt.Println("  " + dim("•") + " " + c)
		}
	}
	fmt.Println()
	fmt.Println(dim("环境还在，可以进去复盘。清理：chaoslab rm " + id))
}

// printNotFixed keeps the feedback loop legible while the fault is unfixed.
func printNotFixed(id, message string) {
	if message == "" {
		message = "还没修好"
	}
	fmt.Printf("%s %s %s\n", glyphPending(), warn(message), dim(id))
}

// wrap breaks text into lines of at most width runes, preferring spaces.
func wrap(s string, width int) []string {
	var out []string
	for _, paragraph := range strings.Split(s, "\n") {
		line := ""
		for _, word := range strings.Fields(paragraph) {
			candidate := word
			if line != "" {
				candidate = line + " " + word
			}
			if displayWidth(candidate) > width && line != "" {
				out = append(out, line)
				line = word
				continue
			}
			line = candidate
		}
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}
