//go:build !rewakefault

package state

// FaultEnv is what a process passes on to the processes it starts for their
// faults: nothing, in a build without the rewakefault tag.
func FaultEnv() []string { return nil }
