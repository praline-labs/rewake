package endpoint

import (
	"errors"
	"fmt"
	"net"
	"os"
	"syscall"

	"github.com/praline-labs/rewake/internal/infra/build"
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

// sameBuild says whether a process runs the build this one runs: the same
// content behind /proc/<pid>/exe, the identity a room's lease compares
// (docs/v2/stage3-state.md#one-build-per-room). A byte-identical copy is the
// same build; a file replaced on disk since is another one, however its
// version reads. The same file is the same content — a running image cannot
// be written — so only another file is read.
func sameBuild(pid int) error {
	theirs := fmt.Sprintf("/proc/%d/exe", pid)
	ownFile, err := os.Stat("/proc/self/exe")
	if err != nil {
		return err
	}
	theirFile, err := os.Stat(theirs)
	if err != nil {
		return err
	}
	if os.SameFile(ownFile, theirFile) {
		return nil
	}
	own, err := build.ID()
	if err != nil {
		return err
	}
	their, err := build.FileID(theirs)
	if err != nil {
		return err
	}
	if their != own {
		return errors.New("its executable is another build")
	}
	return nil
}
