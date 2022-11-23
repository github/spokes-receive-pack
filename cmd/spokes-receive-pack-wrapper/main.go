//go:build integration

package main

import (
	"fmt"
	"os"
	"os/exec"
)

// This little program is only a function used to pass the required environment to the actual `spokes-receive-pack`
// binary during our integration tests
func main() {
	command := exec.Command("spokes-receive-pack", os.Args[1:]...)
	command.Env = append(
		os.Environ(),
		"GIT_SOCKSTAT_VAR_spokes_quarantine=true",
		"GIT_SOCKSTAT_VAR_quarantine_dir=objects/quarantine",
	)
	command.Stdout = os.Stdout
	command.Stdin = os.Stdin
	command.Stderr = os.Stderr

	if err := command.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error running the spokes-receive-pack binary. Error: %s", err.Error())
		os.Exit(1)
	}
}
