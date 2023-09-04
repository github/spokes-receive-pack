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

const (
	defaultBranch = "refs/heads/main"
	createBranch  = "refs/heads/newbranch"

	testCommit = "e589bdee50e39beac56220c4b7a716225f79e3cf"
)

func setupTestRepo(t *testing.T) string {
	wd, err := os.Getwd()
	require.NoError(t, err)
	origin := filepath.Join(wd, "testdata/remote/git-internals-fork.git")

	testRepo := t.TempDir()
	requireRun(t, "git", "init", "--bare", testRepo)
	requireRun(t, "git", "-C", testRepo, "fetch", origin, defaultBranch+":"+defaultBranch)

	return testRepo
}

func TestPushOptions(t *testing.T) {
	testRepo := setupTestRepo(t)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
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
	pack, err := os.Open("testdata/empty.pack")
	require.NoError(t, err)
	if _, err := io.Copy(srpIn, pack); err != nil {
		t.Logf("error writing pack to spokes-receive-pack input: %v", err)
	}

	require.NoError(t, srpIn.Close())

	refStatus, unpackRes, _, err := readResult(t, bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		createBranch: "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}

func TestPushOptionsLimitCount(t *testing.T) {
	testRepo := setupTestRepo(t)
	requireRun(t, "git", "-C", testRepo, "config", "receive.pushOptionsCountLimit", "2")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
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
	assert.Equal(t, refs, map[string]string{
		defaultBranch: testCommit,
	})

	oldnew := fmt.Sprintf("%040d %s", 0, testCommit)
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\x00report-status report-status-v2 side-band-64k push-options object-format=sha1\n", oldnew, createBranch))
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	// the limit is 2, let's send 3 push options
	for i := 0; i < 3; i++ {
		require.NoError(t, writePktlinef(srpIn,
			fmt.Sprintf("option-%d\n", i)))
	}
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	// Send an example pack, since we're using commits that are already in
	// the repo.
	pack, err := os.Open("testdata/empty.pack")
	require.NoError(t, err)
	if _, err := io.Copy(srpIn, pack); err != nil {
		t.Logf("error writing pack to spokes-receive-pack input: %v", err)
	}

	require.NoError(t, srpIn.Close())

	refStatus, unpackRes, _, err := readResult(t, bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		createBranch: "ng push options count exceeds maximum",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}
