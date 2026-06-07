//go:build !windows

package health

import "syscall"

func platformStatFS(s *unixFSStat, path string) error {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return err
	}
	s.bsizeF = func() uint64 { return uint64(stat.Bsize) }
	s.bavailF = func() uint64 { return uint64(stat.Bavail) }
	return nil
}
