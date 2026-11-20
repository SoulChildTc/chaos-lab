package cli

import (
	"fmt"
	"runtime"
)

// Injected at build time by the release workflow:
//
//	go build -ldflags "-X .../internal/cli.Version=v0.1.0 -X .../internal/cli.Commit=abc1234"
//
// A release binary carries no git information, so without this a downloaded
// binary cannot report what it is.
var (
	Version = "dev"
	Commit  = "none"
	Date    = "unknown"
)

// RunVersion prints the build identity. Plain output for humans, JSON for
// anything that has to check it.
func RunVersion(asJSON bool) (int, error) {
	if asJSON {
		return ExitOK, printJSON(struct {
			Version string `json:"version"`
			Commit  string `json:"commit"`
			Date    string `json:"date"`
			OS      string `json:"os"`
			Arch    string `json:"arch"`
		}{Version, Commit, Date, runtime.GOOS, runtime.GOARCH})
	}
	fmt.Printf("chaoslab %s (commit %s, built %s, %s/%s)\n",
		Version, Commit, Date, runtime.GOOS, runtime.GOARCH)
	return ExitOK, nil
}
