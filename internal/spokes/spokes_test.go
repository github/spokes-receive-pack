package spokes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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
	require.NoError(t, os.Chdir("testdata/lots-of-refs.git"))
	t.Cleanup(func() { _ = os.Chdir("../..") })

	var buf bytes.Buffer
	wd, _ := os.Getwd()
	r := &spokesReceivePack{
		config:       &config.Config{},
		output:       &buf,
		repoPath:     wd,
		capabilities: "anything",
	}

	assert.NoError(t, r.performReferenceDiscovery(context.Background()))
	assert.Equal(t, expectedReferenceList, buf.String())
}
