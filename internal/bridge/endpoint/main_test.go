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

// helperChildHoldGreeted holds a child the same way after the endpoint
// admitted its hello and before it asks for the confirmation.
const helperChildHoldGreeted = "ENDPOINT_TEST_HOLD_GREETED"

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
	holdChild(os.Getenv(helperChildHold))
	if err := confirmHeld(filepath.Join(os.Getenv(state.DirEnv), "api.ctx"), ticket); err != nil {
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

// confirmHeld is Confirm, held between the hello and the request when the
// test asks for it.
func confirmHeld(path string, ticket bridge.Ticket) error {
	dir := os.Getenv(helperChildHoldGreeted)
	if dir == "" {
		return Confirm(path, ticket)
	}
	conn, err := greet(path, hello{Role: roleChild, Capability: ticket.Capability}, ConfirmWait)
	if err != nil {
		return err
	}
	defer func() { _ = conn.Close() }()
	holdChild(dir)
	return ask(conn, request{ID: 1, Op: opConfirm, Ticket: &ticket})
}

// holdChild leaves "started" in dir and waits, bounded, for "go"; an empty
// dir holds nothing.
func holdChild(dir string) {
	if dir == "" {
		return
	}
	_ = os.WriteFile(filepath.Join(dir, "started"), nil, 0o600)
	for until := time.Now().Add(10 * time.Second); time.Now().Before(until); time.Sleep(10 * time.Millisecond) {
		if _, err := os.Stat(filepath.Join(dir, "go")); err == nil {
			return
		}
	}
}
