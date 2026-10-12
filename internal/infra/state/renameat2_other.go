//go:build !amd64

package state

import "syscall"

// sysRenameat2 is renameat2's number, as the syscall package names it.
const sysRenameat2 = syscall.SYS_RENAMEAT2
