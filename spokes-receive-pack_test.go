package main

import (
	"runtime"
	"testing"
)

func TestSetGOMAXPROCS(t *testing.T) {
	previous := runtime.GOMAXPROCS(0)
	t.Cleanup(func() {
		runtime.GOMAXPROCS(previous)
	})

	setGOMAXPROCS()

	if got := runtime.GOMAXPROCS(0); got != 1 {
		t.Fatalf("GOMAXPROCS() = %d, want 1", got)
	}
}
