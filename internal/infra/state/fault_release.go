//go:build !rewaketest

package state

// FaultEnv is what a process passes on to the processes it starts for their
// faults: nothing, in a build without the rewaketest tag.
func FaultEnv() []string { return nil }
