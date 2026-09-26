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
}
