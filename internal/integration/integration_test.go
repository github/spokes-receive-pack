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
	"testing"

	"github.com/github/go-pipe/pipe"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

const bogusCommit = `tree %s
author Spokes Receive Pack 1234567890 +0000
committer Spokes Receive Pack <spokes@receive.pack> 1234567890 +0000

This commit object intentionally broken
`

type SpokesReceivePackTestSuite struct {
	suite.Suite
	localRepo, remoteRepo string
}

func (suite *SpokesReceivePackTestSuite) SetupTest() {
	var err error
	req := require.New(suite.T())

	// set up a folder that will be used as a "local" Git repo
	localRepo, err := os.MkdirTemp("", "local")
	req.NoError(err, fmt.Sprintf("unable to create the local Git repo: %s", err))

	// set up a folder that will be used as a "remote" Git repo
	remoteRepo, err := os.MkdirTemp("", "remote")
	req.NoError(err, "unable to create the remote repository directory")

	req.NoError(os.Chdir(localRepo), "unable to chdir new local Git repo")

	// init and config the local Git repo
	req.NoError(exec.Command("git", "init").Run())
	req.NoError(exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	req.NoError(exec.Command("git", "config", "user.name", "spokes-receive-pack").Run())

	// add some content to our repo and commit it
	req.NoError(
		os.WriteFile("README.md", []byte("A simple README.md file"), 0644),
		"unable to create a README.md file in the Git repo")
	req.NoError(exec.Command("git", "add", ".").Run())
	req.NoError(exec.Command("git", "commit", "--message", "First commit").Run())

	// add some extra content in different branches
	branches := []string{"branch-1", "branch-2", "branch-3"}
	for i, branch := range branches {
		req.NoError(exec.Command("git", "checkout", "-b", branch).Run())
		name := fmt.Sprintf("file-%d.txt", i)
		req.NoErrorf(
			os.WriteFile(name, []byte(fmt.Sprintf("A test file with name %s", name)), 0644),
			"unable to create %s file in the Git repo", name)
		req.NoError(exec.Command("git", "add", ".").Run())
		req.NoError(exec.Command("git", "commit", "--message", fmt.Sprintf("Commit %d", i)).Run())
	}

	// configure the remote
	req.NoError(exec.Command("git", "remote", "add", "r", remoteRepo).Run())
	req.NoError(os.Chdir(remoteRepo), "unable to chdir to project base directory")

	req.NoError(exec.Command("git", "init", "--quiet", "--template=.", "--bare").Run())

	// store the state
	suite.localRepo = localRepo
	suite.remoteRepo = remoteRepo
}

func (suite *SpokesReceivePackTestSuite) TearDownTest() {
	require := require.New(suite.T())

	// Clean the environment before exiting
	require.NoError(os.RemoveAll(suite.remoteRepo))
	require.NoError(os.RemoveAll(suite.localRepo))
}

func (suite *SpokesReceivePackTestSuite) TestDefaultReceivePackSimplePush() {
	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--receive-pack=spokes-receive-pack", "r", "master").Run(),
		"unexpected error running the push with the default receive-pack implementation")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackSimplePush() {
	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--receive-pack=spokes-receive-pack-wrapper", "r", "master").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePush() {
	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushWithExtraReceiveOptions() {
	assert.NoError(suite.T(), os.Chdir(suite.remoteRepo), "unable to chdir into our remote Git repo")
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "true").Run())
	// This value is the default value we set in our production config
	require.NoError(suite.T(), exec.Command("git", "config", "receive.maxsize", "2147483648").Run())
	require.NoError(suite.T(), exec.Command("git", "config", "receive.refupdatecommandlimit", "10").Run())

	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushFailMaxSize() {
	assert.NoError(suite.T(), os.Chdir(suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Set a really low value to receive.maxsize in order to make it fail
	require.NoError(suite.T(), exec.Command("git", "config", "receive.maxsize", "1").Run())

	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	out, err := exec.Command("git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").CombinedOutput()
	assert.Error(
		suite.T(),
		err,
		"unexpected success running the push with the custom spokes-receive-pack program; it should have failed")
	outString := string(out)
	assert.Contains(suite.T(), outString, "remote: fatal: pack exceeds maximum allowed size")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushFailRefUpdateCommandLimit() {
	assert.NoError(suite.T(), os.Chdir(suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Set a low value to receive.refupdatecommandlimit in order to make it fail
	require.NoError(suite.T(), exec.Command("git", "config", "receive.refupdatecommandlimit", "1").Run())

	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")
	out, err := exec.Command("git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").CombinedOutput()
	assert.Error(
		suite.T(),
		err,
		"unexpected success running the push with the custom spokes-receive-pack program; it should have failed")
	outString := string(out)
	assert.Contains(suite.T(), outString, "maximum ref updates exceeded")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackWrongObjectFailFsckObject() {
	assert.NoError(suite.T(), os.Chdir(suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Enable the `receive.fsckObjects option
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "true").Run())

	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")

	createBogusObjectAndPush(suite, func(suite *SpokesReceivePackTestSuite, err error, out []byte) {
		assert.Error(
			suite.T(),
			err,
			"unexpected success running the push with the custom spokes-receive-pack program; it should have failed")
		outString := string(out)
		assert.Contains(suite.T(), outString, "fatal: fsck error in packed object")
	})
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackWrongObjectSucceedFsckObject() {
	assert.NoError(suite.T(), os.Chdir(suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Disable the `receive.fsckObjects option
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "false").Run())

	assert.NoError(suite.T(), os.Chdir(suite.localRepo), "unable to chdir into our local Git repo")

	createBogusObjectAndPush(suite, func(suite *SpokesReceivePackTestSuite, err error, _ []byte) {
		assert.NoError(
			suite.T(),
			err,
			"unexpected error running the push with the custom spokes-receive-pack program; it should have succeed since fsck is disabled")
	})
}

func createBogusObjectAndPush(suite *SpokesReceivePackTestSuite, validations func(*SpokesReceivePackTestSuite, error, []byte)) {
	// let's create an invalid object
	p := pipe.New(pipe.WithDir(suite.localRepo))
	p.Add(
		pipe.Command("git", "rev-parse", "HEAD^{tree}"),
		pipe.LinewiseFunction(
			"generate-bogus-tree-object",
			func(_ context.Context, _ pipe.Env, line []byte, output *bufio.Writer) error {
				output.WriteString(fmt.Sprintf(bogusCommit, string(line)))
				return nil
			},
		),
		pipe.Function(
			"compute-hash",
			func(_ context.Context, env pipe.Env, stdin io.Reader, stdout io.Writer) error {
				cmd := exec.Command("git", "hash-object", "-t", "commit", "-w", "--stdin")

				// write the bogus tree object to the stdin of the previous process
				commit := make([]byte, 201)
				n, err := stdin.Read(commit)
				require.NoError(suite.T(), err)
				b := bytes.Buffer{}
				b.Write(commit[0:n])
				cmd.Stdin = &b

				// Pass the result (hash) to the next step in the pipe
				hash, err := cmd.CombinedOutput()
				require.NoError(suite.T(), err)
				stdout.Write(hash)
				return nil
			},
		),
		pipe.LinewiseFunction(
			"push-to-remote",
			func(_ context.Context, _ pipe.Env, line []byte, _ *bufio.Writer) error {
				out, err := exec.Command(
					"git",
					"push",
					"--receive-pack=spokes-receive-pack-wrapper",
					"r",
					fmt.Sprintf("%s:refs/heads/bogus", string(line))).CombinedOutput()

				validations(suite, err, out)
				return nil
			},
		),
	)

	p.Run(context.Background())
}

func TestSpokesReceivePackTestSuite(t *testing.T) {
	suite.Run(t, new(SpokesReceivePackTestSuite))
}
