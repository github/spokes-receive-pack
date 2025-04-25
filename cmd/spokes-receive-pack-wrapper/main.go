//go:build integration

package main

import (
	"fmt"
	"os"

	"github.com/github/spokes-receive-pack/internal/integration"
)

// This little program is only a function used to pass the required environment to the actual `spokes-receive-pack`
// binary during our integration tests
func main() {
	env := []string{
		"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
		"GIT_SOCKSTAT_VAR_quarantine_id=test_quarantine_id",
	}
	if err := integration.RunMain(env); err != nil {
		fmt.Fprintf(os.Stderr, "unexpected error running the spokes-receive-pack binary. Error: %s", err.Error())
		os.Exit(1)
	}
}
