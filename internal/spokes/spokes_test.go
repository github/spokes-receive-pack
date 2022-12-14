package spokes

import (
	"fmt"
	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/stretchr/testify/assert"
	"testing"
)

func TestCheckHiddenRefs(t *testing.T) {
	hiddenRefs := []config.ConfigEntry{
		{
			Key:   "receive.hiderefs",
			Value: "refs/pull/",
		},
		{
			Key:   "receive.hiderefs",
			Value: "refs/gh/",
		},
		{
			Key:   "receive.hiderefs",
			Value: "refs/__gh__",
		},
	}
	for _, p := range []struct {
		line       []byte
		hiddenRefs []config.ConfigEntry
		expected   bool
	}{
		{[]byte("886459bb202402741948881fe9e99554ba264cac refs/heads/add-testify-framework"), hiddenRefs, false},
		{[]byte("602bc9cb256c46fcf9c3351864431448096f8538 refs/heads/advertise-capabilities"), hiddenRefs, false},
		{[]byte("4fff972d2c997e98d80039551162d4cb51111760 refs/heads/initial-version"), hiddenRefs, false},
		{[]byte("b01cd23d0137c518529ab21e0d138291bd481980 refs/heads/introduce-custom-mode"), hiddenRefs, false},
		{[]byte("28e3c79ae2b2798e7468d6eeb8601408a613cbcd refs/heads/main"), hiddenRefs, false},
		{[]byte("b7841935938c6c73666e050d73e7bc8e9a547f70 refs/heads/read-commands-phase"), hiddenRefs, false},
		{[]byte("65b64fe7f4419e4ffaa988fa7ac9801baf790034 refs/remotes/origin/HEAD"), hiddenRefs, false},
		{[]byte("886459bb202402741948881fe9e99554ba264cac refs/remotes/origin/add-testify-framework"), hiddenRefs, false},
		{[]byte("e1adb492bcee63e359e30c82237b868347323f67 refs/remotes/origin/advertise-capabilities"), hiddenRefs, false},
		{[]byte("4fff972d2c997e98d80039551162d4cb51111760 refs/remotes/origin/initial-version"), hiddenRefs, false},
		{[]byte("b01cd23d0137c518529ab21e0d138291bd481980 refs/remotes/origin/introduce-custom-mode"), hiddenRefs, false},
		{[]byte("65b64fe7f4419e4ffaa988fa7ac9801baf790034 refs/remotes/origin/main"), hiddenRefs, false},
		{[]byte("93dc373b5dd6f280e57ada1ca2b41aa7dba52f89 refs/__gh__/pull/99986/rebase"), hiddenRefs, true},
		{[]byte("99681cf8b50d6c0616f10e67d7bb9f3589ca6a8d refs/gh/merge_queue/156066/6e33e3a2c52017bec941ffd6f15c20a1ae002ad9"), hiddenRefs, true},
		{[]byte("dc3d88418f0e0ad43842f5645b9a36db55187a40 refs/pull/95628/head"), hiddenRefs, true},
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
