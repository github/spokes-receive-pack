package spokes

import (
	"bufio"
	"bytes"
	"context"
	"flag"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/github/go-pipe/pipe"
	"github.com/github/spokes-receive-pack/internal/config"
	"github.com/github/spokes-receive-pack/internal/governor"
	"github.com/github/spokes-receive-pack/internal/pktline"
	"golang.org/x/sync/errgroup"
)

const (
	capabilities = "report-status report-status-v2 delete-refs side-band-64k ofs-delta atomic push-options object-format=sha1"
	// maximum length of a pkt-line's data component
	maxPacketDataLength = 65516
	nullSHA1OID         = "0000000000000000000000000000000000000000"
	nullSHA256OID       = "000000000000000000000000000000000000000000000000000000000000"
)

// Exec is similar to a main func for the new version of receive-pack.
func Exec(ctx context.Context, stdin io.Reader, stdout io.Writer, stderr io.Writer, args []string, version string) (int, error) {
	ctx, stop := signal.NotifyContext(ctx, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer stop()

	statelessRPC := flag.Bool("stateless-rpc", false, "Indicates we are using the HTTP protocol")
	httpBackendInfoRefs := flag.Bool("http-backend-info-refs", false, "Indicates we only need to announce the references")
	flag.BoolVar(httpBackendInfoRefs, "advertise-refs", *httpBackendInfoRefs, "alias of --http-backend-info-refs")
	flag.Parse()

	if flag.NArg() != 1 {
		return 1, fmt.Errorf("Unexpected number of keyword args (%d). Expected repository name, got %s ", flag.NArg(), flag.Args())
	}

	if err := os.Chdir(flag.Args()[0]); err != nil {
		return 1, fmt.Errorf("error entering repo: %w", err)
	}

	repoPath, err := os.Getwd()
	if err != nil {
		return 1, err
	}

	g, err := governor.Start(ctx, repoPath)
	if err != nil {
		return 75, err
	}
	defer g.Finish(ctx)

	config, err := config.GetConfig(".")
	if err != nil {
		g.SetError(1, err.Error())
		return 1, err
	}

	quarantineID := os.Getenv("GIT_SOCKSTAT_VAR_quarantine_id")
	if quarantineID == "" {
		err := fmt.Errorf("missing required sockstat var quarantine_id")
		g.SetError(1, err.Error())
		return 1, err
	}

	rp := &spokesReceivePack{
		input:            stdin,
		output:           stdout,
		err:              stderr,
		capabilities:     capabilities + fmt.Sprintf(" agent=github/spokes-receive-pack-%s", version),
		repoPath:         repoPath,
		config:           config,
		statelessRPC:     *statelessRPC,
		advertiseRefs:    *httpBackendInfoRefs,
		quarantineFolder: filepath.Join(repoPath, "objects", quarantineID),
		governor:         g,
	}

	if err := rp.execute(ctx); err != nil {
		g.SetError(1, err.Error())
		return 1, fmt.Errorf("unexpected error running spokes receive pack: %w", err)
	}

	return 0, nil
}

// spokesReceivePack is used to model our own impl of the git-receive-pack
type spokesReceivePack struct {
	input            io.Reader
	output           io.Writer
	err              io.Writer
	capabilities     string
	repoPath         string
	config           *config.Config
	statelessRPC     bool
	advertiseRefs    bool
	quarantineFolder string
	governor         *governor.Conn
}

// execute executes our custom implementation
// It tries to model the behaviour described in the "Pushing Data To a Server" section of the
// https://github.com/github/git/blob/github/Documentation/technical/pack-protocol.txt document
func (r *spokesReceivePack) execute(ctx context.Context) error {
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

	if capabilities.IsDefined(pktline.PushOptions) {
		// We don't use push-options here.
		if err := r.dumpPushOptions(ctx); err != nil {
			return err
		}
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
			c := &commands[i]
			if c.err != "" {
				continue
			}
			var singleObjectErr error
			c.reportFF = "ok"
			if err != nil {
				singleObjectErr = r.performCheckConnectivityOnObject(ctx, c.newOID)
				if singleObjectErr != nil {
					c.err = "missing necessary objects"
					c.reportFF = "ng"
				}
			}

			if singleObjectErr == nil && c.isUpdate() && r.isReportStatusFFConfigEnabled() {
				// check if a fast-forward could be performed
				if r.isFastForward(c, ctx) {
					c.reportFF = "ff"
				} else {
					c.reportFF = "nf"
				}
			}
		}
	}

	if capabilities.IsDefined(pktline.ReportStatusV2) || capabilities.IsDefined(pktline.ReportStatus) {
		if err := r.report(ctx, unpackErr == nil, commands, capabilities); err != nil {
			return err
		}
	}

	return unpackErr
}

func (r *spokesReceivePack) isFastForward(c *command, ctx context.Context) bool {
	cmd := exec.CommandContext(
		ctx,
		"git",
		"merge-base",
		"--is-ancestor",
		c.oldOID,
		c.newOID,
	)
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, r.getAlternateObjectDirsEnv()...)

	if err := cmd.Run(); err != nil {
		return false
	}

	return true
}

// performReferenceDiscovery performs the reference discovery bits of the protocol
// It writes back to the client the capability listing and a packet-line for every reference
// terminated with a flush-pkt
func (r *spokesReceivePack) performReferenceDiscovery(ctx context.Context) error {
	hiddenRefs := r.getHiddenRefs()

	var wroteCapabilities bool
	advertiseRef := func(line []byte) error {
		if len(line) < 41 {
			return fmt.Errorf("malformed ref line: %q", string(line))
		}

		// Ignore the current line if it is a hidden ref
		ref := strings.TrimSuffix(string(line[41:]), "\n")
		if ref != ".have" && isHiddenRef(ref, hiddenRefs) {
			return nil
		}

		if wroteCapabilities {
			if err := writePacketf(r.output, "%s\n", line); err != nil {
				return fmt.Errorf("writing ref advertisement packet: %w", err)
			}
		} else {
			wroteCapabilities = true
			if err := writePacketf(r.output, "%s\x00%s\n", line, r.capabilities); err != nil {
				return fmt.Errorf("writing capability packet: %w", err)
			}
		}

		return nil
	}

	p := pipe.New(pipe.WithDir("."), pipe.WithStdout(r.output))
	p.Add(
		pipe.Command("git", "for-each-ref", "--format=%(objectname) %(refname)"),
		pipe.LinewiseFunction(
			"collect-references",
			func(ctx context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
				return advertiseRef(line)
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
						return advertiseRef(line)
					},
				),
			)
		}
	}

	if err := p.Run(ctx); err != nil {
		return fmt.Errorf("collecting references: %w", err)
	}

	if !wroteCapabilities {
		if err := writePacketf(r.output, "%s capabilities^{}\x00%s", nullSHA1OID, r.capabilities); err != nil {
			return fmt.Errorf("writing lonely capability packet: %w", err)
		}
	}

	if _, err := fmt.Fprintf(r.output, "0000"); err != nil {
		return fmt.Errorf("writing flush packet: %w", err)
	}

	return nil
}

func (r *spokesReceivePack) getHiddenRefs() []string {
	var hiddenRefs []string
	hiddenRefs = append(hiddenRefs, r.config.GetAll("receive.hiderefs")...)
	hiddenRefs = append(hiddenRefs, r.config.GetAll("transfer.hiderefs")...)
	return hiddenRefs
}

func (r *spokesReceivePack) networkRepoPath() (string, error) {
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
func isHiddenRef(ref string, hiddenRefs []string) bool {
	isHidden := false
	for _, hr := range hiddenRefs {
		neg, strippedRef := isNegativeRef(hr)

		if strings.HasPrefix(ref, strippedRef) {
			if neg {
				isHidden = false
			} else {
				isHidden = true
			}

		}
	}
	return isHidden
}

func isNegativeRef(ref string) (bool, string) {
	if strings.HasPrefix(ref, "!") {
		return true, ref[1:]
	}
	return false, ref
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
func (r *spokesReceivePack) readCommands(_ context.Context) ([]command, []string, pktline.Capabilities, error) {
	var commands []command
	var shallowInfo []string

	first := true
	pl := pktline.New()
	var capabilities pktline.Capabilities

	hiddenRefs := r.getHiddenRefs()

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

		if m := validReferenceName.FindStringSubmatch(payload); m != nil {
			c := command{
				oldOID:  m[1],
				newOID:  m[2],
				refname: m[3],
			}
			if isHiddenRef(c.refname, hiddenRefs) {
				c.reportFF = "ng"
				c.err = "deny updating a hidden ref"
			}

			commands = append(commands, c)
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

func (r *spokesReceivePack) dumpPushOptions(ctx context.Context) error {
	pl := pktline.New()

	for {
		err := pl.Read(r.input)
		if err != nil {
			return fmt.Errorf("error reading push-options: %w", err)
		}

		if pl.IsFlush() {
			return nil
		}
	}
}

// readPack reads a packfile from `r.input` (if one is needed) and pipes it into `git index-pack`.
// Report errors to the error sideband in `w`.
func (r *spokesReceivePack) readPack(ctx context.Context, commands []command, capabilities pktline.Capabilities) error {
	// We only get a pack if there are non-deletes.
	if !includeNonDeletes(commands) {
		return nil
	}

	if err := r.makeQuarantineDirs(); err != nil {
		return err
	}

	// mimic https://github.com/git/git/blob/950264636c68591989456e3ba0a5442f93152c1a/builtin/receive-pack.c#L2252-L2273
	// and https://github.com/github/git/blob/d4a224977e032f93b1b8fd3201201f098d4f6757/builtin/receive-pack.c#L2362-L2386

	args := []string{"index-pack", "--stdin"}

	// FIXME? add --pack_header similar to git's push_header_arg

	if useSideBand(capabilities) {
		args = append(args, "--show-resolving-progress", "--report-end-of-input")
	}

	args = append(args, "--fix-thin")

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

	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, r.getAlternateObjectDirsEnv()...)

	// index-pack will read the rest of spokes-receive-pack's stdin.
	cmd.Stdin = r.input

	// Forward stderr to `w`.
	// Depending on the sideband capability we would need to do it in a sideband
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return fmt.Errorf("creating pipe for 'index-pack' stderr: %w", err)
	}

	// Collect stdout for use in reporting to governor.
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("creating pipe for 'index-pack' stdout: %w", err)
	}
	indexPackOut := make(chan []byte, 1)
	go func(r io.ReadCloser, res chan<- []byte) {
		defer close(indexPackOut)
		defer r.Close()
		out, err := io.ReadAll(r)
		if err != nil {
			log.Printf("error reading index-pack output: %v", err)
		} else {
			indexPackOut <- out
		}
	}(stdout, indexPackOut)

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

	select {
	case out, ok := <-indexPackOut:
		if ok && (bytes.HasPrefix(out, []byte("pack\t")) || bytes.HasPrefix(out, []byte("keep\t"))) {
			packID := string(bytes.TrimSpace(out[5:]))
			packPath := filepath.Join(r.quarantineFolder, "pack", "pack-"+packID+".pack")
			if info, err := os.Stat(packPath); err == nil {
				r.governor.SetReceivePackSize(info.Size())
			}
		} else {
			log.Printf("index-pack exited without telling us its packfile (%s)", out)
		}
	case <-time.After(time.Second):
		// For some reason, index-pack's output isn't available. Just move on...
		log.Print("index-pack output was too slow")
	}

	return nil
}

func (r *spokesReceivePack) isReportStatusFFConfigEnabled() bool {
	reportStatusFF := r.config.Get("receive.reportStatusFF")

	return reportStatusFF == "true"

}

func (r *spokesReceivePack) isFsckConfigEnabled() bool {
	receiveFsck := r.config.Get("receive.fsckObjects")
	transferFsck := r.config.Get("transfer.fsckObjects")

	if receiveFsck == "true" || transferFsck == "true" {
		return true
	}

	return false
}

func (r *spokesReceivePack) getMaxInputSize() (int, error) {
	maxSize := r.config.Get("receive.maxsize")

	if maxSize != "" {
		return strconv.Atoi(maxSize)
	}

	return 0, nil
}

func (r *spokesReceivePack) getWarnObjectSize() (int, error) {
	warnObjectSize := r.config.Get("receive.warnobjectsize")

	if warnObjectSize != "" {
		return strconv.Atoi(warnObjectSize)
	}

	return 0, nil
}

func (r *spokesReceivePack) getRefUpdateCommandLimit() (int, error) {
	refUpdateCommandLimit := r.config.Get("receive.refupdatecommandlimit")

	if refUpdateCommandLimit != "" {
		return strconv.Atoi(refUpdateCommandLimit)
	}

	return 0, nil
}

// startSidebandMultiplexer checks if a sideband capability has been required and, in that case, starts multiplexing the
// stderr of the command `cmd` into the indicated `output`
func startSidebandMultiplexer(stderr io.ReadCloser, output io.Writer, capabilities pktline.Capabilities) (*errgroup.Group, error) {
	if !useSideBand(capabilities) {
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
				bufferSize := sideBandBufSize(capabilities)
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

func (r *spokesReceivePack) getAlternateObjectDirsEnv() []string {
	// mimic https://github.com/git/git/blob/950264636c68591989456e3ba0a5442f93152c1a/tmp-objdir.c#L149-L153
	return []string{
		fmt.Sprintf("GIT_ALTERNATE_OBJECT_DIRECTORIES=%s", filepath.Join(r.repoPath, "objects")),
		fmt.Sprintf("GIT_OBJECT_DIRECTORY=%s", r.quarantineFolder),
		fmt.Sprintf("GIT_QUARANTINE_PATH=%s", r.quarantineFolder),
	}
}

func (r *spokesReceivePack) makeQuarantineDirs() error {
	return os.MkdirAll(filepath.Join(r.quarantineFolder, "pack"), 0700)
}

// performCheckConnectivity checks that the "new" oid provided in `commands` are
// closed under reachability, stopping the traversal at any objects
// reachable from the pre-existing reference values.
func (r *spokesReceivePack) performCheckConnectivity(ctx context.Context, commands []command) error {
	nonRejectedCommands := filterNonRejectedCommands(commands)
	if len(nonRejectedCommands) == 0 {
		// all the commands have been previously rejected so there is no need to perform
		// a connectivity check
		return nil
	}

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
	cmd.Env = append([]string{}, os.Environ()...)
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
		return fmt.Errorf("performCheckConnectivity error: %w", err)
	}

	return nil
}

func filterNonRejectedCommands(commands []command) []command {
	var nonRejectedCommands []command
	for _, c := range commands {
		if c.err == "" {
			nonRejectedCommands = append(nonRejectedCommands, c)
		}
	}
	return nonRejectedCommands
}

func (r *spokesReceivePack) performCheckConnectivityOnObject(ctx context.Context, oid string) error {
	cmd := exec.CommandContext(
		ctx,
		"git",
		"rev-list",
		"--objects",
		"--no-object-names",
		oid,
		"--not",
		"--all",
		"--alternate-refs",
	)
	cmd.Env = append([]string{}, os.Environ()...)
	cmd.Env = append(cmd.Env, r.getAlternateObjectDirsEnv()...)

	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("performCheckConnectivityOnObject on oid %s: %s. Details: %s", oid, err, string(out))
	}

	return nil
}

// report the success/failure of the push operation to the client
func writeReport(w io.Writer, unpackOK bool, commands []command) error {
	if unpackOK {
		if err := writePacketLine(w, []byte("unpack ok\n")); err != nil {
			return err
		}
	} else {
		if err := writePacketLine(w, []byte("unpack index-pack failed\n")); err != nil {
			return err
		}
	}
	for _, c := range commands {
		if c.err != "" {
			if err := writePacketf(w, "ng %s %s\n", c.refname, c.err); err != nil {
				return err
			}
		} else {
			if err := writePacketf(w, "%s %s\n", c.reportFF, c.refname); err != nil {
				return err
			}
			// FIXME? if statusV2, maybe also write option refname, option old-oid, option new-oid, option forced-update
		}
	}

	if _, err := fmt.Fprint(w, "0000"); err != nil {
		return err
	}

	return nil
}

func (r *spokesReceivePack) report(_ context.Context, unpackOK bool, commands []command, capabilities pktline.Capabilities) error {
	if !useSideBand(capabilities) {
		return writeReport(r.output, unpackOK, commands)
	}

	var buf bytes.Buffer

	if err := writeReport(&buf, unpackOK, commands); err != nil {
		return err
	}

	output := buf.Bytes()

	packetMax := sideBandBufSize(capabilities)

	for len(output) > 0 {
		n := packetMax - 5
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

func useSideBand(c pktline.Capabilities) bool {
	return c.IsDefined(pktline.SideBand) || c.IsDefined(pktline.SideBand64k)
}

func sideBandBufSize(capabilities pktline.Capabilities) int {
	if capabilities.IsDefined(pktline.SideBand64k) {
		return 65519
	}
	return 999
}
