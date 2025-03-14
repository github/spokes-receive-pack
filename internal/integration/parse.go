//go:build integration

package integration

import (
	"bytes"
	"fmt"
	"io"
	"os/exec"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func readAdv(r io.Reader) (map[string]string, string, error) {
	caps := ""
	refs := make(map[string]string)
	firstLine := true
	for {
		data, err := readPktline(r)
		if err != nil {
			return nil, "", err
		}
		if data == nil {
			return refs, caps, nil
		}

		if firstLine {
			parts := bytes.SplitN(data, []byte{0}, 2)
			if len(parts) != 2 {
				return nil, "", fmt.Errorf("expected capabilities on first line of ref advertisement %q", string(data))
			}
			data = parts[0]
			caps = string(parts[1])
			firstLine = false
		}

		parts := bytes.SplitN(data, []byte(" "), 2)
		if len(parts) != 2 || len(parts[0]) != 40 {
			return nil, "", fmt.Errorf("bad advertisement line: %q", string(data))
		}
		oid := string(parts[0])
		refName := strings.TrimSuffix(string(parts[1]), "\n")
		if _, ok := refs[refName]; ok {
			return nil, "", fmt.Errorf("duplicate entry for %q", refName)
		}
		refs[refName] = oid
	}
}

func readResult(t *testing.T, r io.Reader) (map[string]string, string, [][]byte, error) {
	var (
		refStatus map[string]string
		unpackRes string
		sideband  [][]byte
	)

	// Read all of the output so that we can include it with errors.
	data, err := io.ReadAll(r)
	if err != nil {
		if len(data) > 0 {
			t.Logf("got data, but there was an error: %v", err)
		} else {
			return nil, "", nil, err
		}
	}

	// Replace r.
	r = bytes.NewReader(data)

	for {
		pkt, err := readPktline(r)
		switch {
		case err != nil:
			return nil, "", nil, fmt.Errorf("%w while parsing %q", err, string(data))

		case pkt == nil:
			if refStatus == nil {
				return nil, "", nil, fmt.Errorf("no sideband 1 packet in %q", string(data))
			}
			return refStatus, unpackRes, sideband, nil

		case bytes.HasPrefix(pkt, []byte{1}):
			if refStatus != nil {
				return nil, "", nil, fmt.Errorf("repeated sideband 1 packet in %q", string(data))
			}
			refStatus, unpackRes, err = parseSideband1(pkt[1:])
			if err != nil {
				return nil, "", nil, err
			}

		case bytes.HasPrefix(pkt, []byte{2}):
			sideband = append(sideband, append([]byte{}, data...))

		default:
			return nil, "", nil, fmt.Errorf("todo: handle %q from %q", string(pkt), string(data))
		}
	}
}

func parseSideband1(data []byte) (map[string]string, string, error) {
	refs := make(map[string]string)
	unpack := ""

	r := bytes.NewReader(data)

	for {
		pkt, err := readPktline(r)
		switch {
		case err != nil:
			return nil, "", fmt.Errorf("%w while parsing sideband 1 packet %q", err, string(data))

		case pkt == nil:
			return refs, unpack, nil

		case bytes.HasPrefix(pkt, []byte("unpack ")):
			unpack = unpack + string(pkt)

		case bytes.HasPrefix(pkt, []byte("ng ")):
			parts := bytes.SplitN(bytes.TrimSuffix(pkt[3:], []byte("\n")), []byte(" "), 2)
			if len(parts) == 2 {
				refs[string(parts[0])] = "ng " + string(parts[1])
			} else {
				refs[string(parts[0])] = "ng"
			}

		case len(pkt) > 3 && pkt[2] == ' ':
			refs[string(bytes.TrimSuffix(pkt[3:], []byte("\n")))] = string(pkt[0:2])

		default:
			return nil, "", fmt.Errorf("unrecognized status %q in sideband 1 packet %q", string(pkt), string(data))
		}
	}
}

func readPktline(r io.Reader) ([]byte, error) {
	sizeBuf := make([]byte, 4)
	n, err := r.Read(sizeBuf)
	if err != nil {
		return nil, err
	}
	if n != 4 {
		return nil, fmt.Errorf("expected 4 bytes but got %d (%s)", n, sizeBuf[:n])
	}

	size, err := strconv.ParseUint(string(sizeBuf), 16, 16)
	if err != nil {
		return nil, err
	}

	if size == 0 {
		return nil, nil
	}
	if size < 4 {
		return nil, fmt.Errorf("invalid length %q", sizeBuf)
	}

	buf := make([]byte, size-4)
	n, err = io.ReadFull(r, buf)
	return buf, err
}

func writePktlinef(w io.Writer, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
	err := writePktline(w, msg)
	return err
}

func writePktline(w io.Writer, msg string) error {
	_, err := fmt.Fprintf(w, "%04x%s", 4+len(msg), msg)
	return err
}

func requireRun(t *testing.T, program string, args ...string) {
	t.Logf("run %s %v", program, args)
	cmd := exec.Command(program, args...)
	out, err := cmd.CombinedOutput()
	if len(out) > 0 {
		t.Logf("%s", out)
	}
	require.NoError(t, err, "%s %v:\n%s", program, args, out)
}
