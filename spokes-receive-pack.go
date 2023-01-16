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

var BuildVersion string

func main() {
	if err := mainImpl(os.Stdin, os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl(stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	ctx := context.Background()
	if os.Getenv(GitSockstatVarSpokesQuarantine) != "bool:true" {
		rp := receivepack.NewReceivePack(stdin, stdout, stderr, args)
		if err := rp.Execute(ctx); err != nil {
			return fmt.Errorf("unexpected error running receive pack: %w", err)
		}
	} else {
		rp, err := spokes.NewSpokesReceivePack(stdin, stdout, stderr, args, BuildVersion)
		if err != nil {
			return err
		}
		if err := rp.Execute(ctx); err != nil {
			return fmt.Errorf("unexpected error running spokes receive pack: %w", err)
		}
	}

	return nil
}
