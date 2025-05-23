package governor

import (
	"bufio"
	"context"
	"net"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/github/spokes-receive-pack/internal/sockstat"
)

const (
	connectTimeout = time.Second
)

func scheduleTimeout() time.Duration {
	timeout := time.Second
	if v := os.Getenv("SCHEDULE_CMD_TIMEOUT"); v != "" {
		if d, err := strconv.ParseInt(v, 10, 64); err == nil {
			timeout = time.Duration(d) * time.Millisecond
		}
	}

	return timeout
}

func shouldFailClosed() bool {
	return os.Getenv("FAIL_CLOSED") == "1"
}

// Start connects to governor and sends the "update" and "schedule" messages.
//
// If "schedule" says to wait, Start will pause for the specified time and try
// calling "schedule" again.
//
// If there is a connection or other low level error when talking to governor,
// Start will return (nil, nil).
func Start(ctx context.Context, gitDir string) (*Conn, error) {
	sock, err := connect(ctx)
	if err != nil {
		return nil, nil
	}

	updateData := readSockstat(os.Environ())
	updateData.PID = os.Getpid()
	updateData.Program = "spokes-receive-pack"
	updateData.GitDir = gitDir
	if err := update(sock, updateData); err != nil {
		sock.Close()
		return nil, nil
	}

	timeout := scheduleTimeout()
	failClosed := shouldFailClosed()
	br := bufio.NewReader(sock)
	for {
		// Give governor a limited time to respond.
		if err := sock.SetReadDeadline(time.Now().Add(timeout)); err != nil {
			break
		}

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
		case net.Error:
			sock.Close()

			if e.Timeout() && failClosed {
				return nil, err
			}

			return nil, nil
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
		path = "/run/governor/client.sock"
	}

	dialer := &net.Dialer{}
	return dialer.DialContext(ctx, "unix", path)
}

func readSockstat(environ []string) updateData {
	var res updateData

	for _, env := range environ {
		if !strings.HasPrefix(env, sockstat.Prefix) {
			continue
		}
		env = env[len(sockstat.Prefix):]
		parts := strings.SplitN(env, "=", 2)
		if len(parts) != 2 {
			continue
		}
		switch parts[0] {
		case "repo_name":
			res.RepoName = sockstat.StringValue(parts[1])
		case "repo_id":
			res.RepoID = sockstat.Uint32Value(parts[1])
		case "network_id":
			res.NetworkID = sockstat.Uint32Value(parts[1])
		case "user_id":
			res.UserID = sockstat.Uint32Value(parts[1])
		case "real_ip":
			res.RealIP = sockstat.StringValue(parts[1])
		case "request_id":
			res.RequestID = sockstat.StringValue(parts[1])
		case "user_agent":
			res.UserAgent = sockstat.StringValue(parts[1])
		case "features":
			res.Features = sockstat.StringValue(parts[1])
		case "via":
			res.Via = sockstat.StringValue(parts[1])
		case "ssh_connection":
			res.SSHConnection = sockstat.StringValue(parts[1])
		case "babeld":
			res.Babeld = sockstat.StringValue(parts[1])
		case "git_protocol":
			res.GitProtocol = sockstat.StringValue(parts[1])
		case "pubkey_verifier_id":
			res.PubkeyVerifierID = sockstat.Uint32Value(parts[1])
		case "pubkey_creator_id":
			res.PubkeyCreatorID = sockstat.Uint32Value(parts[1])
		case "max_delay":
			res.MaxDelay = sockstat.Uint32Value(parts[1])
		case "command_id":
			res.CommandID = sockstat.StringValue(parts[1])
		case "group_id":
			res.GroupID = sockstat.StringValue(parts[1])
		case "group_leader":
			res.GroupLeader = sockstat.GetBool(parts[1])
		case "is_importing":
			res.IsImporting = sockstat.BoolValue(parts[1])
		case "import_skip_push_limit":
			res.ImportSkipPushLimit = sockstat.BoolValue(parts[1])
		case "import_soft_throttling":
			res.ImportSoftThrottling = sockstat.BoolValue(parts[1])
		}
	}

	return res
}
