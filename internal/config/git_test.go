package config

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestConfigKeyMatchesPrefix(t *testing.T) {
	for _, p := range []struct {
		key, prefix    string
		expectedBool   bool
		expectedString string
	}{
		{"foo.bar", "", true, "foo.bar"},
		{"foo.bar", "foo", true, "bar"},
		{"foo.bar", "foo.", true, "bar"},
		{"foo.bar", "foo.bar", true, ""},
		{"foo.bar", "foo.bar.", false, ""},
		{"foo.bar", "foo.bar.baz", false, ""},
		{"foo.bar", "foo.barbaz", false, ""},
		{"foo.bar.baz", "foo.bar", true, "baz"},
		{"foo.barbaz", "foo.bar", false, ""},
		{"foo.bar", "bar", false, ""},
	} {
		t.Run(
			fmt.Sprintf("TestConfigKeyMatchesPrefix(%q, %q)", p.key, p.prefix),
			func(t *testing.T) {
				ok, s := configKeyMatchesPrefix(p.key, p.prefix)
				assert.Equal(t, p.expectedBool, ok)
				assert.Equal(t, p.expectedString, s)
			},
		)
	}
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

	config, err := GetConfig(localRepo, "receive.hiderefs")
	assert.NoError(t, err, "unable to properly extract the receive section from the GitConfig")

	assert.Equalf(t, 3, len(config.Entries), "expected %d under %s prefix but got %d", 3, config.Prefix, len(config.Entries))
	assert.Equal(t, config.Entries[0].Value, "refs/pull/")
	assert.Equal(t, config.Entries[1].Value, "refs/gh/")
	assert.Equal(t, config.Entries[2].Value, "refs/__gh__")
}
