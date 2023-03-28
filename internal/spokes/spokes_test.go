package spokes

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/stretchr/testify/assert"
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
// git -C internal/spokes/testdata/lots-of-refs.git for-each-ref --format='%(objectname) %(refname)' | ruby -ne 'printf "%04x%s", $_.size, $_'
const expectedReferenceList = `00396a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/heads/main
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-1
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-10
00406a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-100
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-11
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-12
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-13
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-14
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-15
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-16
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-17
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-18
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-19
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-2
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-20
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-21
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-22
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-23
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-24
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-25
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-26
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-27
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-28
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-29
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-3
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-30
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-31
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-32
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-33
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-34
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-35
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-36
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-37
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-38
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-39
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-4
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-40
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-41
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-42
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-43
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-44
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-45
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-46
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-47
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-48
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-49
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-5
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-50
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-51
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-52
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-53
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-54
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-55
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-56
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-57
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-58
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-59
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-6
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-60
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-61
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-62
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-63
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-64
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-65
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-66
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-67
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-68
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-69
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-7
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-70
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-71
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-72
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-73
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-74
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-75
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-76
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-77
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-78
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-79
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-8
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-80
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-81
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-82
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-83
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-84
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-85
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-86
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-87
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-88
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-89
003e6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-9
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-90
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-91
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-92
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-93
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-94
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-95
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-96
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-97
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-98
003f6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-99
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-1
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-10
005d6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-100
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-11
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-12
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-13
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-14
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-15
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-16
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-17
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-18
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-19
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-2
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-20
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-21
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-22
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-23
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-24
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-25
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-26
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-27
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-28
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-29
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-3
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-30
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-31
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-32
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-33
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-34
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-35
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-36
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-37
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-38
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-39
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-4
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-40
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-41
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-42
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-43
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-44
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-45
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-46
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-47
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-48
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-49
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-5
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-50
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-51
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-52
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-53
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-54
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-55
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-56
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-57
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-58
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-59
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-6
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-60
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-61
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-62
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-63
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-64
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-65
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-66
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-67
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-68
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-69
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-7
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-70
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-71
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-72
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-73
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-74
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-75
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-76
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-77
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-78
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-79
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-8
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-80
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-81
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-82
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-83
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-84
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-85
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-86
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-87
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-88
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-89
005b6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-9
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-90
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-91
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-92
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-93
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-94
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-95
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-96
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-97
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-98
005c6a9ee41101de417acd4db5b7a18b66a5e1b54496 refs/tags/tag-aaaa-lakdjsf-asdfjkasdklfj-asdkfj-99
`

func TestPerformReferenceDiscovery(t *testing.T) {
	os.Chdir("testdata/lots-of-refs.git")
	t.Cleanup(func() { os.Chdir("../..") })

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
