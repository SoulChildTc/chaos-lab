package cli

import (
	"context"

	"github.com/soulchild/chaos-lab/internal/plugin"
)

// runProc is a seam so tests could intercept child processes if needed.
func runProc(ctx context.Context, dir string, env []string, name string, args ...string) (int, error) {
	_, code, err := plugin.Run(ctx, dir, env, name, args...)
	return code, err
}
