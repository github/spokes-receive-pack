package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/github/spokes-receive-pack/internal/receivepack"
)

func main() {
	if err := mainImpl(os.Stdin, os.Stdout, os.Stderr, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}
}

func mainImpl(stdin io.Reader, stdout, stderr io.Writer, args []string) error {
	ctx := context.Background()
	rp := receivepack.NewReceivePack(stdin, stdout, stderr, args)

	if err := rp.Execute(ctx); err != nil {
		return fmt.Errorf("unexpected error running receive pack: %w", err)
	}

	return nil
}
