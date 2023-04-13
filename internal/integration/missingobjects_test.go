//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/github/go-pipe/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMissingObjects(t *testing.T) {
	var info struct {
		OldOID string `json:"push_from"`
		NewOID string `json:"push_to"`
		Ref    string `json:"ref"`
	}
	const (
		remote   = "testdata/missing-objects/remote.git"
		badPack  = "testdata/missing-objects/bad.pack"
		infoFile = "testdata/missing-objects/info.json"
		otherRef = "refs/heads/other"

		quarantineID = "test-quarantine-id"
	)

	infoJSON, err := os.ReadFile(infoFile)
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(infoJSON, &info))

	origin, err := filepath.Abs(remote)
	require.NoError(t, err)

	testRepo := t.TempDir()
	requireRun(t, "git", "init", "--bare", testRepo)
	requireRun(t, "git", "-C", testRepo, "fetch", origin, info.Ref+":"+info.Ref)

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
		"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
		"GIT_SOCKSTAT_VAR_quarantine_id="+quarantineID)
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
		info.Ref: info.OldOID,
	})

	// Try to update the ref that's already there to commit C (but we won't
	// push its parent and the remote doesn't have the parent either).
	require.NoError(t, writePktlinef(srpIn,
		"%s %s %s\x00report-status report-status-v2 side-band-64k object-format=sha1\n",
		info.OldOID, info.NewOID, info.Ref))
	// Try to create another ref with a commit that the remote already has.
	require.NoError(t, writePktlinef(srpIn,
		"%040d %s %s",
		0, info.OldOID, otherRef))
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
		info.Ref: "ng missing necessary objects",
		otherRef: "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)

	// Now, make sure that we can read all of the objects we just received.
	quarantineDir := filepath.Join(testRepo, "objects", quarantineID)
	quarantinePackDir := filepath.Join(quarantineDir, "pack")
	packs, err := os.ReadDir(quarantinePackDir)
	require.NoError(t, err)
	var idxF io.Reader
	for i := range packs {
		f := packs[i]
		fn := f.Name()
		if strings.HasSuffix(fn, ".idx") {
			t.Logf("found index file %q", fn)
			assert.Nil(t, idxF, "expected only one idx file")
			f, err := os.Open(filepath.Join(quarantinePackDir, fn))
			require.NoError(t, err)
			idxF = f
			defer f.Close()
		}
	}
	require.NotNil(t, idxF, "expected at least one pack index file")

	var checkOut bytes.Buffer
	p := pipe.New(
		pipe.WithStdout(&checkOut),
		pipe.WithStdin(idxF),
		pipe.WithEnvVars([]pipe.EnvVar{
			{Key: "GIT_ALTERNATE_OBJECT_DIRECTORIES", Value: filepath.Join(testRepo, "objects")},
			{Key: "GIT_OBJECT_DIRECTORY", Value: quarantineDir},
			{Key: "GIT_QUARANTINE_PATH", Value: quarantineDir},
		}),
	)
	p.Add(pipe.Command("git", "--git-dir", testRepo, "show-index"))
	p.Add(pipe.LinewiseFunction("parse show-index output", func(_ context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
		parts := bytes.Split(line, []byte(" "))
		if len(parts) < 2 {
			return fmt.Errorf("invalid output from show-index: %s", line)
		}
		_, err := fmt.Fprintf(stdout, "%s\n", parts[1])
		return err
	}))
	p.Add(pipe.Command("git", "cat-file", "--batch"))
	assert.NoError(t, p.Run(ctx))
	assert.Equal(t, `04ae1b030995df0ae3e91c41d1a065aaae405397 commit 205
tree 4b825dc642cb6eb9a060e54bf8d69288fbee4904
parent 8f7da4e89f96103db885fd8919a7dbb245e70d59
author Hubot <hubot@github.com> 1681399020 +1200
committer Hubot <hubot@github.com> 1681399020 +1200

commit C

`, checkOut.String())
}
