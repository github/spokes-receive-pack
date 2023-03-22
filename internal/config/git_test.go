package config

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func testGetConfigEntryValue(repoPath, name string) string {
	c, err := GetConfig(repoPath)
	if err != nil {
		panic(err)
	}
	return c.Get(name)
}

func TestGetConfigMultipleValues(t *testing.T) {
	localRepo, err := os.MkdirTemp("", "repo")
	defer os.RemoveAll(localRepo)

	assert.NoError(t, err, fmt.Sprintf("unable to create the local Git repo: %s", err))
	assert.NoError(t, os.Chdir(localRepo), "unable to chdir new local Git repo")

	// init and config the local Git repo
	assert.NoError(t, exec.Command("git", "init").Run())
	assert.NoError(t, exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, exec.Command("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, exec.Command("git", "config", "receive.hiderefs", "refs/pull/").Run())
	assert.NoError(t, exec.Command("git", "config", "--add", "receive.hiderefs", "refs/gh/").Run())
	assert.NoError(t, exec.Command("git", "config", "--add", "receive.hiderefs", "refs/__gh__").Run())

	config, err := GetConfig(localRepo)
	assert.NoError(t, err, "unable to properly extract the receive section from the GitConfig")

	values := config.GetAll("receive.hiderefs")
	assert.Equalf(t, 3, len(values), "expected %d values but got %d", 3, len(values))
	assert.Equal(t, values[0], "refs/pull/")
	assert.Equal(t, values[1], "refs/gh/")
	assert.Equal(t, values[2], "refs/__gh__")
}

func TestGetConfigEntryValues(t *testing.T) {
	localRepo, err := os.MkdirTemp("", "repo")
	defer os.RemoveAll(localRepo)

	assert.NoError(t, err, fmt.Sprintf("unable to create the local Git repo: %s", err))
	assert.NoError(t, os.Chdir(localRepo), "unable to chdir new local Git repo")

	// init and config the local Git repo
	assert.NoError(t, exec.Command("git", "init").Run())
	assert.NoError(t, exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, exec.Command("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, exec.Command("git", "config", "receive.fsckObjects", "true").Run())
	assert.NoError(t, exec.Command("git", "config", "receive.maxsize", "11").Run())

	fsckObjects := testGetConfigEntryValue(localRepo, "receive.fsckObjects")
	assert.Equal(t, "true", fsckObjects)
	maxSize := testGetConfigEntryValue(localRepo, "receive.maxsize")
	assert.Equal(t, "11", maxSize)
}

func TestGetConfigEntryMultipleValues(t *testing.T) {
	localRepo, err := os.MkdirTemp("", "repo")
	defer os.RemoveAll(localRepo)

	assert.NoError(t, err, fmt.Sprintf("unable to create the local Git repo: %s", err))
	assert.NoError(t, os.Chdir(localRepo), "unable to chdir new local Git repo")

	// init and config the local Git repo
	assert.NoError(t, exec.Command("git", "init").Run())
	assert.NoError(t, exec.Command("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, exec.Command("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, exec.Command("git", "config", "receive.multivalue", "a").Run())
	assert.NoError(t, exec.Command("git", "config", "--add", "receive.multivalue", "b").Run())
	assert.NoError(t, exec.Command("git", "config", "--add", "receive.multivalue", "c").Run())

	fsckObjects := testGetConfigEntryValue(localRepo, "receive.multivalue")
	assert.Equal(t, "c", fsckObjects)
}
