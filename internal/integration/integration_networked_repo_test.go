//go:build integration

package integration

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type SpokesReceivePackNetworkedTestSuite struct {
	suite.Suite
	clone string
}

func (suite *SpokesReceivePackNetworkedTestSuite) SetupTest() {
	req := require.New(suite.T())

	// create a local clone of the fork git-internals-fork available in the network
	clone := "git-internals-fork"
	req.NoError(exec.Command("git", "clone", "testdata/remote/git-internals-fork.git", clone).Run())

	// go into the clone
	wd, _ := os.Getwd()
	req.NoError(chdir(suite.T(), clone), "unable to chdir from %s into the recently cloned repo at %s", wd, clone)
	// init and config the local Git repo
	req.NoError(exec.Command("git", "init").Run())
	req.NoError(exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	req.NoError(exec.Command("git", "config", "user.name", "spokes-receive-pack").Run())

	// add some extra content in different branches
	branches := []string{"branch-in-fork-1", "branch-in-fork-2", "branch-in-fork-3"}
	for i, branch := range branches {
		req.NoError(exec.Command("git", "checkout", "-b", branch).Run())
		name := fmt.Sprintf("file-%d.txt", i)
		req.NoErrorf(
			os.WriteFile(name, []byte(fmt.Sprintf("A test file with name %s", name)), 0644),
			"unable to create %s file in the Git repo", name)
		req.NoError(exec.Command("git", "add", ".").Run())
		req.NoError(exec.Command("git", "commit", "--message", fmt.Sprintf("Commit %d", i)).Run())
	}

	cloneAbsPath, err := filepath.Abs(".")
	req.NoError(err, "unable to compute the absolute path of our cloned repo at %s", clone)
	suite.clone = cloneAbsPath
}

func (suite *SpokesReceivePackNetworkedTestSuite) TearDownTest() {
	require := require.New(suite.T())

	// Clean the environment before exiting
	require.NoError(os.RemoveAll(suite.clone))
	require.NoError(os.RemoveAll("../testdata/remote/git-internals-fork.git/objects/quarantine"))
}

func (suite *SpokesReceivePackNetworkedTestSuite) TestSpokesReceivePackPushFork() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.clone), "unable to chdir into our local clone of a fork at %s", suite.clone)
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--all", "--receive-pack=spokes-receive-pack-networked-wrapper", "origin").Run(),
		"unexpected error running the networked push with the custom spokes-receive-pack program")
}

func TestSpokesReceivePackNetworkedTestSuite(t *testing.T) {
	suite.Run(t, new(SpokesReceivePackNetworkedTestSuite))
}
