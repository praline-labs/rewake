package telemetry

import "syscall"

// Send hands one event to the wrapper listening at path and returns at once.
//
// A datagram socket, written without waiting: when the wrapper is gone the
// send fails on the spot, and when its queue is full the kernel answers EAGAIN
// instead of blocking. Nothing is read back and nothing is retried — a lost
// event costs one stale value, while a hook that waited would cost every
// prompt of the session.
func Send(path string, event Event) {
	if path == "" {
		return
	}
	if event.At == 0 {
		event.At = processStarted
	}
	raw, err := event.encode()
	if err != nil {
		return
	}
	fd, err := syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC|syscall.SOCK_NONBLOCK, 0)
	if err != nil {
		return
	}
	defer func() { _ = syscall.Close(fd) }()
	_ = syscall.Sendto(fd, raw, syscall.MSG_DONTWAIT, &syscall.SockaddrUnix{Name: path})
}
