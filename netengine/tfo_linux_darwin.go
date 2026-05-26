//go:build linux || darwin

package netengine

import "golang.org/x/sys/unix"

// setSockOptTFO enables TCP Fast Open on the listening socket.
//
// On Linux the value is the TFO pending-connection queue depth (backlog);
// a value of 0 disables TFO.  The recommended default is 256.
//
// On Darwin (macOS) any non-zero value enables TFO; the kernel controls
// the actual queue depth via the net.inet.tcp.fastopen_backlog sysctl.
func setSockOptTFO(fd uintptr, queueLen int) error {
	return unix.SetsockoptInt(int(fd), unix.IPPROTO_TCP, unix.TCP_FASTOPEN, queueLen)
}
