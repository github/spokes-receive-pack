//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

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

func TestSpokesReceivePackTestSuite(t *testing.T) {
	suite.Run(t, new(SpokesReceivePackTestSuite))
}
