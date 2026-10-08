package endpoint

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"
)

// Who is at the other end of a connection: its credentials, and whether it
// runs this build.

// peerOf reads the credentials of the process at the other end of a unix
// socket, as the kernel recorded them when it connected or listened.
func peerOf(conn *net.UnixConn) (*syscall.Ucred, error) {
	raw, err := conn.SyscallConn()
	if err != nil {
		return nil, err
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return nil, err
	}
	return cred, credErr
}

// sameBuild says whether a process runs the file this one runs: the same
// device and inode behind /proc/<pid>/exe. A file replaced on disk since is
// another inode, so a server or child started from a newer install is
// refused, however its version reads.
func sameBuild(pid int) error {
	own, err := os.Stat("/proc/self/exe")
	if err != nil {
		return err
	}
	theirs, err := os.Stat(fmt.Sprintf("/proc/%d/exe", pid))
	if err != nil {
		return err
	}
	if !os.SameFile(own, theirs) {
		return errors.New("its executable is another file")
	}
	return nil
}
