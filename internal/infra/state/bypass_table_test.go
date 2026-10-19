package state

// The writes outside the seam that are allowed, by file and effect, each
// with why (docs/v2/stage3-state.md#what-is-a-writer). Classified by effect
// and scope, never a whole package: what writes beside a checkout or makes no
// file at all, and the test helpers that are not _test.go files.
const (
	worktreeOutside = "host/worktree writes beside a checkout and in the user's data directory, outside the state: no room's lease covers it, its own locks order it"
	testHelper      = "a test helper that is not a _test.go file sets a scene a test writes, outside any build"
	workerSettings  = "the Claude Code worker profile's settings.json, replaced in one step when rewake adds its newer defaults, the person's file outside any room"
)

var bypassExceptions = []bypass{
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

	{"internal/adapter/pi/ship.go", "os.MkdirAll", 2, "pi's agent directory and its extensions and skills folders, the person's beside the alias file and outside any room: no room's lease covers them"},
	{"internal/adapter/pi/ship.go", "os.Chmod", 1, "pi's agent directory made private, the person's beside the alias file and outside any room"},

	{"internal/adapter/claude/workerdir.go", "os.MkdirAll", 2, "the Claude Code worker profile's directory and a template's memory in it, the person's beside the alias file and outside any room: no room's lease covers it"},
	{"internal/adapter/claude/workerdir.go", "os.OpenFile", 1, "the Claude Code worker profile's first files, made only where none is, outside any room"},
	{"internal/adapter/claude/workerdir.go", "os.CreateTemp", 1, workerSettings},
	{"internal/adapter/claude/workerdir.go", "os.Rename", 1, workerSettings},
	{"internal/adapter/claude/workerdir.go", "os.Remove", 1, workerSettings},

	{"internal/core/mail/mailtest/mailtest.go", "os.MkdirAll", 2, testHelper},
	{"internal/core/registry/registrytest/registrytest.go", "os.MkdirAll", 1, testHelper + ": a 1.x record no 2.0 lease covers"},
	{"internal/core/registry/registrytest/registrytest.go", "os.WriteFile", 1, testHelper + ": a 1.x record no 2.0 lease covers"},
	{"internal/infra/build/buildtest/buildtest.go", "os.Chmod", 1, testHelper + ": the base of a test's room, made private before the room exists"},
}
