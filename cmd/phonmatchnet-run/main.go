package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/androiddrew/phonmatchnet-run/internal/cli"
)

// Exit codes match naptrap's detect command: 0 accepted, 1 rejected, 2 error.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	command := cli.NewCommand()
	command.SetIn(os.Stdin)
	command.SetOut(os.Stdout)
	command.SetErr(os.Stderr)
	err := command.ExecuteContext(ctx)
	switch {
	case err == nil:
	case errors.Is(err, cli.ErrRejected):
		os.Exit(1)
	default:
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(2)
	}
}
