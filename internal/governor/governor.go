package governor

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"strconv"
	"strings"
	"time"
)

type updateData struct {
	// The process ID of the process being run. On Linux, this will
	// usually be ignored by governor in favor of the PID of the process
	// connecting to the socket.
	PID int `json:"pid,omitempty"`

	// An ID that identifies a group of commands that all make up one
	// logical request.
	GroupID string `json:"group_id,omitempty"`

	// True if this is the top-level request in a group.
	GroupLeader bool `json:"group_leader,omitempty"`

	// The quality of service for this request. If not set, governor will choose the quality of service to use (probably delayable).
	QualityOfService string `json:"qos,omitempty"`

	// The name of the program being run.
	Program string `json:"program,omitempty"`

	// The git directory path.
	GitDir string `json:"git_dir,omitempty"`

	// The repository's NWO, if available.
	RepoName string `json:"repo_name,omitempty"`

	// The repository's numerical ID.
	RepoID uint32 `json:"repo_id,omitempty"`

	// The repository's network ID.
	NetworkID uint32 `json:"network_id,omitempty"`

	// The ID of the GitHub user on whose behalf this process is being
	// run.
	UserID uint32 `json:"user_id,omitempty"`

	// The IP number of the user making the request.
	RealIP string `json:"real_ip,omitempty"`

	// The request ID of the request that triggered this process.
	RequestID string `json:"request_id,omitempty"`

	// The User-Agent from the request. For Spokes API requests, this is
	// the internal client's User-Agent with a spokesd version appended to
	// it.
	UserAgent string `json:"user_agent,omitempty"`

	// The X-Spokesd-TLS-Client header from the request. On dotcom, this is
	// taken from the CN of the client's certificate. In other
	// environments, this will not be set.
	ClientApp string `json:"client_app,omitempty"`

	Features         string `json:"features,omitempty"`
	Via              string `json:"via,omitempty"`
	SSHConnection    string `json:"ssh_connection,omitempty"`
	Babeld           string `json:"babeld,omitempty"`
	GitProtocol      string `json:"git_protocol,omitempty"`
	PubkeyVerifierID uint32 `json:"pubkey_verifier_id,omitempty"`
	PubkeyCreatorID  uint32 `json:"pubkey_creator_id,omitempty"`
	GitmonDelay      uint32 `json:"gitmon_delay,omitempty"`
}

func update(w io.Writer, ud updateData) error {
	updateMsg := struct {
		Command string     `json:"command"`
		Data    updateData `json:"data"`
	}{
		Command: "update",
		Data:    ud,
	}

	msg, err := json.Marshal(updateMsg)
	if err != nil {
		return err
	}

	_, err = w.Write(msg)
	return err
}

type WaitError struct {
	Duration time.Duration
	Reason   string
}

func newWaitError(duration time.Duration, reason string) error {
	return WaitError{
		Duration: duration,
		Reason:   reason,
	}
}

func (err WaitError) Error() string {
	return fmt.Sprintf("governor asked us to wait %s: %s", err.Duration, err.Reason)
}

type FailError struct {
	Reason string
}

func newFailError(reason string) error {
	return FailError{
		Reason: reason,
	}
}

func (err FailError) Error() string {
	return fmt.Sprintf("governor refuses to schedule us: %s", err.Reason)
}

func schedule(r *bufio.Reader, w io.Writer, sideband io.Writer) error {
	const msg = `{"command":"schedule"}`

	_, err := w.Write([]byte(msg))
	if err != nil {
		return err
	}

	b, err := r.ReadBytes('\n')
	if err != nil {
		return err
	}

	line := string(b[:len(b)-1])

	//Forward message to gitrpcd
	if sideband != nil {
		sideband.Write(b)
	}

	words := strings.SplitN(line, " ", 3)
	switch words[0] {
	case "continue":
		return nil
	case "wait":
		duration := 1 * time.Second
		reason := "UNKNOWN"
		if len(words) > 1 {
			d, err := strconv.Atoi(words[1])
			if err != nil {
				log.Printf("warning: 'wait' duration %q could not be parsed", words[1])
			} else {
				duration = time.Duration(d) * time.Second
			}
		}
		if len(words) > 2 {
			reason = strings.Join(words[2:], " ")
		}
		return newWaitError(duration, reason)
	case "fail":
		reason := "UNKNOWN"
		if len(words) > 1 {
			reason = strings.Join(words[1:], " ")
		}
		return newFailError(reason)
	default:
		return fmt.Errorf("unexpected response %q from governor", line)
	}
}

type finishData struct {
	// The command's result code.
	ResultCode uint8 `json:"result_code"`

	// The amount of user plus system CPU used by the command, as an
	// integer number of milliseconds.
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	CPU uint32 `json:"cpu,omitempty"`

	// The number of times that the filesystem had to perform input.
	// (Actually, git sends `ru_inblock`, which is the number of times
	// that the filesystem had to perform input).
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	DiskReadBytes uint64 `json:"disk_read_bytes,omitempty"`

	// The number of bytes written to the filesystem. (Actually, git
	// sends `ru_outblock`, which is the number of times that the
	// filesystem had to perform output).
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	DiskWriteBytes uint64 `json:"disk_write_bytes,omitempty"`

	// The maximum resident set size.
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	RSS uint64 `json:"rss,omitempty"`

	// The size of the uploaded packfile, in bytes (implemented only
	// for `upload-pack`).
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	UploadedBytes uint64 `json:"uploaded_bytes,omitempty"`

	// The size of the received packfile, in bytes (implemented only
	// for `receive-pack`).
	//
	// If this is the GroupLeader, this is an aggregate value for the whole
	// group.
	ReceivePackSize uint64 `json:"receive_pack_size,omitempty"`

	// Bitwise OR of:
	//
	// * 0x01 — Was this invocation of `upload-pack` a clone (as
	//   opposed to a fetch)?
	//
	// * 0x02 — Was it a shallow (as opposed to a full)
	//   clone/fetch?
	Cloning uint8 `json:"cloning,omitempty"`

	// If git died, what was the error message that it emitted?
	Fatal string `json:"fatal,omitempty"`
}

func finish(w io.Writer, fd finishData) error {
	finishMsg := struct {
		Command string     `json:"command"`
		Data    finishData `json:"data"`
	}{
		Command: "finish",
		Data:    fd,
	}

	msg, err := json.Marshal(finishMsg)
	if err != nil {
		return err
	}

	_, err = w.Write(msg)
	return err
}
