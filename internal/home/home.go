// Package home resolves the single directory that holds every piece of local
// state. Tests redirect it with CHAOSLAB_HOME so they never touch ~/.lab.
package home

import (
	"os"
	"path/filepath"
)

// Dir returns $CHAOSLAB_HOME, or ~/.lab when unset.
func Dir() (string, error) {
	if d := os.Getenv("CHAOSLAB_HOME"); d != "" {
		return d, nil
	}
	uh, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(uh, ".lab"), nil
}

// Path joins elem onto Dir without creating anything.
func Path(elem ...string) (string, error) {
	d, err := Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(append([]string{d}, elem...)...), nil
}

// Ensure joins elem onto Dir and creates the directory.
func Ensure(elem ...string) (string, error) {
	p, err := Path(elem...)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(p, 0o755); err != nil {
		return "", err
	}
	return p, nil
}
