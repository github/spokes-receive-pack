package pktline

import (
	"fmt"
	"sort"
	"strings"
)

const (
	MultiAck                 = "multi_ack"
	MultiAckDetailed         = "multi_ack_detailed"
	NoDone                   = "no-done"
	ThinPack                 = "thin-pack"
	SideBand                 = "side-band"
	SideBand64k              = "side-band-64k"
	OfsDelta                 = "ofs-delta"
	Agent                    = "agent"
	ObjectFormat             = "object-format"
	Symref                   = "symref"
	Shallow                  = "shallow"
	DeepenSince              = "deepen-since"
	DeepenNot                = "deepen-not"
	DeepenRelative           = "deepen-relative"
	NoProgress               = "no-progress"
	IncludeTag               = "include-tag"
	ReportStatus             = "report-status"
	ReportStatusV2           = "report-status-v2"
	DeleteRefs               = "delete-refs"
	Quiet                    = "quiet"
	Atomic                   = "atomic"
	PushOptions              = "push-options"
	AllowTipSha1InWant       = "allow-tip-sha1-in-want"
	AllowReachableSha1InWant = "allow-reachable-sha1-in-want"
	PushCert                 = "push-cert"
	Filter                   = "filter"
	SessionId                = "session-id"
)

type Capability struct {
	name  string
	value string
}

func newCapability(data string) (Capability, error) {
	rawCap := strings.Split(data, "=")
	cap := Capability{}
	switch len(rawCap) {
	case 1:
		cap.name = rawCap[0]
	case 2:
		cap.name = rawCap[0]
		cap.value = rawCap[1]
	default:
		return Capability{}, fmt.Errorf("unexpected Capability format %s", data)
	}

	return cap, nil
}

func (c Capability) Name() string {
	return c.name
}

func (c Capability) Value() string {
	return c.value
}

// Capabilities models the capabilities that can be sent across the client and the server in the pack protocol V1
// The abstraction parses all the capabilities defined in the spec but our goal is to focus in those relevant
// for the upload process part
type Capabilities struct {
	caps map[string]Capability
}

// ParseCapabilities converts the passed capabilities (as received in the protocol) into its corresponding typed object
func ParseCapabilities(capabilities []byte) (Capabilities, error) {
	caps := string(capabilities)
	caps = strings.TrimSuffix(caps, "\n")
	splitted := strings.Split(caps, " ")

	parsedCaps := make(map[string]Capability, len(caps))
	for _, c := range splitted {
		cap, err := newCapability(c)
		if err != nil {
			return Capabilities{}, fmt.Errorf("unable to parse Capability %s", c)
		}
		parsedCaps[cap.Name()] = cap
	}

	return Capabilities{caps: parsedCaps}, nil
}

func (c Capabilities) MultiAck() Capability {
	return c.caps[MultiAck]
}
func (c Capabilities) MultiAckDetailed() Capability {
	return c.caps[MultiAckDetailed]
}
func (c Capabilities) NoDone() Capability {
	return c.caps[NoDone]
}
func (c Capabilities) ThinPack() Capability {
	return c.caps[ThinPack]
}
func (c Capabilities) SideBand() Capability {
	return c.caps[SideBand]
}
func (c Capabilities) SideBand64k() Capability {
	return c.caps[SideBand64k]
}
func (c Capabilities) OfsDelta() Capability {
	return c.caps[OfsDelta]
}
func (c Capabilities) Agent() Capability {
	return c.caps[Agent]
}
func (c Capabilities) ObjectFormat() Capability {
	return c.caps[ObjectFormat]
}
func (c Capabilities) Symref() Capability {
	return c.caps[Symref]
}
func (c Capabilities) Shallow() Capability {
	return c.caps[Shallow]
}
func (c Capabilities) DeepenSince() Capability {
	return c.caps[DeepenSince]
}
func (c Capabilities) DeepenNot() Capability {
	return c.caps[DeepenNot]
}
func (c Capabilities) DeepenRelative() Capability {
	return c.caps[DeepenRelative]
}
func (c Capabilities) NoProgress() Capability {
	return c.caps[NoProgress]
}
func (c Capabilities) IncludeTag() Capability {
	return c.caps[IncludeTag]
}
func (c Capabilities) ReportStatus() Capability {
	return c.caps[ReportStatus]
}
func (c Capabilities) ReportStatusV2() Capability {
	return c.caps[ReportStatusV2]
}
func (c Capabilities) DeleteRefs() Capability {
	return c.caps[DeleteRefs]
}
func (c Capabilities) Quiet() Capability {
	return c.caps[Quiet]
}
func (c Capabilities) Atomic() Capability {
	return c.caps[Atomic]
}
func (c Capabilities) PushOptions() Capability {
	return c.caps[PushOptions]
}
func (c Capabilities) AllowTipSha1InWant() Capability {
	return c.caps[AllowTipSha1InWant]
}
func (c Capabilities) AllowReachableSha1InWant() Capability {
	return c.caps[AllowReachableSha1InWant]
}
func (c Capabilities) PushCert() Capability {
	return c.caps[PushCert]
}
func (c Capabilities) Filter() Capability {
	return c.caps[Filter]
}
func (c Capabilities) SessionId() Capability {
	return c.caps[SessionId]
}

func (c Capabilities) Names() []string {
	res := make([]string, 0, len(c.caps))
	for k := range c.caps {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

func (c Capabilities) Get(cap string) (Capability, bool) {
	capability, found := c.caps[cap]
	return capability, found
}

func (c Capabilities) IsDefined(cap string) bool {
	_, found := c.caps[cap]
	return found
}

func IsSafeCapabilityValue(val string) bool {
	// Git needs this not to include \r, \n, \t, or ' '.
	// https://github.com/git/git/blob/d7d8841f67f29e6ecbad85a11805c907d0f00d5d/connect.c#L629
	for _, b := range []byte(val) {
		switch b {
		case ' ', '\r', '\n', '\t':
			return false
		}
	}
	return true
}
