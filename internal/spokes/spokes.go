package spokes

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/github/go-pipe/pipe"
	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/github/spokes-receive-pack/internal/pktline"
	"golang.org/x/sync/errgroup"
)

const (
	capabilities = "report-status delete-refs side-band-64k ofs-delta atomic push-options object-format=sha1"
	// maximum length of a pkt-line's data component
	maxPacketDataLength = 65516
	nullSHA1OID         = "0000000000000000000000000000000000000000"
	nullSHA256OID       = "000000000000000000000000000000000000000000000000000000000000"
)

// SpokesReceivePack is used to model our own impl of the git-receive-pack
type SpokesReceivePack struct {
	input            io.Reader
	output           io.Writer
	err              io.Writer
	capabilities     string
	repoPath         string
	statelessRPC     bool
	advertiseRefs    bool
	quarantineFolder string
}

// NewSpokesReceivePack returns a pointer to a SpokesReceivePack executor
func NewSpokesReceivePack(input io.Reader, output, err io.Writer, args []string, version string) (*SpokesReceivePack, error) {
	statelessRPC := flag.Bool("stateless-rpc", false, "Indicates we are using the HTTP protocol")
	httpBackendInfoRefs := flag.Bool("http-backend-info-refs", false, "Indicates we only need to announce the references")
	flag.BoolVar(httpBackendInfoRefs, "advertise-refs", *httpBackendInfoRefs, "alias of --http-backend-info-refs")
	flag.Parse()

	if flag.NArg() != 1 {
		return nil, fmt.Errorf("Unexpected number of keyword args (%d). Expected repository name, got %s ", flag.NArg(), flag.Args())
	}
	repoPath := flag.Args()[0]

	return &SpokesReceivePack{
		input:         input,
		output:        output,
		err:           err,
		capabilities:  capabilities + fmt.Sprintf(" agent=github/spokes-receive-pack-%s", version),
		repoPath:      repoPath,
		statelessRPC:  *statelessRPC,
		advertiseRefs: *httpBackendInfoRefs,
	}, nil
}

// Execute executes our custom implementation
// It tries to model the behaviour described in the "Pushing Data To a Server" section of the
// https://github.com/github/git/blob/github/Documentation/technical/pack-protocol.txt document
func (r *SpokesReceivePack) Execute(ctx context.Context) error {
	if err := os.Chdir(r.repoPath); err != nil {
		return fmt.Errorf("unable to enter repo at location: %s", r.repoPath)
	}

	// Reference discovery phase
	// We only need to perform the references discovery when we are not using the HTTP protocol or, if we are using it,
	// we only run the discovery phase when the http-backend-info-refs/advertise-refs option has been set
	if r.advertiseRefs || !r.statelessRPC {
		if err := r.performReferenceDiscovery(ctx); err != nil {
			return err
		}
	}

	if r.advertiseRefs {
		// At this point we are using the HTTP protocol and the http-backend-info-refs/advertise-refs option has been set,
		// so we only need to perform the references discovery
		return nil
	}

	// At this point the client knows what references the server is at, so it can send a
	//list of reference update requests.  For each reference on the server
	//that it wants to update, it sends a line listing the obj-id currently on
	//the server, the obj-id the client would like to update it to and the name
	//of the reference.
	commands, _, capabilities, err := r.readCommands(ctx)
	if err != nil {
		return err
	}
	if len(commands) == 0 {
		return nil
	}

	// Now that we have all the commands sent by the client side, we are ready to process them and read the
	// corresponding packfiles
	var unpackErr error
	if unpackErr = r.readPack(ctx, commands, capabilities); unpackErr != nil {
		for i := range commands {
			commands[i].err = fmt.Sprintf("error processing packfiles: %s", unpackErr.Error())
			commands[i].reportFF = "ng"
		}
	} else {
		// We have successfully processed the pack-files, let's check their connectivity
		err := r.performCheckConnectivity(ctx, commands)

		// Let's check two different things for every single command:
		// * If we found a general check-connectivity error, let's check every individual command
		// * If no individual error has been found and the reportStatusFF settings is true, let's see if the reference update could be a fast-forward
		for i := range commands {
			var singleObjectErr error
			c := &commands[i]
			c.reportFF = "ok"
			if err != nil {
				singleObjectErr = r.performCheckConnectivityOnObject(ctx, c.newOID)
				if singleObjectErr != nil {
					c.err = fmt.Sprintf("missing required objects: %s", err.Error())
					c.reportFF = "ng"
				}
			}

			if singleObjectErr == nil && c.isUpdate() && r.isReportStatusFFConfigEnabled() {
				// check if a fast-forward could be performed
				if isFastForward(c, ctx) {
					c.reportFF = "ff"
				} else {
					c.reportFF = "nf"
				}
			}
		}
	}

	if capabilities.IsDefined(pktline.ReportStatus) {
		if err := r.report(ctx, unpackErr == nil, commands); err != nil {
			return err
		}
	}

	return nil
}

func isFastForward(c *command, ctx context.Context) bool {
	cmd := exec.CommandContext(
		ctx,
		"git",
		"merge-base",
		"--is-ancestor",
		c.oldOID,
		c.newOID,
	)

	if err := cmd.Run(); err != nil {
		return false
	}

	return true
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
		if err := writePacketf(r.output, "%s\x00%s\n", references[0], r.capabilities); err != nil {
			return fmt.Errorf("writing capability packet: %w", err)
		}

		for i := 1; i < len(references); i++ {
			if err := writePacketf(r.output, "%s\n", references[i]); err != nil {
				return fmt.Errorf("writing ref advertisement packet: %w", err)
			}
		}
	} else {
		if err := writePacketf(r.output, "%s capabilities^{}\x00%s", nullSHA1OID, r.capabilities); err != nil {
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

	return filepath.Dir(alternates), nil
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
	refname  string
	oldOID   string
	newOID   string
	err      string
	reportFF string
}

func (c *command) isUpdate() bool {
	return (c.oldOID != nullSHA1OID && c.oldOID != nullSHA256OID) && (c.newOID != nullSHA1OID && c.newOID != nullSHA256OID)
}

var validReferenceName = regexp.MustCompile(`^([0-9a-f]{40,64}) ([0-9a-f]{40,64}) (.+)`)

// readCommands reads the set of ref update commands sent by the client side.
func (r *SpokesReceivePack) readCommands(_ context.Context) ([]command, []string, pktline.Capabilities, error) {
	var commands []command
	var shallowInfo []string

	first := true
	pl := pktline.New()
	var capabilities pktline.Capabilities

	for {
		err := pl.Read(r.input)
		if err != nil {
			return nil, nil, pktline.Capabilities{}, fmt.Errorf("reading commands: %w", err)
		}

		if pl.IsFlush() {
			break
		}

		// Parse the shallow "commands" the client could have sent

		payload := string(pl.Payload)
		if strings.HasPrefix(payload, "shallow") {
			payloadParts := strings.Split(payload, " ")
			if len(payloadParts) != 2 {
				return nil, nil, pktline.Capabilities{}, fmt.Errorf("wrong shallow structure: %s", payload)
			}
			shallowInfo = append(shallowInfo, payloadParts[1])
			continue
		}

		if first {
			capabilities, err = pl.Capabilities()
			if err != nil {
				return nil, nil, capabilities, fmt.Errorf("processing capabilities: %w", err)
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

		return nil, nil, capabilities, fmt.Errorf("bogus command: %s", pl.Payload)
	}

	updateCommandLimit, err := r.getRefUpdateCommandLimit()
	if err != nil {
		return nil, nil, capabilities, err
	}

	if (updateCommandLimit > 0) && len(commands) > updateCommandLimit {
		return nil, nil, capabilities, fmt.Errorf("maximum ref updates exceeded: %d commands sent but max allowed is %d", len(commands), updateCommandLimit)
	}

	return commands, shallowInfo, capabilities, nil
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

	args := []string{"index-pack", "--fix-thin", "--stdin", "-v"}

	if r.isFsckConfigEnabled() {
		args = append(args, "--strict")
	}

	maxSize, err := r.getMaxInputSize()
	if err != nil {
		return err
	}

	if maxSize > 0 {
		args = append(args, fmt.Sprintf("--max-input-size=%d", maxSize))
	}

	warnObjectSize, err := r.getWarnObjectSize()
	if err != nil {
		return err
	}

	if warnObjectSize > 0 {
		args = append(args, fmt.Sprintf("--warn-object-size=%d", warnObjectSize))
	}

	// Index-pack will read directly from our input!
	cmd := exec.CommandContext(
		ctx,
		"git",
		args...,
	)

	if quarantine := os.Getenv("GIT_SOCKSTAT_VAR_quarantine_dir"); quarantine != "" {
		packDir := fmt.Sprintf("%s/pack", quarantine)
		if err := os.MkdirAll(packDir, 0700); err != nil {
			return err
		}

		cmd.Args = append(
			cmd.Args,
			filepath.Join(
				packDir,
				fmt.Sprintf("quarantine-%d.pack", time.Now().UnixNano()),
			))

		r.quarantineFolder = quarantine
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
	}

	if waitErr := cmd.Wait(); waitErr != nil {
		return waitErr
	}

	return nil
}

func (r *SpokesReceivePack) isReportStatusFFConfigEnabled() bool {
	reportStatusFF := config.GetConfigEntryValue(r.repoPath, "receive.reportStatusFF")

	return reportStatusFF == "true"

}

func (r *SpokesReceivePack) isFsckConfigEnabled() bool {
	receiveFsck := config.GetConfigEntryValue(r.repoPath, "receive.fsckObjects")
	transferFsck := config.GetConfigEntryValue(r.repoPath, "transfer.fsckObjects")

	if receiveFsck == "true" || transferFsck == "true" {
		return true
	}

	return false
}

func (r *SpokesReceivePack) getMaxInputSize() (int, error) {
	maxSize := config.GetConfigEntryValue(r.repoPath, "receive.maxsize")

	if maxSize != "" {
		return strconv.Atoi(maxSize)
	}

	return 0, nil
}

func (r *SpokesReceivePack) getWarnObjectSize() (int, error) {
	warnObjectSize := config.GetConfigEntryValue(r.repoPath, "receive.warnobjectsize")

	if warnObjectSize != "" {
		return strconv.Atoi(warnObjectSize)
	}

	return 0, nil
}

func (r *SpokesReceivePack) getRefUpdateCommandLimit() (int, error) {
	refUpdateCommandLimit := config.GetConfigEntryValue(r.repoPath, "receive.refupdatecommandlimit")

	if refUpdateCommandLimit != "" {
		return strconv.Atoi(refUpdateCommandLimit)
	}

	return 0, nil
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

func (r *SpokesReceivePack) getAlternateObjectDirsEnv() []string {
	return []string{
		fmt.Sprintf("GIT_OBJECT_DIRECTORY=%s", r.quarantineFolder),
		fmt.Sprintf("GIT_ALTERNATE_OBJECT_DIRECTORIES=%s", filepath.Join(r.repoPath, "objects")),
	}
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
	cmd.Env = append(cmd.Env, r.getAlternateObjectDirsEnv()...)

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
	cmd.Env = append(cmd.Env, r.getAlternateObjectDirsEnv()...)

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
			if err := writePacketf(&buf, "%s %s\n", c.reportFF, c.refname); err != nil {
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
