package spokes

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/github/spokes-receive-pack/internal/sockstat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingWriter struct {
	w      io.Writer
	writes int
}

func (w *countingWriter) Write(p []byte) (int, error) {
	w.writes++
	return w.w.Write(p)
}

func TestCheckHiddenRefs(t *testing.T) {
	hiddenRefs := []string{"refs/pull/", "refs/gh/", "refs/__gh__", "!refs/__gh__/svn"}
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
		{"refs/__gh__/svn/branch-1", hiddenRefs, false},
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

// Generate like this:
// git -C internal/spokes/testdata/lots-of-refs.git for-each-ref --format='%(objectname) %(refname)' | ruby -ne 'printf "%04x%s", 4+$_.size, $_'
// then add capabilities to the first line and a 0000 at the end
const expectedReferenceList = `00466a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/heads/main` + "\x00" + `anything
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-1
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-10
00446a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-100
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-11
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-12
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-13
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-14
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-15
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-16
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-17
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-18
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-19
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-2
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-20
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-21
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-22
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-23
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-24
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-25
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-26
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-27
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-28
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-29
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-3
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-30
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-31
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-32
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-33
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-34
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-35
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-36
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-37
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-38
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-39
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-4
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-40
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-41
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-42
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-43
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-44
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-45
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-46
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-47
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-48
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-49
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-5
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-50
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-51
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-52
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-53
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-54
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-55
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-56
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-57
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-58
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-59
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-6
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-60
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-61
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-62
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-63
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-64
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-65
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-66
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-67
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-68
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-69
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-7
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-70
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-71
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-72
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-73
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-74
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-75
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-76
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-77
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-78
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-79
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-8
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-80
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-81
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-82
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-83
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-84
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-85
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-86
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-87
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-88
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-89
00426a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-9
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-90
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-91
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-92
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-93
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-94
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-95
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-96
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-97
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-98
00436a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-99
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-1
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-10
00616a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-100
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-11
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-12
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-13
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-14
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-15
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-16
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-17
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-18
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-19
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-2
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-20
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-21
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-22
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-23
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-24
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-25
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-26
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-27
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-28
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-29
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-3
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-30
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-31
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-32
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-33
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-34
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-35
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-36
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-37
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-38
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-39
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-4
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-40
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-41
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-42
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-43
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-44
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-45
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-46
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-47
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-48
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-49
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-5
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-50
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-51
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-52
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-53
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-54
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-55
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-56
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-57
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-58
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-59
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-6
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-60
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-61
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-62
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-63
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-64
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-65
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-66
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-67
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-68
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-69
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-7
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-70
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-71
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-72
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-73
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-74
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-75
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-76
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-77
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-78
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-79
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-8
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-80
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-81
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-82
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-83
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-84
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-85
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-86
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-87
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-88
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-89
005f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-9
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-90
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-91
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-92
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-93
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-94
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-95
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-96
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-97
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-98
00606a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-99
0000`

func TestPerformReferenceDiscovery(t *testing.T) {
	// spokesReceivePack assumes that we've already done a chdir into the repo.
	origwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir("testdata/lots-of-refs.git"))
	t.Cleanup(func() { _ = os.Chdir(origwd) })

	var buf bytes.Buffer
	output := &countingWriter{w: &buf}
	wd, _ := os.Getwd()
	r := &spokesReceivePack{
		config:       &config.Config{},
		output:       output,
		repoPath:     wd,
		capabilities: "anything",
	}

	assert.NoError(t, r.performBufferedReferenceDiscovery(context.Background()))
	assert.Equal(t, expectedReferenceList, buf.String())
	assert.Equal(t, 1, output.writes)
}

func TestPerformBufferedReferenceDiscoveryWithUnhiddenRefs(t *testing.T) {
	origwd, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir("testdata/lots-of-refs.git"))
	t.Cleanup(func() { _ = os.Chdir(origwd) })

	var buf bytes.Buffer
	output := &countingWriter{w: &buf}
	wd, _ := os.Getwd()
	r := &spokesReceivePack{
		config: &config.Config{Entries: []config.ConfigEntry{
			{Key: "transfer.hiderefs", Value: "refs/tags/"},
			{Key: "transfer.hiderefs", Value: "!refs/tags/"},
		}},
		output:       output,
		repoPath:     wd,
		capabilities: "anything",
	}

	assert.NoError(t, r.performBufferedReferenceDiscovery(context.Background()))
	assert.Equal(t, expectedReferenceList, buf.String())
	assert.Equal(t, 1, output.writes)
}

func TestPerformCheckConnectivitySkipsDeletesAndRejected(t *testing.T) {
	dir := t.TempDir()
	run := func(stdin string, args ...string) string {
		t.Helper()
		return runGitInput(t, "", nil, stdin, append([]string{"--git-dir=" + dir}, args...)...)
	}
	run("", "init", "--bare")
	tree := run("", "mktree")
	base := run("", "commit-tree", "-m", "base", tree)
	run("", "update-ref", "refs/heads/main", base)
	run("", "update-ref", "refs/heads/gone", base)
	pushed := run("", "commit-tree", "-p", base, "-m", "pushed", tree)
	disconnected := run(fmt.Sprintf("tree %s\nparent %s\nauthor a <a@example.com> 0 +0000\ncommitter a <a@example.com> 0 +0000\n\ndisconnected\n",
		tree, "1234567890123456789012345678901234567890"), "hash-object", "-t", "commit", "-w", "--stdin")
	t.Setenv("GIT_DIR", dir)

	r := &spokesReceivePack{repoPath: dir, quarantineFolder: t.TempDir()}
	deleteAndRejected := []command{
		{refname: "refs/heads/gone", oldOID: base, newOID: nullSHA1OID},
		{refname: "refs/heads/hidden", oldOID: nullSHA1OID, newOID: "1234567890123456789012345678901234567890", err: "deny updating a hidden ref"},
	}
	assert.NoError(t, r.performCheckConnectivity(context.Background(), append([]command{
		{refname: "refs/heads/main", oldOID: base, newOID: pushed},
	}, deleteAndRejected...)))
	assert.Error(t, r.performCheckConnectivity(context.Background(), append([]command{
		{refname: "refs/heads/main", oldOID: base, newOID: disconnected},
	}, deleteAndRejected...)))
}

func TestConnectivityStopperArgs(t *testing.T) {
	origwd, err := os.Getwd()
	require.NoError(t, err)
	// A valid quarantine directory is required so that
	// getAlternateObjectDirsEnv produces well-formed environment
	// variables (an empty GIT_OBJECT_DIRECTORY would make git refuse
	// to operate).
	quarantine := t.TempDir()

	t.Run("flag off uses legacy stoppers", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "")
		// No filesystem operations are required: the legacy path
		// never invokes git.
		r := &spokesReceivePack{}
		got := r.connectivityStopperArgs(context.Background())
		assert.Equal(t, []string{"--exclude-hidden=receive", "--all", "--alternate-refs"}, got)
	})

	t.Run("flag on uses --exclude=*/* --branches HEAD when HEAD resolves", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		require.NoError(t, os.Chdir("testdata/lots-of-refs.git"))
		t.Cleanup(func() { _ = os.Chdir(origwd) })
		wd, _ := os.Getwd()
		r := &spokesReceivePack{repoPath: wd, quarantineFolder: quarantine}
		got := r.connectivityStopperArgs(context.Background())
		assert.Equal(t, []string{"--exclude=*/*", "--branches", "HEAD"}, got)
	})

	t.Run("flag on omits HEAD when HEAD does not resolve", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		// A fresh repo with no commits has HEAD pointing at an
		// unborn branch, so rev-parse --verify HEAD fails.  Use a
		// non-bare repo to side-step any safe.bareRepository
		// configuration the caller may have set.
		dir := t.TempDir()
		require.NoError(t, exec.Command("git", "init", dir).Run())
		require.NoError(t, os.Chdir(dir))
		t.Cleanup(func() { _ = os.Chdir(origwd) })
		r := &spokesReceivePack{
			repoPath:         filepath.Join(dir, ".git"),
			quarantineFolder: quarantine,
		}
		got := r.connectivityStopperArgs(context.Background())
		assert.Equal(t, []string{"--exclude=*/*", "--branches"}, got)
	})
}

// stopperNetwork mirrors the fileserver layout: a fork using network.git as
// an alternate, with parent refs in network.git and the parent's HEAD in
// a separate sibling repository.
type stopperNetwork struct {
	fork, network string

	commits map[string]string
}

// newStopperNetwork gives every non-base commit a missing parent, so the
// connectivity tests succeed only when the intended stops are used.
func newStopperNetwork(t *testing.T) *stopperNetwork {
	t.Helper()
	dir := t.TempDir()
	n := &stopperNetwork{
		fork:    filepath.Join(dir, "fork.git"),
		network: filepath.Join(dir, "network.git"),
		commits: map[string]string{},
	}
	runGit(t, "", "init", "--bare", n.network)
	runGit(t, "", "init", "--bare", n.fork)
	parent := filepath.Join(dir, "100.git")
	runGit(t, "", "init", "--bare", parent)
	runGit(t, "", "--git-dir="+parent, "symbolic-ref", "HEAD", "refs/heads/main")

	commit := func(gitDir, name, ref string, parents ...string) {
		tree := runGit(t, "", "--git-dir="+gitDir, "mktree")
		var body strings.Builder
		fmt.Fprintf(&body, "tree %s\n", tree)
		for _, p := range parents {
			fmt.Fprintf(&body, "parent %s\n", p)
		}
		fmt.Fprintf(&body, "author a <a@example.com> 0 +0000\ncommitter a <a@example.com> 0 +0000\n\n%s\n", name)
		n.commits[name] = runGitInput(t, "", nil, body.String(), "--git-dir="+gitDir, "hash-object", "-t", "commit", "-w", "--stdin")
		runGit(t, "", "--git-dir="+gitDir, "update-ref", ref, n.commits[name])
	}
	missing := "1234567890123456789012345678901234567890"
	commit(n.fork, "base", "refs/heads/base")
	commit(n.fork, "topic", "refs/heads/user/topic", missing)
	commit(n.network, "main", "refs/remotes/100/heads/main", missing)
	commit(n.network, "second", "refs/remotes/100/heads/second", missing)
	commit(n.network, "nested", "refs/remotes/100/heads/user/nested", missing)
	commit(n.network, "other", "refs/remotes/200/heads/main", missing)

	require.NoError(t, os.WriteFile(
		filepath.Join(n.fork, "objects", "info", "alternates"),
		[]byte(filepath.Join(n.network, "objects")+"\n"), 0644))
	// GIT_DIR makes access explicit for safe.bareRepository=explicit.
	t.Setenv("GIT_DIR", n.fork)
	return n
}

func runGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	return runGitEnv(t, dir, nil, args...)
}

func runGitEnv(t *testing.T, dir string, env []string, args ...string) string {
	t.Helper()
	return runGitInput(t, dir, env, "", args...)
}

func runGitInput(t *testing.T, dir string, env []string, stdin string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=a", "GIT_AUTHOR_EMAIL=a@example.com",
		"GIT_COMMITTER_NAME=a", "GIT_COMMITTER_EMAIL=a@example.com")
	cmd.Env = append(cmd.Env, env...)
	cmd.Stdin = strings.NewReader(stdin)
	out, err := cmd.CombinedOutput()
	require.NoError(t, err, "git %v: %s", args, out)
	return string(bytes.TrimSpace(out))
}

func TestConnectivityStopperOIDs(t *testing.T) {
	n := newStopperNetwork(t)
	r := &spokesReceivePack{repoPath: n.fork}
	c := n.commits
	commands := []command{
		{refname: "refs/heads/user/topic", oldOID: c["topic"], newOID: c["main"]},
		{refname: "refs/heads/base", oldOID: c["nested"], newOID: c["main"]},
		{refname: "refs/heads/new", oldOID: nullSHA1OID, newOID: c["main"]},
	}

	t.Run("flag off adds nothing", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "uint:100")
		assert.Empty(t, r.connectivityStopperOIDs(context.Background(), commands))
	})

	t.Run("flag on adds the current values of refs being updated", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "")
		assert.Equal(t, []string{c["base"], c["topic"]}, r.connectivityStopperOIDs(context.Background(), commands))
	})

	t.Run("flag on adds a fork parent's default branch", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "uint:100")
		assert.Equal(t, []string{c["base"], c["topic"], c["main"]}, r.connectivityStopperOIDs(context.Background(), commands))
	})

	t.Run("a directory's refs don't match its name", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "")
		// for-each-ref lists refs/heads/user/topic for the pattern refs/heads/user.
		dir := []command{{refname: "refs/heads/user", oldOID: c["topic"], newOID: c["main"]}}
		assert.Empty(t, r.connectivityStopperOIDs(context.Background(), dir))
	})

	t.Run("a default branch name keeps trailing non-ASCII spaces", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "uint:100")
		branch := "main\u00a0"
		runGit(t, "", "--git-dir="+filepath.Join(filepath.Dir(n.fork), "100.git"), "symbolic-ref", "HEAD", "refs/heads/"+branch)
		runGit(t, "", "--git-dir="+n.network, "update-ref", "refs/remotes/100/heads/"+branch, c["second"])
		assert.Equal(t, []string{c["base"], c["topic"], c["second"]}, r.connectivityStopperOIDs(context.Background(), commands))
	})

	t.Run("a missing default branch adds nothing, even if its name parses as an object", func(t *testing.T) {
		t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
		t.Setenv(sockstat.Prefix+"parent_repo_id", "uint:100")
		runGit(t, "", "--git-dir="+filepath.Join(filepath.Dir(n.fork), "100.git"), "symbolic-ref", "HEAD", "refs/heads/x-g"+c["nested"][:12])
		assert.Equal(t, []string{c["base"], c["topic"]}, r.connectivityStopperOIDs(context.Background(), commands))
	})
}

func TestPerformCheckConnectivityWithStoppers(t *testing.T) {
	n := newStopperNetwork(t)
	t.Setenv(sockstat.Prefix+"spokes_receive_pack_narrow_connectivity_stopper", "bool:true")
	t.Setenv(sockstat.Prefix+"parent_repo_id", "uint:100")
	quarantine := t.TempDir()
	r := &spokesReceivePack{repoPath: n.fork, quarantineFolder: quarantine}
	pushEnv := []string{
		"GIT_OBJECT_DIRECTORY=" + quarantine,
		"GIT_ALTERNATE_OBJECT_DIRECTORIES=" + filepath.Join(n.fork, "objects"),
	}
	c := n.commits

	onMain := runGitEnv(t, "", pushEnv, "commit-tree", c["main"]+"^{tree}", "-p", c["main"], "-m", "on main")
	onTopic := runGitEnv(t, "", pushEnv, "commit-tree", c["topic"]+"^{tree}", "-p", c["topic"], "-m", "on topic")
	require.NoError(t, r.performCheckConnectivity(context.Background(), []command{
		{refname: "refs/heads/new", oldOID: nullSHA1OID, newOID: onMain},
		{refname: "refs/heads/user/topic", oldOID: c["topic"], newOID: onTopic},
	}))
	// A rename: the deleted ref's current value is the only stop near the new tip.
	require.NoError(t, r.performCheckConnectivity(context.Background(), []command{
		{refname: "refs/heads/user/renamed", oldOID: nullSHA1OID, newOID: onTopic},
		{refname: "refs/heads/user/topic", oldOID: c["topic"], newOID: nullSHA1OID},
	}))

	tree := runGit(t, "", "rev-parse", c["main"]+"^{tree}")
	missingParent := "1234567890123456789012345678901234567890"
	disconnectedCommit := fmt.Sprintf("tree %s\nparent %s\nauthor a <a@example.com> 0 +0000\ncommitter a <a@example.com> 0 +0000\n\ndisconnected\n", tree, missingParent)
	disconnected := runGitInput(t, "", pushEnv, disconnectedCommit, "hash-object", "-t", "commit", "-w", "--stdin")
	assert.Error(t, r.performCheckConnectivity(context.Background(), []command{
		{refname: "refs/heads/user/topic", oldOID: c["topic"], newOID: disconnected},
	}))
	// Client-chosen names must not make the pushed object its own stop, eg.
	// a name rev-list would read as git describe output for it.
	for _, name := range []string{disconnected, "refs/heads/x-g" + disconnected[:12]} {
		assert.Error(t, r.performCheckConnectivity(context.Background(), []command{
			{refname: "refs/heads/victim", oldOID: nullSHA1OID, newOID: disconnected},
			{refname: name, oldOID: c["topic"], newOID: nullSHA1OID},
		}), name)
	}
}
