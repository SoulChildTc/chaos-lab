// Command chaoslab runs local fault-drill scenarios. It is a pure orchestrator:
// every environment is created and destroyed by a plugin, never by this binary.
package main

import (
	"context"
	"fmt"
	"os"

	"github.com/soulchild/chaos-lab/internal/cli"
)

func main() {
	rt, err := cli.NewRuntimeDefault()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(cli.ExitUserError)
	}
	code, err := cli.Run(context.Background(), rt, os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
	}
	os.Exit(code)
}
