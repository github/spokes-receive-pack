//go:build integration

package integration

import (
	"bufio"
	"bytes"
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

func TestNoSideBand(t *testing.T) {
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
		"%s %s\x00report-status report-status-v2 push-options object-format=sha1\n", oldnew, createBranch))
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

	lines, err := readResultNoSideBand(t, bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, []string{
		"unpack ok\n",
		"ok refs/heads/newbranch\n",
	}, lines)
}

func readResultNoSideBand(t *testing.T, r io.Reader) ([]string, error) {
	var lines []string

	// Read all of the output so that we can include it with errors.
	data, err := io.ReadAll(r)
	if err != nil {
		if len(data) > 0 {
			t.Logf("got data, but there was an error: %v", err)
		} else {
			return nil, err
		}
	}

	// Replace r.
	r = bytes.NewReader(data)

	for {
		pkt, err := readPktline(r)
		switch {
		case err != nil:
			return nil, fmt.Errorf("%w while parsing %q", err, string(data))

		case pkt == nil:
			return lines, nil

		default:
			lines = append(lines, string(pkt))
		}
	}
}
