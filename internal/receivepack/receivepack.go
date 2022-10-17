package receivepack

import (
	"context"
	"fmt"
	"io"
	"os/exec"
)

// ReceivePack is used to model a receive-pack executor
type ReceivePack struct {
	stdin  io.Reader
	stdout io.Writer
	stderr io.Writer
	args   []string
}

// NewReceivePack returns a pointer to a ReceivePack executor
func NewReceivePack(stdin io.Reader, stdout, stderr io.Writer, args []string) *ReceivePack {
	return &ReceivePack{
		stdin:  stdin,
		stdout: stdout,
		stderr: stderr,
		args:   args,
	}
}

// Execute executes the git-receive-pack program spawning the actual Git process
func (r *ReceivePack) Execute(ctx context.Context) error {
	cmd := exec.CommandContext(ctx, "git-receive-pack", r.args...)
	cmd.Stdin = r.stdin
	cmd.Stdout = r.stdout
	cmd.Stderr = r.stderr

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unexpected error executing the git-receive-pack Git command: %w", err)
	}

	return nil
}
