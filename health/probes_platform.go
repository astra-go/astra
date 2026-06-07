package health

import (
	"runtime"
)

// memoryStat provides cross-platform memory stats.
type memoryStat struct{}

// available returns the estimated available memory in bytes.
func (m *memoryStat) available() uint64 {
	var s runtime.MemStats
	runtime.ReadMemStats(&s)
	// Sys is total memory obtained from OS, HeapIdle is returned to OS
	// This is an approximation; for precise values, use syscall.Sysinfo on Linux
	return s.Sys - s.HeapInuse
}

// unixFSStat abstracts statfs for cross-platform disk checks.
type unixFSStat struct {
	bsizeF  func() uint64
	bavailF func() uint64
	err     error
}

// statFS populates the stat using platform-specific syscalls.
func (s *unixFSStat) statFS(path string) error {
	// Platform-specific implementation
	return platformStatFS(s, path)
}

func (s *unixFSStat) bsize() uint64 {
	if s.bsizeF != nil {
		return s.bsizeF()
	}
	return 4096 // default block size
}

func (s *unixFSStat) bavail() uint64 {
	if s.bavailF != nil {
		return s.bavailF()
	}
	return 0
}
