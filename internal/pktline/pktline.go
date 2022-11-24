package pktline

import (
	"bytes"
	"fmt"
	"io"
	"strconv"
)

const (
	MaxPayload = 65519
	HeaderSize = 4
)

var FlushPktline = []byte("0000")
var HeartbeatPktline = []byte("0004")

type Pktline struct {
	buf                   [HeaderSize + MaxPayload + 1]byte
	payloadSize           []byte
	Payload               []byte
	CapabilitiesPayload   []byte
	processedCapabilities bool
}

func New() *Pktline {
	pl := Pktline{}
	pl.Reset()
	return &pl
}

func (pl *Pktline) IsFlush() bool {
	return bytes.Equal(pl.payloadSize, []byte("0000"))
}

func (pl *Pktline) IsHeartbeat() bool {
	return bytes.Equal(pl.payloadSize, []byte("0004"))
}

func (pl *Pktline) Capabilities() (Capabilities, error) {
	return ParseCapabilities(pl.CapabilitiesPayload)
}

// Size returns the total size of `pl` (including the length) by
// parsing `pl.payloadSize`.
func (pl *Pktline) Size() (int, error) {
	size, err := strconv.ParseUint(string(pl.payloadSize), 16, 16)
	if err != nil {
		return 0, fmt.Errorf("read-header: illformed pktline size: %w", err)
	}

	if size > HeaderSize+MaxPayload+1 {
		return 0, fmt.Errorf("read-header: invalid pkt-line length: %d", size)
	}
	return int(size), nil
}

// Reset resets the pktline to read the next pktline
func (pl *Pktline) Reset() {
	pl.payloadSize = pl.buf[:4]
	pl.Payload = pl.buf[4:]
}

// Read reads the next pktline from `r` into `pl` (resetting `pl`
// first). If `r` is already at EOF, return `io.EOF`. If EOF is
// encountered after reading part but not all the pktline, return
// `io.ErrUnexpectedEOF`.
func (pl *Pktline) Read(r io.Reader) error {
	pl.Reset()
	// Read header
	if _, err := io.ReadFull(r, pl.payloadSize); err != nil {
		if err == io.EOF {
			return io.EOF
		}
		return fmt.Errorf("reading pktline size: %w", err)
	}

	size, err := pl.Size()
	if err != nil {
		return err
	}

	if size <= HeaderSize {
		// No payload
		pl.Payload = pl.buf[4:4]
		return nil
	}

	// Read payload
	if _, err := io.ReadFull(r, pl.buf[4:size]); err != nil {
		if err == io.EOF {
			err = io.ErrUnexpectedEOF
		}
		return fmt.Errorf("reading pktline data: %w", err)
	}

	pl.Payload = pl.buf[4:size]

	// Capabilities are (optionally) sent along the first packet line
	if !pl.processedCapabilities {
		if index := bytes.IndexByte(pl.Payload, 0); index != -1 {
			pl.CapabilitiesPayload = pl.Payload[index+1:]
			pl.processedCapabilities = true
			pl.Payload = pl.Payload[:index]
		}
	}

	return nil
}
