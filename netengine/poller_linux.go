//go:build linux

package netengine

import (
	"golang.org/x/sys/unix"
)

type epollPoller struct {
	fd      int // epoll instance fd
	wakeR   int // wakeup pipe read-end fd (non-blocking)
	wakeW   int // wakeup pipe write-end fd
	scratch []unix.EpollEvent // reused across wait() calls; never escapes to heap
}

func newPoller(_ *eventLoop) (pollerBackend, error) {
	efd, err := unix.EpollCreate1(unix.EPOLL_CLOEXEC)
	if err != nil {
		return nil, err
	}
	var fds [2]int
	if err = unix.Pipe2(fds[:], unix.O_NONBLOCK|unix.O_CLOEXEC); err != nil {
		unix.Close(efd)
		return nil, err
	}
	p := &epollPoller{fd: efd, wakeR: fds[0], wakeW: fds[1],
		scratch: make([]unix.EpollEvent, 512)}
	// Register the wakeup pipe's read end for level-triggered read.
	if err = unix.EpollCtl(efd, unix.EPOLL_CTL_ADD, fds[0], &unix.EpollEvent{
		Events: unix.EPOLLIN,
		Fd:     int32(fds[0]),
	}); err != nil {
		unix.Close(efd)
		unix.Close(fds[0])
		unix.Close(fds[1])
		return nil, err
	}
	return p, nil
}

// add registers fd with EPOLLIN | EPOLLRDHUP | EPOLLONESHOT.
func (p *epollPoller) add(fd int) error {
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_ADD, fd, &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLONESHOT,
		Fd:     int32(fd),
	})
}

// mod re-arms fd after a one-shot event fired.
func (p *epollPoller) mod(fd int) error {
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_MOD, fd, &unix.EpollEvent{
		Events: unix.EPOLLIN | unix.EPOLLRDHUP | unix.EPOLLONESHOT,
		Fd:     int32(fd),
	})
}

func (p *epollPoller) del(fd int) error {
	return unix.EpollCtl(p.fd, unix.EPOLL_CTL_DEL, fd, nil)
}

func (p *epollPoller) wait(events []pollEvent) (int, error) {
	if cap(p.scratch) < len(events) {
		p.scratch = make([]unix.EpollEvent, len(events))
	}
	epEvents := p.scratch[:len(events)]
	for {
		n, err := unix.EpollWait(p.fd, epEvents, -1)
		if err != nil {
			if err == unix.EINTR {
				continue
			}
			return 0, err
		}
		count := 0
		for i := 0; i < n; i++ {
			ev := &epEvents[i]
			// Drain wakeup pipe internally; don't surface to caller.
			if int(ev.Fd) == p.wakeR {
				var buf [64]byte
				for {
					if _, e := unix.Read(p.wakeR, buf[:]); e != nil {
						break // EAGAIN or EOF
					}
				}
				continue
			}
			events[count].fd = int(ev.Fd)
			events[count].readable = ev.Events&(unix.EPOLLIN|unix.EPOLLRDHUP) != 0
			events[count].hangup = ev.Events&unix.EPOLLHUP != 0
			events[count].errored = ev.Events&unix.EPOLLERR != 0
			count++
		}
		return count, nil
	}
}

func (p *epollPoller) wakeup() {
	unix.Write(p.wakeW, []byte{1}) //nolint:errcheck
}

func (p *epollPoller) close() error {
	unix.Close(p.wakeR)
	unix.Close(p.wakeW)
	return unix.Close(p.fd)
}
