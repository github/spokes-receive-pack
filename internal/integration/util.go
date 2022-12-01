package integration

import (
	"fmt"
	"os"
	"os/exec"
)

func RunMain(env []string) error {
	command := exec.Command("spokes-receive-pack", os.Args[1:]...)
	command.Env = append(
		os.Environ(),
		env...,
	)
	command.Stdout = os.Stdout
	command.Stdin = os.Stdin
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error running the spokes-receive-pack binary. Error: %s", err.Error())
		return err
	}

	return nil
}
