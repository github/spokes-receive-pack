package config

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// ConfigEntry represents an entry in the gitconfig.
type ConfigEntry struct {
	// Key is the entry's key, with any common `prefix` removed (see
	// `Config()`).
	Key string

	// Value is the entry's value, as a string.
	Value string
}

// Config represents the gitconfig, or part of the gitconfig, read by
// `ReadConfig()`.
type Config struct {
	// Prefix is the key prefix that was read to fill this `Config`.
	Prefix string

	// Entries contains the configuration entries that matched
	// `Prefix`, in the order that they are reported by `git config
	// --list`.
	Entries []ConfigEntry
}

// GetConfig returns the entries from gitconfig in the repo located at repo.
// If `prefix` is provided, then only include entries in that section, which must
// match the at a component boundary (as defined by
// `configKeyMatchesPrefix()`), and strip off the prefix in the keys
// that are returned.
func GetConfig(repo string, prefix string) (*Config, error) {
	if err := os.Chdir(repo); err != nil {
		return nil, err
	}
	cmd := exec.Command(
		"git",
		"config",
		"--list",
		"-z")

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading git configuration: %w", err)
	}

	config := Config{
		Prefix: prefix,
	}

	for len(out) > 0 {
		keyEnd := bytes.IndexByte(out, '\n')
		if keyEnd == -1 {
			return nil, errors.New("invalid output from 'git config'")
		}
		key := string(out[:keyEnd])
		out = out[keyEnd+1:]
		valueEnd := bytes.IndexByte(out, 0)
		if valueEnd == -1 {
			return nil, errors.New("invalid output from 'git config'")
		}
		value := string(out[:valueEnd])
		out = out[valueEnd+1:]

		ok, rest := configKeyMatchesPrefix(key, prefix)
		if !ok {
			continue
		}

		entry := ConfigEntry{
			Key:   rest,
			Value: value,
		}
		config.Entries = append(config.Entries, entry)
	}

	return &config, nil
}

// configKeyMatchesPrefix checks whether `key` starts with `prefix` at
// a component boundary (i.e., at a '.'). If yes, it returns `true`
// and the part of the key after the prefix; e.g.:
//
//	configKeyMatchesPrefix("foo.bar", "foo") → true, "bar"
//	configKeyMatchesPrefix("foo.bar", "foo.") → true, "bar"
//	configKeyMatchesPrefix("foo.bar", "foo.bar") → true, ""
//	configKeyMatchesPrefix("foo.bar", "foo.bar.") → false, ""
func configKeyMatchesPrefix(key, prefix string) (bool, string) {
	if prefix == "" {
		return true, key
	}
	if !strings.HasPrefix(key, prefix) {
		return false, ""
	}

	if prefix[len(prefix)-1] == '.' {
		return true, key[len(prefix):]
	}
	if len(key) == len(prefix) {
		return true, ""
	}
	if key[len(prefix)] == '.' {
		return true, key[len(prefix)+1:]
	}
	return false, ""
}
