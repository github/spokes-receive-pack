package spokes

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/github/go-pipe/pipe"
	"github.com/github/spokes-receive-pack/internal/config"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

const (
	capabilities = "report-status delete-refs side-band-64k ofs-delta"
	// maximum length of a pkt-line's data component
	maxPacketDataLength = 65516
	nullOID             = "0000000000000000000000000000000000000000"
)

// SpokesReceivePack is used to model our own impl of the git-receive-pack
type SpokesReceivePack struct {
	input    io.Reader
	output   io.Writer
	err      io.Writer
	repoPath string
}

// NewSpokesReceivePack returns a pointer to a SpokesReceivePack executor
func NewSpokesReceivePack(input io.Reader, output, err io.Writer, repoPath string) *SpokesReceivePack {
	return &SpokesReceivePack{
		input:    input,
		output:   bufio.NewWriter(output),
		err:      err,
		repoPath: repoPath,
	}
}

// Execute executes our custom implementation
// It tries to model the behaviour described in the "Pushing Data To a Server" section of the
// https://github.com/github/git/blob/github/Documentation/technical/pack-protocol.txt document
func (r *SpokesReceivePack) Execute(ctx context.Context) error {
	if err := os.Chdir(r.repoPath); err != nil {
		return fmt.Errorf("unable to enter repo at location: %s", r.repoPath)
	}

	// Reference discovery phase
	if err := r.performReferenceDiscovery(ctx); err != nil {
		return err
	}

	// At this point the client knows what references the server is at, so it can send a
	//list of reference update requests.  For each reference on the server
	//that it wants to update, it sends a line listing the obj-id currently on
	//the server, the obj-id the client would like to update it to and the name
	//of the reference.
	commands, _, err := r.readCommands(ctx)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		return nil
	}

	panic("Not complete yet!")
}

// performReferenceDiscovery performs the reference discovery bits of the protocol
// It writes back to the client the capability listing and a packet-line for every reference
// terminated with a flush-pkt
func (r *SpokesReceivePack) performReferenceDiscovery(ctx context.Context) error {
	config, err := config.GetConfig(r.repoPath, "receive.hiderefs")
	if err != nil {
		return err
	}

	references := make([][]byte, 0, 100)
	p := pipe.New(pipe.WithDir("."), pipe.WithStdout(r.output))
	p.Add(
		pipe.Command("git", "for-each-ref", "--format=%(objectname) %(refname)"),
		pipe.LinewiseFunction(
			"collect-references",
			func(ctx context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
				// Ignore the current line if it is a hidden ref
				if !isHiddenRef(line, config.Entries) {
					references = append(references, line)
				}

				return nil
			},
		),
	)
	// Collect the reference tips present in the parent repo in case this is a fork
	parentRepoId := os.Getenv("GIT_SOCKSTAT_VAR_parent_repo_id")
	advertiseTags := os.Getenv("GIT_NW_ADVERTISE_TAGS")

	if parentRepoId != "" {
		patterns := fmt.Sprintf("refs/remotes/%s/heads", parentRepoId)
		if advertiseTags != "" {
			patterns += fmt.Sprintf(" refs/remotes/%s/tags", parentRepoId)
		}

		network, err := r.networkRepoPath()
		// if the path in the objects/info/alternates is correct
		if err == nil {
			p.Add(
				pipe.Command(
					"git",
					fmt.Sprintf("--git-dir=%s", network),
					"for-each-ref",
					"--format=%(objectname) .have",
					patterns),
				pipe.LinewiseFunction(
					"collect-alternates-references",
					func(ctx context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
						// Ignore the current line if it is a hidden ref
						if !isHiddenRef(line, config.Entries) {
							references = append(references, line)
						}

						return nil
					},
				),
			)
		}
	}

	if err := p.Run(ctx); err != nil {
		return fmt.Errorf("collecting references: %w", err)
	}

	if len(references) > 0 {
		if err := r.writePacketf("%s\x00%s\n", references[0], capabilities); err != nil {
			return fmt.Errorf("writing capability packet: %w", err)
		}

		for i := 1; i < len(references); i++ {
			if err := r.writePacketf("%s\n", references[i]); err != nil {
				return fmt.Errorf("writing ref advertisement packet: %w", err)
			}
		}
	} else {
		if _, err := fmt.Fprintf(r.output, "%s capabilities^{}\x00%s", nullOID, capabilities); err != nil {
			return fmt.Errorf("writing lonely capability packet: %w", err)
		}
	}

	if _, err := fmt.Fprintf(r.output, "0000"); err != nil {
		return fmt.Errorf("writing flush packet: %w", err)
	}

	return nil
}

func (r *SpokesReceivePack) networkRepoPath() (string, error) {
	alternatesPath := filepath.Join(r.repoPath, "objects", "info", "alternates")
	alternatesBytes, err := os.ReadFile(alternatesPath)
	if err != nil {
		return "", fmt.Errorf("could not read objects/info/alternates of '%s': %w", r.repoPath, err)
	}
	alternates := strings.TrimSuffix(string(alternatesBytes), "\n")

	if !filepath.IsAbs(alternates) {
		alternates, err = filepath.Abs(filepath.Join(r.repoPath, "objects", alternates))
		if err != nil {
			return "", fmt.Errorf("could not get absolute repo path: %w", err)
		}
	}

	fi, err := os.Stat(alternates)
	if err != nil {
		return "", err
	}

	if !fi.IsDir() {
		return "", fmt.Errorf("alternates path is not a directory: %v", alternates)
	}

	if !strings.HasPrefix(alternates, filepath.Dir(r.repoPath)) {
		return "", fmt.Errorf("alternates and repo are not in the same parent directory")
	}

	return alternates, nil
}

// isHiddenRef determines if the line passed as the first argument belongs to the list of
// potential references that we don't want to advertise
// This method assumes the config entries passed as a second argument are the ones in the `receive.hiderefs` section
func isHiddenRef(line []byte, entries []config.ConfigEntry) bool {
	l := string(line)
	for _, entry := range entries {
		if strings.Contains(l, entry.Value) {
			return true
		}
	}
	return false
}

// writePacket writes `data` to the `r.output` as a pkt-line.
func (r *SpokesReceivePack) writePacketLine(data []byte) error {
	if len(data) > maxPacketDataLength {
		return fmt.Errorf("data exceeds maximum pkt-line length: %d", len(data))
	}
	if _, err := fmt.Fprintf(r.output, "%04x", 4+len(data)); err != nil {
		return fmt.Errorf("writing packet length: %w", err)
	}
	if _, err := r.output.Write(data); err != nil {
		return fmt.Errorf("writing packet: %w", err)
	}
	return nil
}

// writePacketf formats the given data then writes the result to the output stored in the `SpokesReceivePack`
// as a pkt-line.
func (r *SpokesReceivePack) writePacketf(format string, a ...interface{}) error {
	var buf bytes.Buffer
	if _, err := fmt.Fprintf(&buf, format, a...); err != nil {
		return fmt.Errorf("formatting packet: %w", err)
	}

	// According to the pkt-line spec:
	//
	// > Implementations SHOULD NOT send an empty pkt-line ("0004").
	if buf.Len() == 0 {
		return nil
	}
	return r.writePacketLine(buf.Bytes())
}

type command struct {
	refname string
	oldOID  string
	newOID  string
}

var validReferenceName = regexp.MustCompile(`^([0-9a-f]{40}) ([0-9a-f]{40}) (.+)`)

// readCommands reads the set of ref update commands sent by the client side.
func (r *SpokesReceivePack) readCommands(_ context.Context) ([]command, []string, error) {
	var commands []command
	var clientCaps []string

	first := true

	for {
		data, err := r.readPacket()
		if err != nil {
			return nil, nil, fmt.Errorf("reading commands: %w", err)
		}

		if data == nil {
			// That signifies a flush packet.
			break
		}

		if first {
			if i := bytes.IndexByte(data, 0); i != -1 {
				clientCaps = strings.Split(string(data[i+1:]), " ")
				data = data[:i]
			}
			first = false
		}

		if m := validReferenceName.FindStringSubmatch(string(data)); m != nil {
			commands = append(
				commands,
				command{
					oldOID:  m[1],
					newOID:  m[2],
					refname: m[3],
				},
			)
			continue
		}

		return nil, nil, fmt.Errorf("bogus command: %s", data)
	}

	return commands, clientCaps, nil
}

// readPacket reads and returns the data from one packet.
// `flush` packet returns `nil,nil`
// `zero-length` packet, return a zero-length (but not nil) byte slice and a nil error.
func (r *SpokesReceivePack) readPacket() ([]byte, error) {
	var lenBytes [4]byte
	// Read the packet length
	if _, err := io.ReadFull(r.input, lenBytes[:]); err != nil {
		return nil, fmt.Errorf("reading packet length: %w", err)
	}
	l, err := strconv.ParseUint(string(lenBytes[:]), 16, 16)
	if err != nil {
		return nil, fmt.Errorf("parsing packet length: %w", err)
	}

	if l == 0 {
		// That was a flush packet.
		return nil, nil
	}

	if l < 4 {
		// Packet lengths needs to be 4 bytes
		return nil, fmt.Errorf("invalid packet length: %d", l)
	}

	// We are ready to read the data itself
	data := make([]byte, int(l-4))
	if _, err := io.ReadFull(r.input, data); err != nil {
		return nil, fmt.Errorf("reading packet data: %w", err)
	}
	return data, nil
}
