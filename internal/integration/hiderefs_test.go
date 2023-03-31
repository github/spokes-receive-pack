//go:build integration

package integration

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHiderefsConfig(t *testing.T) {
	const (
		defaultBranch  = "refs/heads/main"
		transferhide1  = "refs/__transferhiderefs1/example/anything"
		transferhide2  = "refs/__transferhiderefs2/example/anything"
		transferunhide = "refs/__transferhiderefs2/exception/anything" // nested inside of __transferhiderefs2
		receivehide1   = "refs/__receivehiderefs1/example/anything"
		receivehide2   = "refs/__receivehiderefs2/example/anything"
		uploadhide     = "refs/__uploadhiderefs/example/anything"

		createBranch         = "refs/heads/newbranch"
		createtransferhide1  = "refs/__transferhiderefs1/example/new"
		createtransferhide2  = "refs/__transferhiderefs2/example/new"
		createtransferunhide = "refs/__transferhiderefs2/exception/new" // nested inside of __transferhiderefs2
		createreceivehide1   = "refs/__receivehiderefs1/example/new"
		createreceivehide2   = "refs/__receivehiderefs2/example/new"
		createuploadhide     = "refs/__uploadhiderefs/example/new"

		// This needs to be reachable from refs/heads/main
		testCommit = "e589bdee50e39beac56220c4b7a716225f79e3cf"

		gitConfigParameters = `'transfer.hideRefs=refs/__transferhiderefs2' ` +
			`'transfer.hideRefs='\!'refs/__transferhiderefs2/exception' ` +
			`'receive.hideRefs=refs/__receivehiderefs2'`
	)

	wd, err := os.Getwd()
	require.NoError(t, err)
	origin := filepath.Join(wd, "testdata/remote/git-internals-fork.git")

	testRepo := t.TempDir()
	requireRun(t, "git", "init", "--bare", testRepo)
	requireRun(t, "git", "-C", testRepo, "fetch", origin, defaultBranch+":"+defaultBranch)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferhide1, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferhide2, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", transferunhide, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", receivehide1, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", receivehide2, testCommit)
	requireRun(t, "git", "-C", testRepo, "update-ref", uploadhide, testCommit)
	requireRun(t, "git", "-C", testRepo, "config", "transfer.hiderefs", "refs/__transferhiderefs1")
	requireRun(t, "git", "-C", testRepo, "config", "receive.hiderefs", "refs/__receivehiderefs1")
	requireRun(t, "git", "-C", testRepo, "config", "uploadpack.hiderefs", "refs/__uploadhiderefs")

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	srp := exec.CommandContext(ctx, "spokes-receive-pack", ".")
	srp.Dir = testRepo
	srp.Env = append(os.Environ(),
		"GIT_CONFIG_PARAMETERS="+gitConfigParameters,
		"GIT_SOCKSTAT_VAR_spokes_quarantine=bool:true",
		"GIT_SOCKSTAT_VAR_quarantine_id=config-test-quarantine-id")
	srp.Stderr = os.Stderr
	srpIn, err := srp.StdinPipe()
	require.NoError(t, err)
	srpOut, err := srp.StdoutPipe()
	require.NoError(t, err)

	srpErr := make(chan error)
	go func() { srpErr <- srp.Run() }()

	bufSRPOut := bufio.NewReader(srpOut)

	refs, _, err := readAdv(bufSRPOut)
	require.NoError(t, err)
	assert.Contains(t, refs, defaultBranch)
	assert.NotContains(t, refs, transferhide1)
	assert.NotContains(t, refs, transferhide2)
	assert.Contains(t, refs, transferunhide)
	assert.NotContains(t, refs, receivehide1)
	assert.NotContains(t, refs, receivehide2)
	assert.Contains(t, refs, uploadhide)

	oldnew := fmt.Sprintf("%040d %s", 0, testCommit)
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\x00report-status report-status-v2 side-band-64k object-format=sha1\n", oldnew, createBranch))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferhide1))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferhide2))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createtransferunhide))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createreceivehide1))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createreceivehide2))
	require.NoError(t, writePktlinef(srpIn,
		"%s %s\n", oldnew, createuploadhide))
	_, err = srpIn.Write([]byte("0000"))
	require.NoError(t, err)

	packObjects := exec.CommandContext(ctx, "git", "-C", testRepo, "pack-objects", "--all-progress-implied", "--revs", "--stdout", "--thin", "--delta-base-offset", "--progress")
	// no stdin
	packObjects.Stderr = os.Stderr
	pack, err := packObjects.StdoutPipe()
	require.NoError(t, err)
	go packObjects.Run()
	_, err = io.Copy(srpIn, pack)
	require.NoError(t, err)

	// No packfile, we're updating with objects that are already there and
	// don't need a pack for this test.
	require.NoError(t, srpIn.Close())

	refStatus, unpackRes, _, err := readResult(bufSRPOut)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		createBranch:         "ok",
		createtransferhide1:  "ng deny updating a hidden ref",
		createtransferhide2:  "ng deny updating a hidden ref",
		createtransferunhide: "ok",
		createreceivehide1:   "ng deny updating a hidden ref",
		createreceivehide2:   "ng deny updating a hidden ref",
		createuploadhide:     "ok",
	}, refStatus)
	assert.Equal(t, "unpack ok\n", unpackRes)
}

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

func readResult(r io.Reader) (map[string]string, string, [][]byte, error) {
	var (
		refStatus map[string]string
		unpackRes string
		sideband  [][]byte
	)

	// Read all of the output so that we can include it with errors.
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, "", nil, err
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
	log.Printf("read pkt size: %s", sizeBuf)

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
	log.Printf("read pkt data: %q", string(buf[:n]))
	return buf, err
}

func writePktlinef(w io.Writer, format string, args ...interface{}) error {
	msg := fmt.Sprintf(format, args...)
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
