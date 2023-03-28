//go:build !linux

package governor

import "syscall"

func getProcStats() procStats {
	var ru syscall.Rusage
	if err := syscall.Getrusage(syscall.RUSAGE_SELF, &ru); err != nil {
		return procStats{}
	}
	return procStats{
		CPU:            uint32(ru.Utime.Sec*1000) + uint32(ru.Utime.Usec/1000) + uint32(ru.Stime.Sec*1000) + uint32(ru.Stime.Usec/1000),
		RSS:            uint64(ru.Maxrss),
		DiskReadBytes:  uint64(ru.Inblock),
		DiskWriteBytes: uint64(ru.Oublock),
	}
}
