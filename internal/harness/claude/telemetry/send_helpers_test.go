package telemetry

import "syscall"

func socketFor(string) (int, error) {
	return syscall.Socket(syscall.AF_UNIX, syscall.SOCK_DGRAM|syscall.SOCK_CLOEXEC, 0)
}

func closeSocket(fd int) { _ = syscall.Close(fd) }

func sendTo(fd int, raw []byte, path string) error {
	return syscall.Sendto(fd, raw, syscall.MSG_DONTWAIT, &syscall.SockaddrUnix{Name: path})
}
