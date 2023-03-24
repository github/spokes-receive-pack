package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	"github.com/github/spokes-receive-pack/internal/governor"
	"github.com/github/spokes-receive-pack/internal/receivepack"
	"github.com/github/spokes-receive-pack/internal/spokes"
)

const GitSockstatVarSpokesQuarantine = "GIT_SOCKSTAT_VAR_spokes_quarantine"

var BuildVersion string

func main() {
	exitCode, err := mainImpl(os.Stdin, os.Stdout, os.Stderr, os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
	}
	os.Exit(exitCode)
}

func mainImpl(stdin io.Reader, stdout, stderr io.Writer, args []string) (int, error) {
	ctx := context.Background()
	if os.Getenv(GitSockstatVarSpokesQuarantine) != "bool:true" {
		rp := receivepack.NewReceivePack(stdin, stdout, stderr, args)
		if err := rp.Execute(ctx); err != nil {
			return 1, fmt.Errorf("unexpected error running receive pack: %w", err)
		}

		return 0, nil
	}

	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	rp, err := spokes.NewSpokesReceivePack(stdin, stdout, stderr, args, BuildVersion)
	if err != nil {
		return 1, err
	}

	g, err := governor.Start(ctx, rp.RepoPath)
	if err != nil {
		return 75, err
	}
	defer g.Finish(ctx)

	if err := rp.Execute(ctx, g); err != nil {
		g.SetError(1, err.Error())
		return 1, fmt.Errorf("unexpected error running spokes receive pack: %w", err)
	}

	return 0, nil
}
