package cli

import (
	"context"
	"errors"
	"fmt"
)

// exitError carries a code out of a cobra RunE. Cobra has no notion of exit
// codes, but every chaoslab command has one and scripts depend on them.
type exitError struct {
	code int
	err  error
}

func (e *exitError) Error() string {
	if e.err != nil {
		return e.err.Error()
	}
	return fmt.Sprintf("exit %d", e.code)
}

func (e *exitError) Unwrap() error { return e.err }

// fail returns an error that makes the process exit with code. An empty format
// means "exit non-zero, say nothing": the command already printed what it had to.
func fail(code int, format string, args ...any) error {
	if format == "" {
		return &exitError{code: code}
	}
	return &exitError{code: code, err: fmt.Errorf(format, args...)}
}

// failWith keeps an existing error so errors.Is still works, and attaches a code.
func failWith(code int, err error) error {
	if err == nil {
		return &exitError{code: code}
	}
	return &exitError{code: code, err: err}
}

// Run builds the command tree, executes it, and returns the process exit code
// plus the message to print on stderr.
func Run(ctx context.Context, rt *Runtime, argv []string) (int, error) {
	root := newRoot(rt)
	root.SetArgs(argv)
	err := root.ExecuteContext(ctx)

	var x *exitError
	if errors.As(err, &x) {
		if x.err == nil {
			return x.code, nil
		}
		return x.code, x.err
	}
	if err != nil {
		return ExitUserError, err
	}
	return ExitOK, nil
}
