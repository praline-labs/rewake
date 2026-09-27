package harness

// WorktreeHarness is a harness whose worktree flag rewake takes for itself: the
// harness cannot make its own checkout under rewake, so rewake makes one and
// starts the launch in it (docs/launch.md, "A worktree for a launch").
type WorktreeHarness interface {
	// WorktreeFlag is the harness's own spelling of the flag, which a person
	// already knows; rewake reads it as a switch, or with =<name>.
	WorktreeFlag() string
	// LaunchDirectory is the directory the launch would work in, and the
	// arguments without what chose it: in the checkout, the launch starts
	// in the same place within it, and a flag naming the old one would lead
	// back out.
	LaunchDirectory(args []string) (string, []string, error)
	// WorktreeRefusal says why a launch with these arguments cannot run in a
	// checkout rewake makes, or nil when it can. It is asked before anything
	// is made, so a refused launch leaves nothing behind.
	WorktreeRefusal(args []string) error
}

// ContinueInWorktree is the way on that a refused continuation names: the
// conversation goes on where it was started, and one started in a checkout
// rewake made is found with rewake worktree ls. Launched in the checkout
// without the flag, the harness finds it where it left it.
func ContinueInWorktree(launch, word string) string {
	return "continue it where it was started: for a conversation begun in a rewake worktree, rewake worktree ls names the worktree's path; cd there and run " + launch + " " + word + " without --worktree"
}
