//go:build !linux && !darwin && !freebsd && !netbsd && !openbsd && !windows

package netengine

import "errors"

// errNotSupported is returned on platforms without epoll or kqueue support.
var errNotSupported = errors.New("netengine: Reactor engine requires Linux (epoll) or macOS/BSD (kqueue)")

// newPoller returns an error on unsupported platforms.
// The Engine falls back to net/http on these platforms.
func newPoller(_ *eventLoop) (pollerBackend, error) {
	return nil, errNotSupported
}

// noopPoller satisfies the interface but is never actually used.
type noopPoller struct{}

func (noopPoller) add(int) error              { return errNotSupported }
func (noopPoller) mod(int) error              { return errNotSupported }
func (noopPoller) del(int) error              { return errNotSupported }
func (noopPoller) wait([]pollEvent) (int, error) { return 0, errNotSupported }
func (noopPoller) wakeup()                   {}
func (noopPoller) close() error              { return nil }
