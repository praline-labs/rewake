package pi

import (
	"errors"
	"net"
	"syscall"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

func sameProcess(conn net.Conn, process adapter.RunProcess) error {
	unix, ok := conn.(*net.UnixConn)
	if !ok {
		return errors.New("the extension needs a unix socket")
	}
	raw, err := unix.SyscallConn()
	if err != nil {
		return err
	}
	var cred *syscall.Ucred
	var credErr error
	if err := raw.Control(func(fd uintptr) {
		cred, credErr = syscall.GetsockoptUcred(int(fd), syscall.SOL_SOCKET, syscall.SO_PEERCRED)
	}); err != nil {
		return err
	}
	if credErr != nil {
		return credErr
	}
	if process.PID <= 0 || process.Start == 0 || cred == nil || int(cred.Pid) != process.PID {
		return errors.New("the extension peer is not the harness process")
	}
	start, err := proc.StartTime(process.PID)
	if err != nil {
		return err
	}
	if start != process.Start {
		return errors.New("the harness process identity changed")
	}
	return nil
}
