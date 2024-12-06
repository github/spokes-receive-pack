//go:build integration

package integration

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/spokes-receive-pack/internal/objectformat"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingObjects(t *testing.T) {

	x := setUpMissingObjectsTestRepo(t)
	testRepo := x.TestRepo
	info := x.Info

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := startSpokesReceivePack(ctx, t, testRepo)

	refs, _, err := readAdv(srp.Out)
	require.NoError(t, err)
	assert.Equal(t, refs, map[string]string{
		info.Ref:    info.OldOID,
		info.DelRef: info.OldOID,
	})

	// Send the pack that's missing a commit.
	pack, err := os.Open("testdata/missing-objects/bad.pack")
	require.NoError(t, err)
	defer pack.Close()

	writePushData(
		t, srp,
		[]refUpdate{
			// Try to update the ref that's already there to commit C (but we won't
			// push its parent and the remote doesn't have the parent either).
			{info.OldOID, info.NewOID, info.Ref},
		},
		pack,
	)

	refStatus, unpackRes, _, err := readResult(t, srp.Out)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		info.Ref: "ng error processing packfiles: exit status 128",
	}, refStatus)
	assert.Equal(t, "unpack index-pack failed\n", unpackRes)
}

func TestDeleteAndUpdate(t *testing.T) {
	const refToCreate = "refs/heads/new-branch"

	x := setUpMissingObjectsTestRepo(t)
	testRepo := x.TestRepo
	info := x.Info

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := startSpokesReceivePack(ctx, t, testRepo)

	refs, _, err := readAdv(srp.Out)
	require.NoError(t, err)
	assert.Equal(t, refs, map[string]string{
		info.Ref:    info.OldOID,
		info.DelRef: info.OldOID,
	})

	// Send the pack that's missing a commit.
	pack, err := os.Open("testdata/missing-objects/empty.pack")
	require.NoError(t, err)
	defer pack.Close()

	writePushData(
		t, srp,
		[]refUpdate{
			// Try to create another ref with a commit that the remote already has.
			{objectformat.NullOIDSHA1, info.OldOID, refToCreate},
			// Try to delete a ref.
			{info.OldOID, objectformat.NullOIDSHA1, info.DelRef},
		},
		pack,
	)

	refStatus, unpackRes, _, err := readResult(t, srp.Out)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		refToCreate: "ok",
		info.DelRef: "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}

type missingObjectsTestInfo struct {
	TestRepo string
	Info     struct {
		OldOID string `json:"push_from"`
		NewOID string `json:"push_to"`
		Ref    string `json:"ref"`
		DelRef string `json:"extra_ref"`
	}
}

func setUpMissingObjectsTestRepo(t *testing.T) missingObjectsTestInfo {
	const (
		remote   = "testdata/missing-objects/remote.git"
		badPack  = "testdata/missing-objects/bad.pack"
		infoFile = "testdata/missing-objects/info.json"
	)

	var res missingObjectsTestInfo

	infoJSON, err := os.ReadFile(infoFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(infoJSON, &res.Info))

	origin, err := filepath.Abs(remote)
	require.NoError(t, err)

	res.TestRepo = t.TempDir()
	requireRun(t, "git", "clone", "--mirror", origin, res.TestRepo)

	return res
}

type spokesReceivePackProcess struct {
	Cmd *exec.Cmd
	In  io.WriteCloser
	Out io.Reader
	Err chan error
}

func startSpokesReceivePack(ctx context.Context, t *testing.T, testRepo string) spokesReceivePackProcess {
	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
		"GIT_SOCKSTAT_VAR_quarantine_id=config-test-quarantine-id")
	srp.Stderr = &testLogWriter{t}
	srpIn, err := srp.StdinPipe()
	require.NoError(t, err)
	srpOut, err := srp.StdoutPipe()
	require.NoError(t, err)

	srpErr := make(chan error)
	go func() { srpErr <- srp.Run() }()

	bufSRPOut := bufio.NewReader(srpOut)

	return spokesReceivePackProcess{
		Cmd: srp,
		In:  srpIn,
		Out: bufSRPOut,
		Err: srpErr,
	}
}

type refUpdate struct {
	OldOID, NewOID, Ref string
}

func writePushData(t *testing.T, srp spokesReceivePackProcess, updates []refUpdate, pack io.Reader) {
	caps := "\x00report-status report-status-v2 side-band-64k object-format=sha1\n"
	for _, up := range updates {
		require.NoError(t, writePktlinef(srp.In,
			"%s %s %s%s",
			up.OldOID, up.NewOID, up.Ref,
			caps))
		caps = ""
	}

	_, err := srp.In.Write([]byte("0000"))
	require.NoError(t, err)

	if _, err := io.Copy(srp.In, pack); err != nil {
		t.Logf("error writing pack to spokes-receive-pack input: %v", err)
	}

	require.NoError(t, srp.In.Close())
}
