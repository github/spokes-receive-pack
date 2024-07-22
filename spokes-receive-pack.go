package main

import (
	"context"
	"fmt"
	"io"
	"os"

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
	return spokes.Exec(ctx, stdin, stdout, stderr, args, BuildVersion)
}
