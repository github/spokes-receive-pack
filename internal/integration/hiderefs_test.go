//go:build integration

package integration

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiderefsConfig(t *testing.T) {
	const (
		defaultBranch  = "refs/heads/main"
		transferhide1  = "refs/__transferhiderefs1/example/anything"
		transferhide2  = "refs/__transferhiderefs2/example/anything"
		transferunhide = "refs/__transferhiderefs2/exception/anything" // nested inside of __transferhiderefs2
		receivehide1   = "refs/__receivehiderefs1/example/anything"
		receivehide2   = "refs/__receivehiderefs2/example/anything"
		uploadhide     = "refs/__uploadhiderefs/example/anything"

		createBranch         = "refs/heads/newbranch"
		createtransferhide1  = "refs/__transferhiderefs1/example/new"
		createtransferhide2  = "refs/__transferhiderefs2/example/new"
		createtransferunhide = "refs/__transferhiderefs2/exception/new" // nested inside of __transferhiderefs2
		createreceivehide1   = "refs/__receivehiderefs1/example/new"
		createreceivehide2   = "refs/__receivehiderefs2/example/new"
		createuploadhide     = "refs/__uploadhiderefs/example/new"

		// This needs to be reachable from refs/heads/main
		testCommit = "e589bdee50e39beac56220c4b7a716225f79e3cf"

		gitConfigParameters = `'transfer.hideRefs=refs/__transferhiderefs2' ` +
			`'transfer.hideRefs='\!'refs/__transferhiderefs2/exception' ` +
			`'receive.hideRefs=refs/__receivehiderefs2'`
	)

	wd, err := os.Getwd()
	require.NoError(t, err)
	origin := filepath.Join(wd, "testdata/remote/git-internals-fork.git")

	testRepo := t.TempDir()
	requireRun(t, "git", "init", "--bare", testRepo)
	requireRun(t, "git", "-C", testRepo, "fetch", origin, defaultBranch+":"+defaultBranch)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferhide1, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferhide2, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferunhide, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", receivehide1, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", receivehide2, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", uploadhide, testCommit)
	requireRun(t, "git", "-C", testRepo, "config", "transfer.hiderefs", "refs/__transferhiderefs1")
	requireRun(t, "git", "-C", testRepo, "config", "receive.hiderefs", "refs/__receivehiderefs1")
	requireRun(t, "git", "-C", testRepo, "config", "uploadpack.hiderefs", "refs/__uploadhiderefs")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
		"GIT_CONFIG_PARAMETERS="+gitConfigParameters,
		"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
		"GIT_SOCKSTAT_VAR_quarantine_id=config-test-quarantine-id")
	srp.Stderr = &testLogWriter{t}
	srpIn, err := srp.StdinPipe()
	require.NoError(t, err)
	srpOut, err := srp.StdoutPipe()
	require.NoError(t, err)

	srpErr := make(chan error)
	go func() { srpErr <- srp.Run() }()

	bufSRPOut := bufio.NewReader(srpOut)

	refs, _, err := readAdv(bufSRPOut)
	require.NoError(t, err)
	assert.Contains(t, refs, defaultBranch)
	assert.NotContains(t, refs, transferhide1)
	assert.NotContains(t, refs, transferhide2)
	assert.Contains(t, refs, transferunhide)
	assert.NotContains(t, refs, receivehide1)
	assert.NotContains(t, refs, receivehide2)
	assert.Contains(t, refs, uploadhide)

	oldnew := fmt.Sprintf("%040d %s", 0, testCommit)
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\x00report-status report-status-v2 side-band-64k object-format=sha1\n", oldnew, createBranch))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferhide1))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferhide2))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferunhide))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createreceivehide1))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createreceivehide2))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createuploadhide))
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	// Send an empty pack, since we're using commits that are already in
	// the repo.
	pack, err := os.Open("testdata/empty.pack")
	require.NoError(t, err)
	if _, err := io.Copy(srpIn, pack); err != nil {
		t.Logf("error writing pack to spokes-receive-pack input: %v", err)
	}

	refStatus, unpackRes, _, err := readResult(t, bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		createBranch:         "ok",
		createtransferhide1:  "ng deny updating a hidden ref",
		createtransferhide2:  "ng deny updating a hidden ref",
		createtransferunhide: "ok",
		createreceivehide1:   "ng deny updating a hidden ref",
		createreceivehide2:   "ng deny updating a hidden ref",
		createuploadhide:     "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}
