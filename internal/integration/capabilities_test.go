//go:build integration

package integration

import (
	"bufio"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/github/spokes-receive-pack/internal/pktline"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCapabilities(t *testing.T) {
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

	getCapsAdv := func(ctx context.Context, t *testing.T, push_options bool, extraEnv ...string) pktline.Capabilities {
		srp := exec.CommandContext(ctx, "spokes-receive-pack", "--stateless-rpc", "--advertise-refs", ".")
		srp.Dir = testRepo
		configParams := gitConfigParameters
		if push_options {
			configParams = gitConfigParameters + ` 'receive.advertisePushOptions=true'`
		}

		srp.Env = append(os.Environ(),
			"GIT_CONFIG_PARAMETERS="+configParams,
			"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
			"GIT_SOCKSTAT_VAR_quarantine_id=config-test-quarantine-id")
		srp.Env = append(srp.Env, extraEnv...)
		srp.Stderr = &testLogWriter{t}
		srpOut, err := srp.StdoutPipe()
		require.NoError(t, err)
		srp.Start()

		bufSRPOut := bufio.NewReader(srpOut)
		_, capsLine, err := readAdv(bufSRPOut)
		require.NoError(t, err)
		caps, err := pktline.ParseCapabilities([]byte(capsLine))
		require.NoError(t, err)

		assert.NoError(t, srp.Wait())

		return caps
	}

	t.Run("with request id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		caps := getCapsAdv(ctx, t, false, "GIT_SOCKSTAT_VAR_request_id=test:request:id")

		assert.Equal(t, []string{
			"agent",
			"atomic",
			"delete-refs",
			"object-format",
			"ofs-delta",
			"quiet",
			"report-status",
			"report-status-v2",
			"session-id",
			"side-band-64k",
		}, caps.Names())

		assert.Equal(t, caps.SessionId().Value(), "test:request:id")
		assert.Equal(t, caps.ObjectFormat().Value(), "sha1")
		assert.Regexp(t, "^github/spokes-receive-pack-[0-9a-f]+$", caps.Agent().Value())
	})

	t.Run("without request id", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		caps := getCapsAdv(ctx, t, false)

		assert.Equal(t, []string{
			"agent",
			"atomic",
			"delete-refs",
			"object-format",
			"ofs-delta",
			"quiet",
			"report-status",
			"report-status-v2",
			"side-band-64k",
		}, caps.Names())

		assert.Equal(t, caps.ObjectFormat().Value(), "sha1")
		assert.Regexp(t, "^github/spokes-receive-pack-[0-9a-f]+$", caps.Agent().Value())
	})

	t.Run("with push options", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()

		caps := getCapsAdv(ctx, t, true)

		assert.Equal(t, []string{
			"agent",
			"atomic",
			"delete-refs",
			"object-format",
			"ofs-delta",
			"push-options",
			"quiet",
			"report-status",
			"report-status-v2",
			"side-band-64k",
		}, caps.Names())

		assert.Equal(t, "push-options", caps.PushOptions().Name())
	})
}
