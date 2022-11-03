package spokes

import (
	"context"
	"io"
)

// SpokesReceivePack is used to model our own impl of the git-receive-pack
type SpokesReceivePack struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
}

// NewSpokesReceivePack returns a pointer to a SpokesReceivePack executor
func NewSpokesReceivePack(stdin io.Reader, stdout, stderr io.Writer, args []string) *SpokesReceivePack {
	return &SpokesReceivePack{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		args:   args,
	}
}

// Execute executes our custom implementation
func (r *SpokesReceivePack) Execute(ctx context.Context) error {
	panic("Needs to be implemented")
}
