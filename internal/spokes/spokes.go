package spokes

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"github.com/github/spokes-receive-pack/internal/pipe"
	"io"
	"os"
)

const (
	capabilities = "report-status delete-refs side-band-64k ofs-delta"
	// maximum length of a pkt-line's data component
	maxPacketDataLength = 65516
	nullOID             = "0000000000000000000000000000000000000000"
)

// SpokesReceivePack is used to model our own impl of the git-receive-pack
type SpokesReceivePack struct {
	input  io.Reader
	output io.Writer
	err    io.Writer
	args   []string
}

// NewSpokesReceivePack returns a pointer to a SpokesReceivePack executor
func NewSpokesReceivePack(input io.Reader, output, err io.Writer, args []string) *SpokesReceivePack {
	return &SpokesReceivePack{
		input:  input,
		output: output,
		err:    err,
		args:   args,
	}
}

// Execute executes our custom implementation
// It tries to model the behaviour described in the "Pushing Data To a Server" section of the
// https://github.com/github/git/blob/github/Documentation/technical/pack-protocol.txt document
func (r *SpokesReceivePack) Execute(ctx context.Context) error {
	if err := os.Chdir(r.args[0]); err != nil {
		return fmt.Errorf("unable to enter repo at location: %s", r.args[0])
	}

	// Reference discovery phase
	if err := r.performReferenceDiscovery(ctx); err != nil {
		return err
	}

	panic("Not complete yet!")
}

// performReferenceDiscovery performs the reference discovery bits of the protocol
// It writes back to the client the capability listing and a packet-line for every reference
// terminated with a flush-pkt
func (r *SpokesReceivePack) performReferenceDiscovery(ctx context.Context) error {
	capsOutput := false
	p := pipe.New(".", pipe.WithStdout(r.output))
	p.Add(
		pipe.GitCommand("for-each-ref", "--format=%(objectname) %(refname)"),
		pipe.LinewiseFunction(
			"print-advertisement",
			func(ctx context.Context, _ pipe.Env, line []byte, stdout *bufio.Writer) error {
				if !capsOutput {
					if err := r.writePacketf("%s\x00%s\n", line, capabilities); err != nil {
						return fmt.Errorf("writing capability packet: %w", err)
					}
					capsOutput = true
				} else {
					if err := r.writePacketf("%s\n", line); err != nil {
						return fmt.Errorf("writing ref advertisement packet: %w", err)
					}
				}
				return nil
			},
		),
	)

	if err := p.Run(ctx); err != nil {
		return fmt.Errorf("writing advertisements: %w", err)
	}

	if !capsOutput {
		if _, err := fmt.Fprintf(r.output, "%s capabilities^{}\x00%s", nullOID, capabilities); err != nil {
			return fmt.Errorf("writing lonely capability packet: %w", err)
		}
	}

	fmt.Fprintf(r.output, "0000")

	return nil
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
