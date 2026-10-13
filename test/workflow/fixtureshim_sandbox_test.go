package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"slices"
	"syscall"
	"unsafe"
)

// A shell whose sandbox cannot connect() to any socket, as a harness that
// confines the commands its model runs leaves it (docs/v2/stage3-decision.md#the-command):
// the program re-executes itself with fixtureSandboxEnv, installs a seccomp
// filter that answers connect with EPERM, and executes rewake under it. The
// filter outlives execve and every process the command starts inherits it,
// so nothing below the shell reaches the wrapper's address; the wrapper's own
// children, the tool's among them, are outside it.

// fixtureSandboxEnv marks the re-executed program that installs the filter.
const fixtureSandboxEnv = "RW_SHIM_SANDBOXED"

// connectSyscall is connect's number on the architectures the filter is
// written for, with the audit architecture seccomp reports for each.
var connectSyscall = map[string]struct{ arch, nr uint32 }{
	"amd64": {0xc000003e, 42},
	"arm64": {0xc00000b7, 203},
}

// sandboxSupported says whether this machine's architecture has a filter.
func sandboxSupported() bool {
	_, ok := connectSyscall[runtime.GOARCH]
	return ok
}

// sandboxedCommand is rewake run with args below the filter.
func sandboxedCommand(env []string, args ...string) (*exec.Cmd, error) {
	self, err := os.Executable()
	if err != nil {
		return nil, err
	}
	command := exec.Command(self, args...)
	command.Env = append(slices.Clone(env), fixtureSandboxEnv+"=1")
	return command, nil
}

// runSandboxed installs the filter on this thread and becomes rewake with
// args; it returns only when either fails.
func runSandboxed(args []string) int {
	call, ok := connectSyscall[runtime.GOARCH]
	if !ok {
		fmt.Fprintf(os.Stderr, "sandbox: no filter for %s\n", runtime.GOARCH)
		return 2
	}
	rewake, err := exec.LookPath("rewake")
	if err != nil {
		fmt.Fprintf(os.Stderr, "sandbox: %v\n", err)
		return 2
	}
	const (
		ldAbsWord  = 0x20 // BPF_LD | BPF_W | BPF_ABS
		jumpEqual  = 0x15 // BPF_JMP | BPF_JEQ | BPF_K
		returnK    = 0x06 // BPF_RET | BPF_K
		allow      = 0x7fff0000
		errno      = 0x00050000
		archOffset = 4
		nrOffset   = 0
	)
	program := []syscall.SockFilter{
		{Code: ldAbsWord, K: archOffset},
		{Code: jumpEqual, Jt: 0, Jf: 3, K: call.arch},
		{Code: ldAbsWord, K: nrOffset},
		{Code: jumpEqual, Jt: 0, Jf: 1, K: call.nr},
		{Code: returnK, K: errno | uint32(syscall.EPERM)},
		{Code: returnK, K: allow},
	}
	filter := syscall.SockFprog{Len: uint16(len(program)), Filter: &program[0]}
	// The filter and the exec on one thread: a filter binds the thread that
	// installs it, and execve keeps the calling thread's.
	runtime.LockOSThread()
	const (
		setNoNewPrivs = 38
		setSeccomp    = 22
		modeFilter    = 2
	)
	if _, _, e := syscall.RawSyscall(syscall.SYS_PRCTL, setNoNewPrivs, 1, 0); e != 0 {
		fmt.Fprintf(os.Stderr, "sandbox: no_new_privs: %v\n", e)
		return 2
	}
	if _, _, e := syscall.RawSyscall(syscall.SYS_PRCTL, setSeccomp, modeFilter, uintptr(unsafe.Pointer(&filter))); e != 0 {
		fmt.Fprintf(os.Stderr, "sandbox: seccomp: %v\n", e)
		return 2
	}
	env := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		if len(entry) < len(fixtureSandboxEnv) || entry[:len(fixtureSandboxEnv)] != fixtureSandboxEnv {
			env = append(env, entry)
		}
	}
	err = syscall.Exec(rewake, append([]string{"rewake"}, args...), env)
	fmt.Fprintf(os.Stderr, "sandbox: exec %s: %v\n", rewake, err)
	return 2
}
