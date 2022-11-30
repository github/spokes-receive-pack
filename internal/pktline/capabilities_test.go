package pktline

import (
	"fmt"
	"github.com/stretchr/testify/assert"
	"testing"
)

const (
	capabilities = "agent=spokes-pack-tests delete-refs multi_ack thin-pack no-done atomic filter=x push-cert=foo side-band side-band-64k ofs-delta shallow allow-tip-sha1-in-want allow-reachable-sha1-in-want deepen-since deepen-not deepen-relative no-progress include-tag multi_ack_detailed"
)

func TestParseSimpleCapabilities(t *testing.T) {
	bytes := []byte(capabilities)
	for _, p := range []struct {
		capabilities []byte
		capability   string
	}{
		{bytes, MultiAck},
		{bytes, MultiAckDetailed},
		{bytes, NoDone},
		{bytes, ThinPack},
		{bytes, SideBand},
		{bytes, SideBand64k},
		{bytes, OfsDelta},
		{bytes, Shallow},
		{bytes, DeepenSince},
		{bytes, DeepenNot},
		{bytes, DeepenRelative},
		{bytes, NoProgress},
		{bytes, IncludeTag},
		{bytes, Atomic},
		{bytes, AllowTipSha1InWant},
		{bytes, AllowReachableSha1InWant},
		{bytes, PushCert},
		{bytes, Filter},
		{bytes, DeleteRefs},
		{bytes, Agent},
	} {
		t.Run(
			fmt.Sprintf("TestParseCapabilities(%s)", p.capabilities),
			func(t *testing.T) {
				capabilities, err := ParseCapabilities(p.capabilities)
				assert.NoError(t, err)
				cap, found := capabilities.caps[p.capability]
				assert.True(t, found)
				assert.Equal(t, cap.Name(), p.capability)
			},
		)
	}
}

func TestParseCapabilitiesWithArguments(t *testing.T) {
	bytes := []byte(capabilities)
	for _, p := range []struct {
		capabilities []byte
		capability   string
		value        string
	}{
		{bytes, Agent, "spokes-pack-tests"},
		{bytes, Filter, "x"},
		{bytes, PushCert, "foo"},
	} {
		t.Run(
			fmt.Sprintf("TestParseCapabilitiesWithArguments(%s)", p.capabilities),
			func(t *testing.T) {
				capabilities, err := ParseCapabilities(p.capabilities)
				assert.NoError(t, err)
				cap, found := capabilities.caps[p.capability]
				assert.True(t, found)
				assert.Equal(t, cap.Value(), p.value)
			},
		)
	}
}
