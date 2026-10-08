package workflow

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
)

// What a session of the suite reports about itself while it runs: what
// `rewake list` says of it, until it is asked to stop.

// shimReportState keeps the session alive and writes what `rewake list` says
// about it. The session runs the command: an observation the test process made
// for itself would prove nothing about what a session can see.
func shimReportState() int {
	target := os.Getenv(shimStateFile)
	if err := insideACase(); err != nil {
		fmt.Fprintf(os.Stderr, "shim: %v\n", err)
		return 1
	}
	deadline := time.Now().Add(25 * time.Second)
	if os.Getenv(shimRequestDir) != "" {
		// A scenario may still ask for something after the usual state loop.
		deadline = time.Now().Add(requestLifetime)
	}
	for time.Now().Before(deadline) {
		if target != "" {
			out, err := exec.Command("rewake", "list", "--json").Output()
			if err == nil && len(out) > 0 {
				_ = os.WriteFile(target+".tmp", out, 0o600)
				_ = os.Rename(target+".tmp", target)
			}
		}
		if _, err := os.Stat(os.Getenv(shimExitFile)); err == nil {
			return 0
		}
		if os.Getenv(shimExitAfterTurn) != "" && endedATurn() {
			// Nobody asked it to stop. This is the control for "the session
			// was still there when the case was judged".
			return 0
		}
		time.Sleep(100 * time.Millisecond)
	}
	return 0
}

// workedATurn reports whether this session has recorded a turn yet. The two
// halves of the shim are separate processes, so the file is how the client
// learns what the server did.
func workedATurn() bool {
	turns := os.Getenv(shimTurnsFile)
	if turns == "" {
		return false
	}
	info, err := os.Stat(turns)
	return err == nil && info.Size() > 0
}

// endedATurn reports whether the fixture's program has ended a turn. Its turn
// records what it read before it ends, so a record alone may still have the
// end, and the report it carries, to come; the completed event is written
// after both.
func endedATurn() bool {
	turns := os.Getenv(shimTurnsFile)
	if turns == "" {
		return false
	}
	raw, err := os.ReadFile(turns)
	return err == nil && strings.Contains(string(raw), turnEventMark+"\tcompleted\t")
}
