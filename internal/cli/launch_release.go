//go:build !rewaketest

package cli

// directIdentity is a person's launch: always main, in the room of its
// directory.
func directIdentity(Call) (launchIdentity, error) { return ownRoom() }

// directOptions are the launch flags only the test build has.
func directOptions() []Option { return nil }
