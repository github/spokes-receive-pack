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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingObjects(t *testing.T) {
	var info struct {
		OldOID string `json:"push_from"`
		NewOID string `json:"push_to"`
		Ref    string `json:"ref"`
		DelRef string `json:"extra_ref"`
	}
	const (
		remote      = "testdata/missing-objects/remote.git"
		badPack     = "testdata/missing-objects/bad.pack"
		infoFile    = "testdata/missing-objects/info.json"
		refToCreate = "refs/heads/new-branch"
	)

	infoJSON, err := os.ReadFile(infoFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(infoJSON, &info))

	origin, err := filepath.Abs(remote)
	require.NoError(t, err)

	testRepo := t.TempDir()
	requireRun(t, "git", "clone", "--mirror", origin, testRepo)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

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

	refs, _, err := readAdv(bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, refs, map[string]string{
		info.Ref:    info.OldOID,
		info.DelRef: info.OldOID,
	})

	// Try to update the ref that's already there to commit C (but we won't
	// push its parent and the remote doesn't have the parent either).
	require.NoError(t, writePktlinef(srpIn,
		"%s %s %s\x00report-status report-status-v2 side-band-64k object-format=sha1\n",
		info.OldOID, info.NewOID, info.Ref))

	// Try to create another ref with a commit that the remote already has.
	require.NoError(t, writePktlinef(srpIn,
		"%040d %s %s",
		0, info.OldOID, refToCreate))

	// Try to delete another ref.
	require.NoError(t, writePktlinef(srpIn,
		"%s %040d %s",
		info.OldOID, 0, info.DelRef))

	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	// Send the pack that's missing a commit.
	pack, err := os.Open("testdata/missing-objects/bad.pack")
	require.NoError(t, err)
	if _, err := io.Copy(srpIn, pack); err != nil {
		t.Logf("error writing pack to spokes-receive-pack input: %v", err)
	}

	require.NoError(t, srpIn.Close())

	refStatus, unpackRes, _, err := readResult(t, bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		info.Ref:    "ng missing necessary objects",
		info.DelRef: "ok",
		refToCreate: "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}
