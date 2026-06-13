//go:build darwin || freebsd || netbsd || openbsd

package netengine

import (
	"golang.org/x/sys/unix"
)

type kqueuePoller struct {
	fd      int // kqueue instance fd
	wakeR   int // wakeup pipe read-end fd (non-blocking)
	wakeW   int // wakeup pipe write-end fd
	scratch []unix.Kevent_t // reused across wait() calls; never escapes to heap
}

func newPoller(_ *eventLoop) (pollerBackend, error) {
	kfd, err := unix.Kqueue()
	if err != nil {
		return nil, err
	}
	var fds [2]int
	if err = unix.Pipe(fds[:]); err != nil {
		unix.Close(kfd)
		return nil, err
	}
	unix.SetNonblock(fds[0], true) //nolint:errcheck
	unix.SetNonblock(fds[1], true) //nolint:errcheck

	p := &kqueuePoller{fd: kfd, wakeR: fds[0], wakeW: fds[1],
		scratch: make([]unix.Kevent_t, 512)}
	// Register wakeup pipe read-end with EV_CLEAR (edge-like: drain on each edge).
	if _, err = unix.Kevent(kfd, []unix.Kevent_t{{
		Ident:  uint64(fds[0]), // #nosec G115 - fds[0] 是文件描述符，值很小
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_CLEAR,
	}}, nil, nil); err != nil {
		unix.Close(kfd)
		unix.Close(fds[0])
		unix.Close(fds[1])
		return nil, err
	}
	return p, nil
}

// add registers fd for one-shot read notification using EV_DISPATCH.
// EV_DISPATCH disables the filter after firing (like EPOLLONESHOT).
func (p *kqueuePoller) add(fd int) error {
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{{
		Ident:  uint64(fd), // #nosec G115 - fd 是文件描述符，值很小
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ADD | unix.EV_ENABLE | unix.EV_DISPATCH,
	}}, nil, nil)
	return err
}

// mod re-enables a dispatched (disabled) filter for the next event.
func (p *kqueuePoller) mod(fd int) error {
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{{
		Ident:  uint64(fd), // #nosec G115 - fd 是文件描述符，值很小
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_ENABLE | unix.EV_DISPATCH,
	}}, nil, nil)
	return err
}

func (p *kqueuePoller) del(fd int) error {
	_, err := unix.Kevent(p.fd, []unix.Kevent_t{{
		Ident:  uint64(fd), // #nosec G115 - fd 是文件描述符，值很小
		Filter: unix.EVFILT_READ,
		Flags:  unix.EV_DELETE,
	}}, nil, nil)
	return err
}

func (p *kqueuePoller) wait(events []pollEvent) (int, error) {
	if cap(p.scratch) < len(events) {
		p.scratch = make([]unix.Kevent_t, len(events))
	}
	kEvents := p.scratch[:len(events)]
	for {
		n, err := unix.Kevent(p.fd, nil, kEvents, nil)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return 0, err
		}
		count := 0
		for i := range n {
			ke := &kEvents[i]
			// Drain wakeup pipe internally.
			// #nosec G115 - ke.Ident 是文件描述符，在 BSD 系统上不会超过 int 最大值
			if int(ke.Ident) == p.wakeR {
				var buf [64]byte
				for {
					if _, e := unix.Read(p.wakeR, buf[:]); e != nil {
						break
					}
				}
				continue
			}
			// #nosec G115 - ke.Ident 是文件描述符，在 BSD 系统上不会超过 int 最大值
			events[count].fd = int(ke.Ident)
			events[count].readable = ke.Filter == unix.EVFILT_READ
			events[count].hangup = ke.Flags&unix.EV_EOF != 0
			events[count].errored = ke.Flags&unix.EV_ERROR != 0
			count++
		}
		return count, nil
	}
}

func (p *kqueuePoller) wakeup() {
	unix.Write(p.wakeW, []byte{1}) //nolint:errcheck
}

func (p *kqueuePoller) close() error {
	unix.Close(p.wakeR)
	unix.Close(p.wakeW)
	return unix.Close(p.fd)
}
