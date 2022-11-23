package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/github/spokes-receive-pack/internal/receivepack"
	"github.com/github/spokes-receive-pack/internal/spokes"
)

const GitSockstatVarSpokesQuarantine = "GIT_SOCKSTAT_VAR_spokes_quarantine"

func main() {
	args := os.Args[1:]
	if len(args) > 1 {
		fmt.Fprintf(os.Stderr, "unexpected number (%d) of arguments: only one argument should be passed", len(args))
		os.Exit(1)
	}

	if err := mainImpl(os.Stdin, os.Stdout, os.Stderr, args); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl(stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	ctx := context.Background()
	if os.Getenv(GitSockstatVarSpokesQuarantine) != "true" {
		rp := receivepack.NewReceivePack(stdin, stdout, stderr, args)
		if err := rp.Execute(ctx); err != nil {
			return fmt.Errorf("unexpected error running receive pack: %w", err)
		}
	} else {
		rp := spokes.NewSpokesReceivePack(stdin, stdout, stderr, args[0])
		if err := rp.Execute(ctx); err != nil {
			return fmt.Errorf("unexpected error running spokes receive pack: %w", err)
		}
	}

	return nil
}
