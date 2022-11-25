package pktline_test

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"

	"github.com/github/spokes-receive-pack/internal/pktline"
	"github.com/stretchr/testify/assert"
)

type expectedPktline struct {
	size    int
	payload string
}

var expectFlush = expectedPktline{
	size:    0,
	payload: "",
}

func (expected *expectedPktline) CheckEqual(pl *pktline.Pktline) error {
	size, err := pl.Size()
	if err != nil {
		return fmt.Errorf("invalid pktline size: %w", err)
	}
	if size != expected.size {
		return fmt.Errorf("incorrect pktline size: expected %d, got %d", expected.size, size)
	}

	payload := string(pl.Payload)
	if payload != expected.payload {
		return fmt.Errorf(
			"incorrect pktline payload: expected %q, got %q",
			expected.payload, payload,
		)
	}
	return nil
}

func TestRead(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected []expectedPktline
	}{
		{
			name:     "nothing",
			input:    "",
			expected: nil,
		},
		{
			name:  "flush",
			input: "0000",
			expected: []expectedPktline{
				expectFlush,
			},
		},
		{
			name:  "short",
			input: "0002",
			expected: []expectedPktline{
				{
					size:    2,
					payload: "",
				},
			},
		},
		{
			name:  "keepalive",
			input: "0004",
			expected: []expectedPktline{
				{
					size:    4,
					payload: "",
				},
			},
		},
		{
			name:  "receive-pack-packet-line",
			input: "006874730d410fcb6603ace96f1dc55ea6196122532d 5a3f6be755bbb7deae50065988cbfa1ffa9ab68a refs/heads/master\n",
			expected: []expectedPktline{
				{
					size:    104,
					payload: "74730d410fcb6603ace96f1dc55ea6196122532d 5a3f6be755bbb7deae50065988cbfa1ffa9ab68a refs/heads/master\n",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := pktline.New()
			r := strings.NewReader(tc.input)
			for i, expected := range tc.expected {
				assert.NoError(t, pl.Read(r), "reading pktline")
				assert.NoErrorf(t, expected.CheckEqual(pl), "pktline %d incorrect", i)
			}

			err := pl.Read(r)
			assert.True(t, errors.Is(err, io.EOF), "expected io.EOF after reading all pktlines")
		})
	}
}

func TestReadWithCapabilities(t *testing.T) {
	for _, tc := range []struct {
		name     string
		input    string
		expected []expectedPktline
	}{
		{
			name:  "single-line",
			input: "00820000000000000000000000000000000000000000 f9cc25952a0d66c0a388ee0decfda12a0122404d refs/heads/main\000report-status side-band-64k\n",
			expected: []expectedPktline{
				{
					size:    130,
					payload: "0000000000000000000000000000000000000000 f9cc25952a0d66c0a388ee0decfda12a0122404d refs/heads/main",
				},
			},
		},
		{
			name: "two-lines",
			input: "00860000000000000000000000000000000000000000 791d15c40c6f465afebc1ba6a11761c0b43e1c35 refs/heads/branch-1\000report-status side-band-64k\n" +
				"00650000000000000000000000000000000000000000 f01567bcabe2741094ab2b67155ce26a9527746f refs/heads/main",
			expected: []expectedPktline{
				{
					size:    134,
					payload: "0000000000000000000000000000000000000000 791d15c40c6f465afebc1ba6a11761c0b43e1c35 refs/heads/branch-1",
				},
				{
					size:    101,
					payload: "0000000000000000000000000000000000000000 f01567bcabe2741094ab2b67155ce26a9527746f refs/heads/main",
				},
			},
		},
		{
			name: "four-lines",
			input: "00860000000000000000000000000000000000000000 bf7bf7a2eeee69e967d59aaab78f9022a1447b12 refs/heads/branch-1\000report-status side-band-64k\n" +
				"00690000000000000000000000000000000000000000 f13ce6e8f50b0aa7aae764434ee15a414da3f50f refs/heads/branch-2" +
				"00690000000000000000000000000000000000000000 6d0be418a4c1776981726d1a8d39cd7f790efb61 refs/heads/branch-3" +
				"00650000000000000000000000000000000000000000 5bf437d78e72522939e6b17aeed1a5b0ae73a100 refs/heads/main",
			expected: []expectedPktline{
				{
					size:    134,
					payload: "0000000000000000000000000000000000000000 bf7bf7a2eeee69e967d59aaab78f9022a1447b12 refs/heads/branch-1",
				},
				{
					size:    105,
					payload: "0000000000000000000000000000000000000000 f13ce6e8f50b0aa7aae764434ee15a414da3f50f refs/heads/branch-2",
				},
				{
					size:    105,
					payload: "0000000000000000000000000000000000000000 6d0be418a4c1776981726d1a8d39cd7f790efb61 refs/heads/branch-3",
				},
				{
					size:    101,
					payload: "0000000000000000000000000000000000000000 5bf437d78e72522939e6b17aeed1a5b0ae73a100 refs/heads/main",
				},
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := pktline.New()
			r := strings.NewReader(tc.input)
			for i, expected := range tc.expected {
				assert.NoError(t, pl.Read(r), "reading pktline")
				assert.NoErrorf(t, expected.CheckEqual(pl), "pktline %d incorrect", i)
			}

			err := pl.Read(r)
			assert.True(t, errors.Is(err, io.EOF), "expected io.EOF after reading all pktlines")

			caps, err := pl.Capabilities()
			assert.NoError(t, err)

			assert.Equal(t, "report-status", caps.ReportStatus().Name())
			assert.Equal(t, "side-band-64k", caps.SideBand64k().Name())
		})
	}
}

func TestReadErrors(t *testing.T) {
	for _, tc := range []struct {
		name          string
		input         string
		expectedError string
	}{
		{
			name:          "truncated-size",
			input:         "01",
			expectedError: "unexpected EOF",
		},
		{
			name:          "invalid-size",
			input:         "foob",
			expectedError: "illformed pktline size",
		},
		{
			name:          "truncated-payload",
			input:         "fff4" + "2" + "not enough bytes",
			expectedError: "unexpected EOF",
		},
		{
			name:          "size-too-large",
			input:         "fff5" + "2" + "these bytes not read",
			expectedError: "read-header: invalid pkt-line length",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pl := pktline.New()
			r := strings.NewReader(tc.input)
			err := pl.Read(r)
			if !strings.Contains(err.Error(), tc.expectedError) {
				t.Fatal(
					"expected error '"+tc.expectedError+
						"' after reading all pktlines; got ", err,
				)
			}
		})
	}
}
