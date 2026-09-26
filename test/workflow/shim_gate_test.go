package workflow

import (
	"os"
	"time"
)

// shimReadGate makes a session stop before it reads its mail, after the notice
// has started its turn, until the scenario creates the file this names. While
// it waits, <file>.waiting exists. That is the moment a sender can act on a
// message whose notice has gone out and that nobody has read: withdraw it,
// replace it, add to it.
const shimReadGate = "RW_SHIM_READ_GATE"

// readGateWait bounds the wait, so a scenario that never opens the gate ends
// the turn rather than holding the session up for good.
const readGateWait = 60 * time.Second

// waitAtReadGate is called by either fixture just before it reads the mailbox.
// Once the gate is open every later read passes straight through.
func waitAtReadGate() {
	gate := os.Getenv(shimReadGate)
	if gate == "" {
		return
	}
	if _, err := os.Stat(gate); err == nil {
		return
	}
	_ = os.WriteFile(gate+".waiting", nil, 0o600)
	deadline := time.Now().Add(readGateWait)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(gate); err == nil {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
}
