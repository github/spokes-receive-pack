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

	cmd := commandBuilderInDir(localRepo)

	// init and config the local Git repo
	assert.NoError(t, cmd("git", "init").Run())
	assert.NoError(t, cmd("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, cmd("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, cmd("git", "config", "receive.hiderefs", "refs/pull/").Run())
	assert.NoError(t, cmd("git", "config", "--add", "receive.hiderefs", "refs/gh/").Run())
	assert.NoError(t, cmd("git", "config", "--add", "receive.hiderefs", "refs/__gh__").Run())

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

	cmd := commandBuilderInDir(localRepo)

	// init and config the local Git repo
	assert.NoError(t, cmd("git", "init").Run())
	assert.NoError(t, cmd("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, cmd("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, cmd("git", "config", "receive.fsckObjects", "true").Run())
	assert.NoError(t, cmd("git", "config", "receive.maxsize", "11").Run())

	fsckObjects := testGetConfigEntryValue(localRepo, "receive.fsckObjects")
	assert.Equal(t, "true", fsckObjects)
	maxSize := testGetConfigEntryValue(localRepo, "receive.maxsize")
	assert.Equal(t, "11", maxSize)
}

func TestGetConfigEntryMultipleValues(t *testing.T) {
	localRepo, err := os.MkdirTemp("", "repo")
	defer os.RemoveAll(localRepo)

	assert.NoError(t, err, fmt.Sprintf("unable to create the local Git repo: %s", err))

	cmd := commandBuilderInDir(localRepo)

	// init and config the local Git repo
	assert.NoError(t, cmd("git", "init").Run())
	assert.NoError(t, cmd("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, cmd("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, cmd("git", "config", "receive.multivalue", "a").Run())
	assert.NoError(t, cmd("git", "config", "--add", "receive.multivalue", "b").Run())
	assert.NoError(t, cmd("git", "config", "--add", "receive.multivalue", "c").Run())

	fsckObjects := testGetConfigEntryValue(localRepo, "receive.multivalue")
	assert.Equal(t, "c", fsckObjects)
}
func TestGetPrefixParsesArgs(t *testing.T) {
	localRepo, err := os.MkdirTemp("", "repo")
	defer os.RemoveAll(localRepo)

	assert.NoError(t, err, fmt.Sprintf("unable to create the local Git repo: %s", err))

	cmd := commandBuilderInDir(localRepo)

	// init and config the local Git repo
	assert.NoError(t, cmd("git", "init").Run())
	assert.NoError(t, cmd("git", "config", "user.email", "spokes-receive-pack@github.com").Run())
	assert.NoError(t, cmd("git", "config", "user.name", "spokes-receive-pack").Run())
	assert.NoError(t, cmd("git", "config", "receive.fsck.missingEmail", "ignore").Run())
	assert.NoError(t, cmd("git", "config", "receive.fsck.badTagName", "ignore").Run())
	assert.NoError(t, cmd("git", "config", "--add", "receive.fsck.badTagName", "error").Run())

	config, _ := GetConfig(localRepo)
	prefix := config.GetPrefix("receive.fsck.")

	assert.Equal(t, prefix["missingemail"][0], "ignore")
	assert.Equal(t, prefix["badtagname"][0], "ignore")
	assert.Equal(t, prefix["badtagname"][1], "error")
}

func commandBuilderInDir(dir string) func(string, ...string) *exec.Cmd {
	return func(program string, args ...string) *exec.Cmd {
		c := exec.Command(program, args...)
		c.Dir = dir
		return c
	}
}

func TestParseSigned(t *testing.T) {
	for _, c := range []struct {
		str     string
		want    int
		wantErr string
	}{
		// valid input, no suffix
		{"81", 81, ""},

		// valid input, with lower- and upper-case suffixes
		{"2k", 2 * 1024, ""},
		{"3m", 3 * 1024 * 1024, ""},
		{"4g", 4 * 1024 * 1024 * 1024, ""},
		{"2K", 2 * 1024, ""},
		{"3M", 3 * 1024 * 1024, ""},
		{"4G", 4 * 1024 * 1024 * 1024, ""},

		// valid negative input, with lower- and upper-case suffixes
		{"-2k", -2 * 1024, ""},
		{"-3m", -3 * 1024 * 1024, ""},
		{"-4g", -4 * 1024 * 1024 * 1024, ""},
		{"-2K", -2 * 1024, ""},
		{"-3M", -3 * 1024 * 1024, ""},
		{"-4G", -4 * 1024 * 1024 * 1024, ""},

		// empty input, just a suffix
		{"k", 0, "strconv.Atoi: parsing \"\": invalid syntax"},
		{"m", 0, "strconv.Atoi: parsing \"\": invalid syntax"},
		{"g", 0, "strconv.Atoi: parsing \"\": invalid syntax"},

		// invalid input, no suffix
		{"NaN", 0, "strconv.Atoi: parsing \"NaN\": invalid syntax"},
	} {
		got, gotErr := ParseSigned(c.str)

		assert.Equal(t, c.want, got)
		if c.wantErr != "" {
			assert.EqualError(t, gotErr, c.wantErr)
		}
	}
}
