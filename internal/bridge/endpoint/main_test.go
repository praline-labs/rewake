package endpoint

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
	"github.com/praline-labs/rewake/internal/state"
)

// The test binary plays two other processes when run again: a call's child,
// as the CLI in bridge mode is, and a process below the transport asking for a
// call, as a command the model runs would.

// helperTransport names the endpoint a helper run asks a call of.
const helperTransport = "ENDPOINT_TEST_TRANSPORT"

// helperChildPrint makes a child print this many bytes after its words.
const helperChildPrint = "ENDPOINT_TEST_PRINT"

// helperChildHold names a directory where a child, before it confirms its
// ticket, leaves "started" and waits for "go".
const helperChildHold = "ENDPOINT_TEST_HOLD"

func TestMain(m *testing.M) {
	switch {
	case os.Getenv(bridge.TicketEnv) != "":
		os.Exit(runAsChild())
	case os.Getenv(helperTransport) != "":
		_, err := CallTool(os.Getenv(helperTransport), ToolCall{Tool: "inbox", CallID: "helper", Conversation: "th", Turn: "t1"}, 5*time.Second)
		fmt.Print(err)
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// runAsChild confirms its ticket with the endpoint of its run, as the CLI
// does, and prints its words.
func runAsChild() int {
	ticket, err := bridge.ReadTicket()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	if dir := os.Getenv(helperChildHold); dir != "" {
		_ = os.WriteFile(filepath.Join(dir, "started"), nil, 0o600)
		for until := time.Now().Add(10 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
				break
			}
		}
	}
	if err := Confirm(filepath.Join(os.Getenv(state.DirEnv), "api.ctx"), ticket); err != nil {
		fmt.Fprintln(os.Stderr, "not confirmed:", err)
		return 1
	}
	fmt.Println("ran:", strings.Join(os.Args[1:], " "))
	if size := os.Getenv(helperChildPrint); size != "" {
		var n int
		_, _ = fmt.Sscan(size, &n)
		fmt.Print(strings.Repeat("x", n))
	}
	return 0
}
