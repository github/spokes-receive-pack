package objectformat

import (
	"fmt"
	"os/exec"
	"strings"
)

const (
	NullOIDSHA1   = "0000000000000000000000000000000000000000"
	NullOIDSHA256 = "0000000000000000000000000000000000000000000000000000000000000000"
)

type ObjectFormat string

// GetObjectFormat returns the object format for the repo located at repo.
func GetObjectFormat(repo string) (ObjectFormat, error) {
	cmd := exec.Command(
		"git",
		"rev-parse",
		"--show-object-format",
	)
	cmd.Dir = repo

	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("reading git object format: %w", err)
	}

	value := strings.TrimSpace(string(out))
	switch value {
	case "sha1", "sha256":
		return ObjectFormat(value), nil
	default:
		return "", fmt.Errorf("unknown object format: %s", value)
	}
}

func (of ObjectFormat) NullOID() string {
	switch of {
	case "sha256":
		return NullOIDSHA256
	default:
		return NullOIDSHA1
	}
}
