//go:build linux

package governor

import (
	"bytes"
	"os"
	"strconv"
	"strings"
	"syscall"
)

func getProcStats() procStats {
	var res procStats

	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err == nil {
		res.CPU = uint32(ru.Utime.Sec*1000) + uint32(ru.Utime.Usec/1000) + uint32(ru.Stime.Sec*1000) + uint32(ru.Stime.Usec/1000)
		res.RSS = uint64(ru.Maxrss)
	}

	if res.RSS == 0 {
		pageSize := syscall.Getpagesize()
		if pageSize > 0 {
			stat, err := os.ReadFile("/proc/self/stat")
			if err == nil {
				fields := bytes.Fields(stat)
				if len(fields) > 23 {
					res.RSS, _ = strconv.ParseUint(fields[23], 10, 64)
				}
			}
		}
	}

	iostats, err := os.ReadFile("/proc/self/io")
	if err == nil {
		const (
			readPrefix           = "read_bytes: "
			writePrefix          = "write_bytes: "
			cancelledWritePrefix = "cancelled_write_bytes: "
		)
		for _, line := range strings.Split(string(iostats), "\n") {
			switch {
			case strings.HasPrefix(line, readPrefix):
				if val, err := strconv.ParseUint(line[len(readPrefix):], 10, 64); err != nil {
					res.ReadBytes = val
				}
			case strings.HasPrefix(line, writePrefix):
				if val, err := strconv.ParseUint(line[len(writePrefix):], 10, 64); err != nil {
					res.WriteBytes = val
				}
			case strings.HasPrefix(line, cancelledWritePrefix):
				if val, err := strconv.ParseUint(line[len(cancelledWritePrefix):], 10, 64); err != nil {
					// This always comes after write_bytes.
					if val > res.WriteBytes {
						res.WriteBytes = 0
					} else {
						res.WriteBytes -= val
					}
				}
			}
		}
	}

	return res
}
