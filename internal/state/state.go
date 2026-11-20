// Package state owns the one file that records instances. Every read and write
// goes through a file lock, and every write is atomic, because the CLI may run
// concurrently and a half-written file would orphan real environments.
package state

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
	"time"
)

// Phase is the human-facing stage of an instance. It is deliberately separate
// from Live: a passed instance still holds its environment until it is stopped.
type Phase string

const (
	PhaseStarting Phase = "starting"
	PhaseRunning  Phase = "running"
	PhasePassed   Phase = "passed"
	PhaseStopped  Phase = "stopped"
	PhaseDirty    Phase = "dirty"
)

// Instance is one running plugin environment. Its scope is the whole plugin:
// a plugin decides which faults it injects and how they share an environment.
type Instance struct {
	ID        string         `json:"id"`
	Plugin    string         `json:"plugin"`
	Slug      string         `json:"slug"`
	Args      map[string]any `json:"args,omitempty"`
	Claims    []string       `json:"claims,omitempty"`
	Phase     Phase          `json:"phase"`
	Live      bool           `json:"live"`
	PID       int            `json:"pid,omitempty"`
	StartedAt time.Time      `json:"started_at"`
	Checks    int            `json:"checks"`
	LastCheck *time.Time     `json:"last_check,omitempty"`
	Error     *string        `json:"error,omitempty"`
	// RunPath is where this instance's run record lives, so later events update
	// the same file instead of writing a second one.
	RunPath string `json:"run_path,omitempty"`
}

type File struct {
	Seq       map[string]int `json:"seq"`
	Instances []Instance     `json:"instances"`
}

type Store struct {
	Path string
}

func Open(path string) *Store { return &Store{Path: path} }

// Read loads the file without taking the lock. Callers that will write must use
// Update instead so the read and the write are one transaction.
func (s *Store) Read(ctx context.Context) (*File, error) {
	return s.load()
}

func (s *Store) load() (*File, error) {
	b, err := os.ReadFile(s.Path)
	if os.IsNotExist(err) {
		return &File{Seq: map[string]int{}}, nil
	}
	if err != nil {
		return nil, err
	}
	var f File
	if err := json.Unmarshal(b, &f); err != nil {
		// Never silently reset: the instances listed in this file own real
		// environments, and dropping them would orphan those environments.
		return nil, fmt.Errorf("state file %s is not valid JSON: %w", s.Path, err)
	}
	if f.Seq == nil {
		f.Seq = map[string]int{}
	}
	return &f, nil
}

// Update runs fn under an exclusive lock and persists the result atomically. If
// fn returns an error nothing is written.
func (s *Store) Update(ctx context.Context, fn func(*File) error) error {
	if err := os.MkdirAll(filepath.Dir(s.Path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(s.Path+".lock", os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err := syscall.Flock(int(lock.Fd()), syscall.LOCK_EX); err != nil {
		return err
	}
	defer syscall.Flock(int(lock.Fd()), syscall.LOCK_UN)

	f, err := s.load()
	if err != nil {
		return err
	}
	if err := fn(f); err != nil {
		return err
	}
	return s.write(f)
}

func (s *Store) write(f *File) error {
	b, err := json.MarshalIndent(f, "", "  ")
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.Path), ".instances-*.tmp")
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(name, s.Path)
}
