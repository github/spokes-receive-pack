package governor

import (
	"bufio"
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	connectTimeout  = time.Second
	scheduleTimeout = time.Second
)

// Start connects to governor and sends the "update" and "schedule" messages.
//
// If "schedule" says to wait, Start will pause for the specified time and try
// calling "schedule" again.
//
// If there is a connection or other low level error when talking to governor,
// Start will return (nil, nil).
func Start(ctx context.Context) (*Conn, error) {
	sock, err := connect(ctx)
	if err != nil {
		return nil, nil
	}

	updateData := readSockstat(os.Environ())
	updateData.PID = os.Getpid()
	updateData.Program = "spokes-receive-pack"
	updateData.GitDir, _ = os.Getwd()
	if err := update(sock, updateData); err != nil {
		sock.Close()
		return nil, nil
	}

	br := bufio.NewReader(sock)
	for {
		// Give governor 1s to respond each schedule call.
		sock.SetReadDeadline(time.Now().Add(scheduleTimeout))
		err := schedule(br, sock)
		if err == nil {
			break
		}

		switch e := err.(type) {
		case WaitError:
			time.Sleep(e.Duration)
		case FailError:
			sock.Close()
			return nil, err
		default:
			sock.Close()
			return nil, nil
		}
	}

	return &Conn{sock: sock}, nil
}

// Conn is an active connection to governor.
type Conn struct {
	sock   net.Conn
	finish finishData
}

// SetError stores an error to include with the finish message.
//
// It is safe to call SetError with a nil *Conn.
func (c *Conn) SetError(exitCode uint8, message string) {
	if c == nil {
		return
	}
	c.finish.ResultCode = exitCode
	c.finish.Fatal = message
}

// SetReceivePackSize records the incoming packfile's size to include with the
// finish message.
//
// It is safe to call SetReceivePackSize with a nil *Conn.
func (c *Conn) SetReceivePackSize(size int64) {
	if c == nil {
		return
	}
	if size > 0 {
		c.finish.ReceivePackSize = uint64(size)
	}
}

// Finish sends the "finish" message to governor and closes the connection.
//
// It is safe to call Finish with a nil *Conn.
func (c *Conn) Finish(ctx context.Context) {
	if c == nil || c.sock == nil {
		return
	}

	stats := getProcStats()
	c.finish.CPU = stats.CPU
	c.finish.RSS = stats.RSS
	c.finish.DiskReadBytes = stats.DiskReadBytes
	c.finish.DiskWriteBytes = stats.DiskWriteBytes

	_ = finish(c.sock, c.finish)

	c.sock.Close()
	c.sock = nil
}

type procStats struct {
	CPU            uint32
	RSS            uint64
	DiskReadBytes  uint64
	DiskWriteBytes uint64
}

func connect(ctx context.Context) (net.Conn, error) {
	ctx, cancel := context.WithTimeout(ctx, connectTimeout)
	defer cancel()

	path := os.Getenv("GIT_SOCKSTAT_PATH")
	if path == "" {
		path = "/var/run/gitmon/gitstats.sock"
	}

	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, "unix", path)
}

func readSockstat(environ []string) updateData {
	var res updateData

	const prefix = "GIT_SOCKSTAT_VAR_"
	for _, env := range environ {
		if !strings.HasPrefix(env, prefix) {
			continue
		}
		env = env[len(prefix):]
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "repo_name":
			res.RepoName = sockstatString(parts[1])
		case "repo_id":
			res.RepoID = sockstatUint32(parts[1])
		case "network_id":
			res.NetworkID = sockstatUint32(parts[1])
		case "user_id":
			res.UserID = sockstatUint32(parts[1])
		case "real_ip":
			res.RealIP = sockstatString(parts[1])
		case "request_id":
			res.RequestID = sockstatString(parts[1])
		case "user_agent":
			res.UserAgent = sockstatString(parts[1])
		case "features":
			res.Features = sockstatString(parts[1])
		case "via":
			res.Via = sockstatString(parts[1])
		case "ssh_connection":
			res.SSHConnection = sockstatString(parts[1])
		case "babeld":
			res.Babeld = sockstatString(parts[1])
		case "git_protocol":
			res.GitProtocol = sockstatString(parts[1])
		case "pubkey_verifier_id":
			res.PubkeyVerifierID = sockstatUint32(parts[1])
		case "pubkey_creator_id":
			res.PubkeyCreatorID = sockstatUint32(parts[1])
		}
	}

	return res
}

// sockstatUint32 parses a string like "uint32:123" and returns the parsed
// uint32 like 123. If the prefix is missing or the value isn't a uint32,
// return 0.
func sockstatUint32(s string) uint32 {
	s, ok := cutPrefix(s, "uint:")
	if !ok {
		return 0
	}
	val, err := strconv.ParseUint(s, 10, 32)
	if err != nil {
		return 0
	}
	return uint32(val)
}

// sockstatString returns the string version of the given sockstat var. For the
// most part, this means just returning the given string. However, if the input
// has a uint or bool prefix, strip that off so that it looks like we parsed
// the value and then stringified it.
func sockstatString(s string) string {
	parts := strings.SplitN(s, ":", 2)
	if len(parts) == 2 && (parts[0] == "uint" || parts[0] == "bool") {
		return parts[1]
	}
	return s
}

// TODO: replace with Go 1.20's strings.CutPrefix
func cutPrefix(s, prefix string) (string, bool) {
	if !strings.HasPrefix(s, prefix) {
		return s, false
	}
	return s[len(prefix):], true
}
