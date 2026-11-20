package plugin

import (
	"errors"
	"fmt"
	"sort"
	"strconv"
)

// ErrUserInput marks a mistake the user can fix (unknown flag, wrong type),
// as opposed to a broken plugin. Callers map it to exit code 1.
var ErrUserInput = errors.New("用法错误")

// Merge applies user-supplied key/value pairs on top of the schema defaults and
// converts them to the declared types. An Arg with no Default stays absent
// unless the user passes it, so "unset" and "set to empty" stay distinguishable.
func Merge(s *Schema, kv map[string]string) (map[string]any, error) {
	byName := make(map[string]Arg, len(s.Args))
	for _, a := range s.Args {
		byName[a.Name] = a
	}

	unknown := make([]string, 0)
	for k := range kv {
		if _, ok := byName[k]; !ok {
			unknown = append(unknown, k)
		}
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		return nil, fmt.Errorf("%w: unknown argument %q", ErrUserInput, unknown[0])
	}

	out := make(map[string]any, len(s.Args))
	for _, a := range s.Args {
		if a.Default != nil {
			out[a.Name] = coerceDefault(a)
		}
	}
	for k, raw := range kv {
		v, err := coerce(byName[k], raw)
		if err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, nil
}

func coerce(a Arg, raw string) (any, error) {
	switch a.Type {
	case "int":
		n, err := strconv.Atoi(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: --%s wants int, got %q", ErrUserInput, a.Name, raw)
		}
		return n, nil
	case "bool":
		b, err := strconv.ParseBool(raw)
		if err != nil {
			return nil, fmt.Errorf("%w: --%s wants bool, got %q", ErrUserInput, a.Name, raw)
		}
		return b, nil
	default:
		return raw, nil
	}
}

// coerceDefault normalises JSON-decoded defaults (float64 for numbers) to the
// same Go types coerce produces, so downstream code sees one shape.
func coerceDefault(a Arg) any {
	if n, ok := a.Default.(float64); ok && a.Type == "int" {
		return int(n)
	}
	return a.Default
}

// HelpLine renders one line for `start --help`.
func (a Arg) HelpLine() string {
	return "  " + a.FlagName() + "  " + a.HelpSummary()
}

// FlagName is the flag as written on the command line.
func (a Arg) FlagName() string { return fmt.Sprintf("--%s=<%s>", a.Name, a.Type) }

// HelpSummary is the description, with the default appended when there is one.
func (a Arg) HelpSummary() string {
	s := a.Help
	if a.Default != nil {
		if s != "" {
			s += " "
		}
		s += fmt.Sprintf("(默认 %v)", a.Default)
	}
	return s
}
