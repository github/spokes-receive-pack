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

func TestPushOptions(t *testing.T) {
	const (
		defaultBranch = "refs/heads/main"
		createBranch  = "refs/heads/newbranch"

		testCommit = "e589bdee50e39beac56220c4b7a716225f79e3cf"
	)

	wd, err := os.Getwd()
	require.NoError(t, err)
	origin := filepath.Join(wd, "testdata/remote/git-internals-fork.git")

	testRepo := t.TempDir()
	requireRun(t, "git", "init", "--bare", testRepo)
	requireRun(t, "git", "-C", testRepo, "fetch", origin, defaultBranch+":"+defaultBranch)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
		"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
		"GIT_SOCKSTAT_VAR_quarantine_id=config-test-quarantine-id")
	srp.Stderr = os.Stderr
	srpIn, err := srp.StdinPipe()
	require.NoError(t, err)
	srpOut, err := srp.StdoutPipe()
	require.NoError(t, err)

	srpErr := make(chan error)
	go func() { srpErr <- srp.Run() }()

	bufSRPOut := bufio.NewReader(srpOut)

	refs, _, err := readAdv(bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, refs, map[string]string{
		defaultBranch: testCommit,
	})

	oldnew := fmt.Sprintf("%040d %s", 0, testCommit)
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\x00report-status report-status-v2 side-band-64k push-options object-format=sha1\n", oldnew, createBranch))
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	require.NoError(t, writePktlinef(srpIn,
		"anything i want to put in a push option\n"))
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	// Send an empty pack, since we're using commits that are already in
	// the repo.
	packObjects := exec.CommandContext(ctx, "git", "-C", testRepo, "pack-objects", "--all-progress-implied", "--revs", "--stdout", "--thin", "--delta-base-offset", "--progress")
	packObjects.Stderr = os.Stderr
	pack, err := packObjects.StdoutPipe()
	require.NoError(t, err)
	go packObjects.Run()
	_, err = io.Copy(srpIn, pack)
	require.NoError(t, err)

	require.NoError(t, srpIn.Close())

	refStatus, unpackRes, _, err := readResult(bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		createBranch: "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}
