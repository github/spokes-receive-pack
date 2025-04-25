package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/github/spokes-receive-pack/internal/receivepack"
	"github.com/github/spokes-receive-pack/internal/sockstat"
	"github.com/github/spokes-receive-pack/internal/spokes"
)

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
	if !sockstat.GetBool("spokes_quarantine") {
		rp := receivepack.NewReceivePack(stdin, stdout, stderr, args)
		if err := rp.Execute(ctx); err != nil {
			return 1, fmt.Errorf("unexpected error running receive pack: %w", err)
		}

		return 0, nil
	}

	return spokes.Exec(ctx, stdin, stdout, stderr, args, BuildVersion)
}
