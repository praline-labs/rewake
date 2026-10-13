package state

// The writes outside the seam that are allowed, by file and effect, each
// with why (docs/v2/stage3-state.md#what-is-a-writer). Classified by effect
// and scope, never a whole package: what writes beside a checkout or makes no
// file at all, and the test helpers that are not _test.go files.
const (
	worktreeOutside = "host/worktree writes beside a checkout and in the user's data directory, outside the state: no room's lease covers it, its own locks order it"
	testHelper      = "a test helper that is not a _test.go file sets a scene a test writes, outside any build"
)

var bypassExceptions = []bypass{
	{"internal/core/grant/authority/server.go", "net.ListenUnix", 1, "the grant authority binds an abstract socket, which makes no file"},

	{"internal/host/terminal/pty.go", "os.OpenFile", 2, "a pseudo-terminal's master and side, device nodes opened read and write, which make no file"},

	{"internal/host/worktree/recordfile.go", "os.CreateTemp", 1, worktreeOutside + ": the worktree records"},
	{"internal/host/worktree/recordfile.go", "os.Chmod", 1, worktreeOutside + ": the worktree records"},
	{"internal/host/worktree/recordfile.go", "os.Rename", 1, worktreeOutside + ": the worktree records"},
	{"internal/host/worktree/recordfile.go", "os.Link", 1, worktreeOutside + ": the worktree records"},
	{"internal/host/worktree/recordfile.go", "os.Remove", 3, worktreeOutside + ": the worktree records"},
	{"internal/host/worktree/serial.go", "os.OpenFile", 1, worktreeOutside + ": the serial lock of a checkout"},
	{"internal/host/worktree/serial.go", "syscall.Flock", 2, worktreeOutside + ": the serial lock of a checkout"},
	{"internal/host/worktree/include.go", "os.OpenFile", 1, worktreeOutside + ": the copies of included files"},
	{"internal/host/worktree/include.go", "os.Remove", 1, worktreeOutside + ": the copies of included files"},
	{"internal/host/worktree/include.go", "os.Mkdir", 1, worktreeOutside + ": the copies of included files"},
	{"internal/host/worktree/hooks.go", "os.CreateTemp", 1, worktreeOutside + ": a hook's temporary file"},
	{"internal/host/worktree/hooks.go", "os.Remove", 1, worktreeOutside + ": a hook's temporary file"},
	{"internal/host/worktree/remove.go", "os.RemoveAll", 1, worktreeOutside + ": the removal of a checkout"},
	{"internal/host/worktree/remove.go", "os.Remove", 2, worktreeOutside + ": the removal of a checkout"},
	{"internal/host/worktree/worktree.go", "os.MkdirAll", 2, worktreeOutside + ": the directories of checkouts and records"},
	{"internal/host/worktree/worktree.go", "os.Remove", 4, worktreeOutside + ": a checkout's directory and record, undone"},

	{"internal/core/mail/mailtest/mailtest.go", "os.MkdirAll", 2, testHelper},
	{"internal/core/registry/registrytest/registrytest.go", "os.MkdirAll", 1, testHelper + ": a 1.x record no 2.0 lease covers"},
	{"internal/core/registry/registrytest/registrytest.go", "os.WriteFile", 1, testHelper + ": a 1.x record no 2.0 lease covers"},
	{"internal/infra/build/buildtest/buildtest.go", "os.Chmod", 1, testHelper + ": the base of a test's room, made private before the room exists"},
}
