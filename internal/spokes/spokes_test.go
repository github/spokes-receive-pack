package spokes

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCheckHiddenRefs(t *testing.T) {
	hiddenRefs := []string{"refs/pull/", "refs/gh/", "refs/__gh__"}
	for _, p := range []struct {
		line       string
		hiddenRefs []string
		expected   bool
	}{
		{"refs/heads/add-testify-framework", hiddenRefs, false},
		{"refs/heads/advertise-capabilities", hiddenRefs, false},
		{"refs/heads/initial-version", hiddenRefs, false},
		{"refs/heads/introduce-custom-mode", hiddenRefs, false},
		{"refs/heads/main", hiddenRefs, false},
		{"refs/heads/read-commands-phase", hiddenRefs, false},
		{"refs/remotes/origin/HEAD", hiddenRefs, false},
		{"refs/remotes/origin/add-testify-framework", hiddenRefs, false},
		{"refs/remotes/origin/advertise-capabilities", hiddenRefs, false},
		{"refs/remotes/origin/initial-version", hiddenRefs, false},
		{"refs/remotes/origin/introduce-custom-mode", hiddenRefs, false},
		{"refs/remotes/origin/main", hiddenRefs, false},
		{"refs/__gh__/pull/99986/rebase", hiddenRefs, true},
		{"refs/gh/merge_queue/156066/6e33e3a2c52017bec941ffd6f15c20a1ae002ad9", hiddenRefs, true},
		{"refs/pull/95628/head", hiddenRefs, true},
	} {
		t.Run(
			fmt.Sprintf("TestCheckHiddenRefs(%q, %q)", p.line, p.hiddenRefs),
			func(t *testing.T) {
				ok := isHiddenRef(p.line, p.hiddenRefs)
				assert.Equal(t, p.expected, ok)
			},
		)
	}
}
