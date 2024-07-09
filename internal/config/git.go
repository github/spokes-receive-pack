package config

import (
	"bytes"
	"errors"
	"fmt"
	"os/exec"
	"strconv"
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
	// Entries contains the configuration entries that matched
	// `Prefix`, in the order that they are reported by `git config
	// --list`.
	Entries []ConfigEntry
}

// GetConfig returns the entries from gitconfig in the repo located at repo.
func GetConfig(repo string) (*Config, error) {
	cmd := exec.Command(
		"git",
		"config",
		"--list",
		"-z")
	cmd.Dir = repo

	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("reading git configuration: %w", err)
	}

	config := &Config{}

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

		entry := ConfigEntry{
			Key:   key,
			Value: value,
		}
		config.Entries = append(config.Entries, entry)
	}

	return config, nil
}

// Get returns the last entry in the list for the request config setting or an empty string in case
// it cannot be found
func (c *Config) Get(name string) string {
	name = strings.ToLower(name)
	value := ""
	for _, entry := range c.Entries {
		if entry.Key == name {
			value = entry.Value
		}
	}

	return value
}

// GetAll returns all values for the requested config setting.
func (c *Config) GetAll(name string) []string {
	name = strings.ToLower(name)
	var res []string
	for _, entry := range c.Entries {
		if entry.Key == name {
			res = append(res, entry.Value)
		}
	}
	return res
}
func (c *Config) GetPrefix(prefix string) map[string][]string {
	var m = make(map[string][]string)
	for _, entry := range c.Entries {
		if strings.HasPrefix(entry.Key, prefix) {
			trimmedKey := strings.TrimPrefix(entry.Key, prefix)
			m[trimmedKey] = append(m[trimmedKey], entry.Value)
		}
	}
	return m
}

// ParseSigned parses a string that may contain a signed integer with an
// optional suffix (either 'k', 'm', or 'g' for their respective IEC values).
func ParseSigned(str string) (int, error) {
	factor := 1

	if len(str) > 0 {
		switch str[len(str)-1] {
		case 'k', 'K':
			factor = 1024
		case 'm', 'M':
			factor = 1024 * 1024
		case 'g', 'G':
			factor = 1024 * 1024 * 1024
		}

		if factor != 1 {
			str = str[:len(str)-1]
		}
	}

	n, err := strconv.Atoi(str)
	if err != nil {
		return 0, err
	}

	return n * factor, nil
}
