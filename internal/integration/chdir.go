//go:build integration

package integration

import (
	"os"
	"testing"
)

// chdir calls os.Chdir and registers a cleanup func to change back to the original directory.
// This is a test helper that should be used in place of os.Chdir in tests.
func chdir(t *testing.T, dir string) error {
	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	t.Logf("chdir %q", dir)
	if err := os.Chdir(dir); err != nil {
		return err
	}

	t.Cleanup(func() {
		// If this fails, it might be because it's changing back to a
		// tmpdir that got removed. In that case, we'll just log it,
		// and trust that there's another cleanup func ready to change
		// back to the original working dir.
		t.Logf("cleanup chdir %q", wd)
		if err := os.Chdir(wd); err != nil {
			t.Logf("error calling chdir(%q) during cleanup: %v", wd, err)
		}
	})

	return nil
}
