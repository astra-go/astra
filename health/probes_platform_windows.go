//go:build windows

package health

func platformStatFS(s *unixFSStat, path string) error {
	// Windows doesn't have statfs; use GetDiskFreeSpaceEx via syscall.
	// For now, return an error indicating unsupported platform.
	s.err = ErrWindowsUnsupported
	return s.err
}

// ErrWindowsUnsupported indicates that disk probes are not supported on Windows.
var ErrWindowsUnsupported = ErrUnsupported("health: disk probes are not supported on Windows")

// ErrUnsupported is returned for platform-unsupported operations.
type ErrUnsupported string

func (e ErrUnsupported) Error() string { return string(e) }
