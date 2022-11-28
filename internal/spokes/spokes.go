package spokes

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/github/go-pipe/pipe"
	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/github/spokes-receive-pack/internal/pktline"
	"golang.org/x/sync/errgroup"
)

const (
	capabilities = "report-status delete-refs side-band-64k ofs-delta"
	// maximum length of a pkt-line's data component
	maxPacketDataLength = 65516
	nullSHA1OID         = "0000000000000000000000000000000000000000"
	nullSHA256OID       = "000000000000000000000000000000000000000000000000000000000000"
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
		output:   output,
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
	commands, capabilities, err := r.readCommands(ctx)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		return nil
	}

	// Now that we have all the commands sent by the client side, we are ready to process them and read the
	// corresponding packfiles
	var unpackErr error
	if unpackErr := r.readPack(ctx, commands, capabilities); unpackErr != nil {
		for i := range commands {
			commands[i].err = fmt.Sprintf("error processing packfiles: %s", unpackErr.Error())
		}
	} else {
		// We have successfully processed the pack-files, let's check their connectivity
		if err := r.performCheckConnectivity(ctx, commands); err != nil {
			for _, c := range commands {
				if err := r.performCheckConnectivityOnObject(ctx, c.newOID); err != nil {
					// Some references have missing objects, let's check them one by one to determine
					// the ones actually failing
					c.err = fmt.Sprintf("missing required objects: %s", err.Error())
				}
			}
		}
	}

	if err := r.report(ctx, unpackErr == nil, commands); err != nil {
		return err
	}

	return nil
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
		if err := writePacketf(r.output, "%s\x00%s\n", references[0], capabilities); err != nil {
			return fmt.Errorf("writing capability packet: %w", err)
		}

		for i := 1; i < len(references); i++ {
			if err := writePacketf(r.output, "%s\n", references[i]); err != nil {
				return fmt.Errorf("writing ref advertisement packet: %w", err)
			}
		}
	} else {
		if err := writePacketf(r.output, "%s capabilities^{}\x00%s", nullSHA1OID, capabilities); err != nil {
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
func writePacketLine(w io.Writer, data []byte) error {
	if len(data) > maxPacketDataLength {
		return fmt.Errorf("data exceeds maximum pkt-line length: %d", len(data))
	}
	if _, err := fmt.Fprintf(w, "%04x", 4+len(data)); err != nil {
		return fmt.Errorf("writing packet length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("writing packet: %w", err)
	}
	return nil
}

// writePacketf formats the given data then writes the result to the output stored in the `SpokesReceivePack`
// as a pkt-line.
func writePacketf(w io.Writer, format string, a ...interface{}) error {
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
	return writePacketLine(w, buf.Bytes())
}

type command struct {
	refname string
	oldOID  string
	newOID  string
	err     string
}

var validReferenceName = regexp.MustCompile(`^([0-9a-f]{40,64}) ([0-9a-f]{40,64}) (.+)`)

// readCommands reads the set of ref update commands sent by the client side.
func (r *SpokesReceivePack) readCommands(_ context.Context) ([]command, pktline.Capabilities, error) {
	var commands []command

	first := true
	pl := pktline.New()
	var capabilities pktline.Capabilities

	for {
		err := pl.Read(r.input)
		if err != nil {
			return nil, pktline.Capabilities{}, fmt.Errorf("reading commands: %w", err)
		}

		if pl.IsFlush() {
			break
		}

		if first {
			capabilities, err = pl.Capabilities()
			if err != nil {
				return nil, capabilities, fmt.Errorf("processing capabilities: %w", err)
			}
			first = false
		}

		if m := validReferenceName.FindStringSubmatch(string(pl.Payload)); m != nil {
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

		return nil, capabilities, fmt.Errorf("bogus command: %s", pl.Payload)
	}

	return commands, capabilities, nil
}

// readPack reads a packfile from `r.input` (if one is needed) and pipes it into `git index-pack`.
// Report errors to the error sideband in `w`.
//
// If GIT_SOCKSTAT_VAR_quarantine_dir is not specified, the pack will be written to objects/pack/ directory within the
// current Git repository with a  default name determined from the pack content
func (r *SpokesReceivePack) readPack(ctx context.Context, commands []command, capabilities pktline.Capabilities) error {
	// We only get a pack if there are non-deletes.
	if !includeNonDeletes(commands) {
		return nil
	}

	// Index-pack will read directly from our input!
	cmd := exec.CommandContext(
		ctx,
		"git",
		"index-pack",
		"--fix-thin",
		"--stdin",
		"-v",
	)

	if quarantine := os.Getenv("GIT_SOCKSTAT_VAR_quarantine_dir"); quarantine != "" {
		if err := os.Mkdir(quarantine, 0700); err != nil {
			return err
		}
		file := fmt.Sprintf("quarantine-%d.pack", time.Now().UnixNano())
		cmd.Args = append(cmd.Args, filepath.Join(quarantine, file))
	}

	// We want to discard stdout but forward stderr to `w`
	// Depending on the sideband capability we would need to do it in a sideband
	cmd.Stdin = r.input
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating pipe for 'index-pack' stderr: %w", err)
	}

	eg, err := startSidebandMultiplexer(stderr, r.output, capabilities)
	if err != nil {
		// Sideband has been requested, but we haven't been able to deal with it
		return err
	}

	if err = cmd.Start(); err != nil {
		if eg != nil {
			_ = eg.Wait()
		}
		return fmt.Errorf("starting 'index-pack': %w", err)
	}

	if eg != nil {
		_ = eg.Wait()
	} else {
		_, _ = io.Copy(r.err, stderr)
	}

	return nil
}

// startSidebandMultiplexer checks if a sideband capability has been required and, in that case, starts multiplexing the
// stderr of the command `cmd` into the indicated `output`
func startSidebandMultiplexer(stderr io.ReadCloser, output io.Writer, capabilities pktline.Capabilities) (*errgroup.Group, error) {
	_, sbDefined := capabilities.Get(pktline.SideBand)
	_, sb64kDefined := capabilities.Get(pktline.SideBand64k)

	if !sbDefined && !sb64kDefined {
		// no sideband capability has been defined
		return nil, nil
	}

	var eg errgroup.Group

	eg.Go(
		func() error {
			defer func() {
				_ = stderr.Close()
			}()
			for {
				var bufferSize = 999
				if sb64kDefined {
					bufferSize = 65519
				}
				buf := make([]byte, bufferSize)

				n, err := stderr.Read(buf[:])
				if n != 0 {
					if err := writePacketf(output, "\x02%s", buf[:n]); err != nil {
						return fmt.Errorf("writing to error sideband: %w", err)
					}
				}
				if err != nil {
					if err == io.EOF {
						return nil
					}
					return fmt.Errorf("reading 'index-pack' stderr: %w", err)
				}
			}
		},
	)

	return &eg, nil
}

// performCheckConnectivity checks that the "new" oid provided in `commands` are
// closed under reachability, stopping the traversal at any objects
// reachable from the pre-existing reference values.
func (r *SpokesReceivePack) performCheckConnectivity(ctx context.Context, commands []command) error {
	devNull, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return fmt.Errorf("opening %s: %w", os.DevNull, err)
	}
	defer func() {
		_ = devNull.Close()
	}()

	cmd := exec.CommandContext(
		ctx,
		"git",
		"rev-list",
		"--objects",
		"--no-object-names",
		"--stdin",
		"--not",
		"--all",
		"--alternate-refs",
	)
	cmd.Stderr = devNull

	p := pipe.New(pipe.WithDir("."), pipe.WithStdout(devNull))
	p.Add(
		pipe.Function(
			"write-new-values",
			func(ctx context.Context, _ pipe.Env, input io.Reader, output io.Writer) error {
				w := bufio.NewWriter(output)

				for _, c := range commands {
					if c.newOID == nullSHA1OID || c.newOID == nullSHA256OID {
						continue
					}
					if _, err := fmt.Fprintln(w, c.newOID); err != nil {
						return fmt.Errorf("writing to 'rev-list' input: %w", err)
					}
				}

				if err := w.Flush(); err != nil {
					return fmt.Errorf("flushing stdin to 'rev-list': %w", err)
				}

				return nil
			},
		),
		pipe.CommandStage("rev-list", cmd),
	)

	if err := p.Run(ctx); err != nil {
		return fmt.Errorf("running 'rev-list': %w", err)
	}

	return nil
}

func (r *SpokesReceivePack) performCheckConnectivityOnObject(ctx context.Context, oid string) error {
	cmd := exec.CommandContext(
		ctx,
		"git",
		"rev-list",
		"--objects",
		"--no-object-names",
		"--not",
		"--all",
		"--alternate-refs",
		oid,
	)

	if err := cmd.Run(); err != nil {
		return fmt.Errorf("running 'rev-list' on oid %s: %s", oid, err)
	}

	return nil
}

// report the success/failure of the push operation to the client
func (r *SpokesReceivePack) report(_ context.Context, unpackOK bool, commands []command) error {
	var buf bytes.Buffer
	if unpackOK {
		if err := writePacketLine(&buf, []byte("unpack ok")); err != nil {
			return err
		}
	} else {
		if err := writePacketLine(&buf, []byte("unpack index-pack failed")); err != nil {
			return err
		}
	}
	for _, c := range commands {
		if c.err != "" {
			if err := writePacketf(&buf, "ng %s %s\n", c.refname, c.err); err != nil {
				return err
			}
		} else {
			if err := writePacketf(&buf, "ok %s\n", c.refname); err != nil {
				return err
			}
		}
	}

	if _, err := fmt.Fprint(&buf, "0000"); err != nil {
		return err
	}

	output := buf.Bytes()

	for len(output) > 0 {
		n := 4096
		if len(output) < n {
			n = len(output)
		}
		if err := writePacketf(r.output, "\x01%s", output[:n]); err != nil {
			return fmt.Errorf("writing output to client: %w", err)
		}
		output = output[n:]
	}
	if _, err := fmt.Fprintf(r.output, "0000"); err != nil {
		return nil
	}
	return nil
}

// includeNonDeletes returns true iff `commands` includes any
// non-delete commands.
func includeNonDeletes(commands []command) bool {
	for _, c := range commands {
		if c.newOID != nullSHA1OID && c.newOID != nullSHA256OID {
			return true
		}
	}
	return false
}
