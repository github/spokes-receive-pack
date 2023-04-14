//go:build integration

package integration

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

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

	req.NoError(chdir(suite.T(), localRepo), "unable to chdir new local Git repo")

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
	req.NoError(chdir(suite.T(), remoteRepo), "unable to chdir to project base directory")

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
	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--receive-pack=spokes-receive-pack", "r", "HEAD").Run(),
		"unexpected error running the push with the default receive-pack implementation")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackSimplePush() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--receive-pack=spokes-receive-pack-wrapper", "r", "HEAD").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePush() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestWithGovernor() {
	govSock, msgs, cleanup := startFakeGovernor(suite.T())
	defer cleanup()

	cmd := exec.Command("git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r")
	cmd.Env = append(os.Environ(), "GIT_SOCKSTAT_PATH="+govSock)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(suite.T(), cmd.Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")

	timeout := time.After(time.Second)
	requireGovernorMessage(suite.T(), timeout, msgs, func(msg govMessage) {
		assert.Equal(suite.T(), "update", msg.Command)
		assert.ElementsMatch(suite.T(), []string{"pid", "program", "git_dir"}, keys(msg.Data))
		assert.Equal(suite.T(), "spokes-receive-pack", msg.Data["program"])
		assert.Equal(suite.T(), filepath.Base(suite.remoteRepo), filepath.Base(msg.Data["git_dir"].(string))) // avoid problems from non-canonical paths, e.g. on macOS with its /tmp symlink.
	})
	requireGovernorMessage(suite.T(), timeout, msgs, func(msg govMessage) {
		assert.Equal(suite.T(), "finish", msg.Command)
		// This varies by platform:
		// assert.ElementsMatch(suite.T(), []string{
		// 	"result_code",
		// 	"receive_pack_size",
		// 	"cpu",
		// 	"rss",
		// 	"read_bytes",
		// 	"write_bytes",
		// }, keys(msg.Data))
		assert.Equal(suite.T(), float64(0), msg.Data["result_code"])
		assert.Greaterf(suite.T(), msg.Data["receive_pack_size"], float64(0), "expect receive_pack_size (%v) to be more than 0", msg.Data["receive_pack_size"])
		assert.Greaterf(suite.T(), msg.Data["cpu"], float64(0), "expect cpu (%v) to be more than 0", msg.Data["cpu"])
		assert.Greaterf(suite.T(), msg.Data["rss"], float64(0), "expect rss (%v) to be more than 0", msg.Data["rss"])
	})
}

func startFakeGovernor(t *testing.T) (string, <-chan govMessage, func()) {
	tmpdir, err := os.MkdirTemp("", "spokes-receive-pack-governor-*")
	require.NoError(t, err)
	cleanup := func() { os.RemoveAll(tmpdir) }

	sockpath := filepath.Join(tmpdir, "gov.sock")
	t.Logf("fake gov listening on %s", sockpath)
	l, err := net.Listen("unix", sockpath)
	require.NoError(t, err)

	msgs := make(chan govMessage, 2)
	go func() {
		defer l.Close()
		defer close(msgs)
		conn, err := l.Accept()
		if err != nil {
			t.Logf("gov: accept error: %v", err)
			return
		}
		t.Logf("gov accepted on %s", sockpath)
		decoder := json.NewDecoder(conn)
		for {
			var msg govMessage
			err := decoder.Decode(&msg)
			if err != nil {
				if err != io.EOF {
					t.Logf("gov: read error: %v", err)
				}
				break
			}
			if msg.Command == "schedule" {
				conn.Write([]byte("continue\n"))
			} else {
				msgs <- msg
			}
		}
	}()

	return sockpath, msgs, cleanup
}

type govMessage struct {
	Command string                 `json:"command"`
	Data    map[string]interface{} `json:"data"`
}

func requireGovernorMessage(t *testing.T, timeout <-chan time.Time, msgs <-chan govMessage, verify func(msg govMessage)) {
	select {
	case msg, ok := <-msgs:
		require.True(t, ok)
		verify(msg)
	case <-timeout:
		t.Fatal("timed out waiting for gov message from spokes-receive-pack")
	}
}

func keys(m map[string]interface{}) []string {
	var res []string
	for k := range m {
		res = append(res, k)
	}
	return res
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushWithExtraReceiveOptions() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.remoteRepo), "unable to chdir into our remote Git repo")
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "true").Run())
	// This value is the default value we set in our production config
	require.NoError(suite.T(), exec.Command("git", "config", "receive.maxsize", "2147483648").Run())
	require.NoError(suite.T(), exec.Command("git", "config", "receive.refupdatecommandlimit", "10").Run())
	require.NoError(suite.T(), exec.Command("git", "config", "receive.reportStatusFF", "true").Run())

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	assert.NoError(
		suite.T(),
		exec.Command(
			"git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").Run(),
		"unexpected error running the push with the custom spokes-receive-pack program")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushFailMaxSize() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Set a really low value to receive.maxsize in order to make it fail
	require.NoError(suite.T(), exec.Command("git", "config", "receive.maxsize", "1").Run())

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	out, err := exec.Command("git", "push", "--all", "--receive-pack=spokes-receive-pack-wrapper", "r").CombinedOutput()
	assert.Error(
		suite.T(),
		err,
		"unexpected success running the push with the custom spokes-receive-pack program; it should have failed")
	outString := string(out)
	assert.Contains(suite.T(), outString, "remote: fatal: pack exceeds maximum allowed size")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackMultiplePushFailRefUpdateCommandLimit() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Set a low value to receive.refupdatecommandlimit in order to make it fail
	require.NoError(suite.T(), exec.Command("git", "config", "receive.refupdatecommandlimit", "1").Run())

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")
	out, err := exec.Command(
		"git",
		"push",
		"--receive-pack=spokes-receive-pack-wrapper",
		"r",
		"branch-1",
		"branch-2",
		"branch-3").CombinedOutput()

	assert.Error(
		suite.T(),
		err,
		"unexpected success running the push with the custom spokes-receive-pack program; it should have failed")
	outString := string(out)
	assert.Contains(suite.T(), outString, "maximum ref updates exceeded")
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackWrongObjectFailFsckObject() {
	assert.NoError(suite.T(), chdir(suite.T(), suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Enable the `receive.fsckObjects option
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "true").Run())

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")

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
	assert.NoError(suite.T(), chdir(suite.T(), suite.remoteRepo), "unable to chdir into our remote Git repo")
	// Disable the `receive.fsckObjects option
	require.NoError(suite.T(), exec.Command("git", "config", "receive.fsckObjects", "false").Run())

	assert.NoError(suite.T(), chdir(suite.T(), suite.localRepo), "unable to chdir into our local Git repo")

	createBogusObjectAndPush(suite, func(suite *SpokesReceivePackTestSuite, err error, _ []byte) {
		assert.NoError(
			suite.T(),
			err,
			"unexpected error running the push with the custom spokes-receive-pack program; it should have succeed since fsck is disabled")
	})
}

func (suite *SpokesReceivePackTestSuite) TestSpokesReceivePackPushFromShallowClone() {
	var cmd *exec.Cmd

	// Set up a bare repository that the shallow clone can clone from and push to.
	remoteForShallow, err := os.MkdirTemp("", "shallow-remote")
	require.NoError(suite.T(), err, "unable to create a dir for a shallow clone")
	suite.T().Cleanup(func() { _ = os.RemoveAll(remoteForShallow) })

	// localRepo has the objects we want the remote to start with, so clone from there into a bare repository.
	cmd = exec.Command("git", "clone", "--bare", suite.localRepo, remoteForShallow)
	require.NoError(suite.T(), cmd.Run(), "git clone --bare %s %s", suite.localRepo, remoteForShallow)

	// Make a shallow clone from our new bare repo.
	shallowClone, err := os.MkdirTemp("", "shallow")
	require.NoError(suite.T(), err, "unable to create a dir for a shallow clone")
	suite.T().Cleanup(func() { _ = os.RemoveAll(shallowClone) })

	cmd = exec.Command("git", "clone", "--depth=1", fmt.Sprintf("file://%s", remoteForShallow), shallowClone)
	require.NoError(suite.T(), cmd.Run(), "git clone --depth=1 %s %s", remoteForShallow, shallowClone)

	mustRunGit := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = shallowClone
		out, err := cmd.CombinedOutput()
		suite.T().Logf("[in %s] git %v:\n%s", shallowClone, args, out)
		require.NoError(suite.T(), err)
	}

	mustRunGit("config", "user.email", "spokes-receive-pack@github.com")
	mustRunGit("config", "user.name", "spokes-receive-pack")

	// Add a file to the shallow clone and push.
	require.NoError(suite.T(),
		os.WriteFile(filepath.Join(shallowClone, "file-from-shallow.txt"),
			[]byte("this is a file created in a shallow clone.\n"),
			0644))

	mustRunGit("add", "file-from-shallow.txt")
	mustRunGit("commit", "--message", "commit in shallow clone")
	mustRunGit("push", "--receive-pack=spokes-receive-pack-wrapper", "origin", "HEAD:test")
}

func createBogusObjectAndPush(suite *SpokesReceivePackTestSuite, validations func(*SpokesReceivePackTestSuite, error, []byte)) {
	var pushOut []byte
	var pushErr error

	h := func(event *pipe.Event) {
		log.Printf("PIPELINE EVENT:")
		log.Printf("-- COMMAND = %q", event.Command)
		log.Printf("-- MSG = %q", event.Msg)
		log.Printf("-- CONTEXT = %v", event.Context)
		for err := event.Err; err != nil; err = errors.Unwrap(err) {
			log.Printf("-- ERROR: (%T) %v", err, err)
			switch e := err.(type) {
			case *exec.ExitError:
				log.Printf("--- exit code: %v", e.ExitCode())
				log.Printf("--- stderr: %s", e.Stderr)
			}
		}
	}

	// let's create an invalid object
	p := pipe.New(pipe.WithDir(suite.localRepo), pipe.WithEventHandler(h))
	p.Add(
		pipe.Command("git", "rev-parse", "HEAD^{tree}"),
		pipe.Function(
			"generate-bogus-tree-object",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, stdout io.Writer) error {
				data, err := io.ReadAll(stdin)
				if err != nil {
					return err
				}
				_, err = stdout.Write([]byte(fmt.Sprintf(bogusCommit, strings.TrimSpace(string(data)))))
				return err
			},
		),
		pipe.Command("git", "hash-object", "-t", "commit", "-w", "--stdin", "--literally"),
		pipe.Function(
			"push-to-remote",
			func(_ context.Context, _ pipe.Env, stdin io.Reader, _ io.Writer) error {
				data, err := io.ReadAll(stdin)
				if err != nil {
					return err
				}
				line := strings.TrimSpace(string(data))
				pushOut, pushErr = exec.Command(
					"git",
					"push",
					"--receive-pack=spokes-receive-pack-wrapper",
					"r",
					fmt.Sprintf("%s:refs/heads/bogus", line)).CombinedOutput()

				return nil
			},
		),
	)

	require.NoError(suite.T(), p.Run(context.Background()))

	validations(suite, pushErr, pushOut)
}

func TestSpokesReceivePackTestSuite(t *testing.T) {
	suite.Run(t, new(SpokesReceivePackTestSuite))
}
