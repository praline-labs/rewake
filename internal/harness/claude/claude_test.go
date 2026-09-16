package claude

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness"
	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
)

func launch(t *testing.T, args []string, socket string) harness.LaunchPlan {
	t.Helper()
	plan, err := New().Launch(harness.LaunchRequest{
		Name:   "api",
		Dir:    t.TempDir(),
		Args:   args,
		Intro:  true,
		Socket: socket,
	})
	if err != nil {
		t.Fatalf("Launch: %v", err)
	}
	return plan
}

func indexOf(args []string, value string) int {
	for index, arg := range args {
		if arg == value {
			return index
		}
	}
	return -1
}

// Everything after "--" is a positional argument of the harness — a prompt, as
// a rule. A flag appended there stops being a flag and becomes part of it.
func TestAddedFlagsStayBeforeTheTerminator(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	plan := launch(t, []string{"--model", "haiku", "--", "write the release notes"}, socket)

	terminator := indexOf(plan.Args, "--")
	if terminator < 0 {
		t.Fatalf("the terminator disappeared from %v", plan.Args)
	}
	for _, flag := range []string{socketFlag, introFlag, toolFlag} {
		at := indexOf(plan.Args, flag)
		if at < 0 {
			t.Errorf("%s was not added: %v", flag, plan.Args)
			continue
		}
		if at > terminator {
			t.Errorf("%s was added after the terminator: %v", flag, plan.Args)
		}
	}
	if plan.Args[len(plan.Args)-1] != "write the release notes" {
		t.Errorf("the prompt is no longer last: %v", plan.Args)
	}
}

func TestCallerSocketIsNotOurs(t *testing.T) {
	theirs := filepath.Join(t.TempDir(), "theirs.sock")
	plan := launch(t, []string{socketFlag, theirs}, filepath.Join(t.TempDir(), "ours.sock"))

	if plan.Socket != theirs {
		t.Errorf("socket = %q, want the caller's %q", plan.Socket, theirs)
	}
	if plan.OwnsSocket {
		t.Error("a socket named by the caller was claimed as ours; ending the session would delete their file")
	}
	if count := strings.Count(strings.Join(plan.Args, " "), socketFlag); count != 1 {
		t.Errorf("the socket flag appears %d times, want 1", count)
	}
}

func TestOwnSocketIsClaimed(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	plan := launch(t, nil, socket)

	if plan.Socket != socket || !plan.OwnsSocket {
		t.Errorf("socket = %q owned = %v; want our own path, owned", plan.Socket, plan.OwnsSocket)
	}
}

func TestLongSocketPathIsRefused(t *testing.T) {
	long := "/tmp/" + strings.Repeat("d", 120) + ".sock"
	_, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Socket: long})
	if err == nil {
		t.Fatal("a socket path over the kernel limit was accepted")
	}
}

// Only "connection refused" proves the owner is gone. Any other failure is about
// this moment, and deleting the file then cuts off a session that is running.
func TestLiveSocketIsNeverRemoved(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "live.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer listener.Close()

	_, err = New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Socket: socket})
	if err == nil {
		t.Fatal("launching over a live socket was allowed")
	}
	if _, statErr := os.Stat(socket); statErr != nil {
		t.Errorf("the live socket was removed: %v", statErr)
	}
}

func TestStaleSocketIsReplaced(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "stale.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	// Closing without unlinking is what a killed session leaves behind.
	if unix, ok := listener.(*net.UnixListener); ok {
		unix.SetUnlinkOnClose(false)
	}
	listener.Close()

	if _, err := New().Launch(harness.LaunchRequest{Name: "api", Dir: t.TempDir(), Socket: socket}); err != nil {
		t.Fatalf("Launch over a stale socket: %v", err)
	}
	if _, err := os.Stat(socket); !os.IsNotExist(err) {
		t.Error("the stale socket was left in place, so the harness cannot bind it")
	}
}

// listenOnce accepts one connection and returns what was written to it.
func listenOnce(t *testing.T, socket string) <-chan string {
	t.Helper()
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	received := make(chan string, 1)
	go func() {
		defer listener.Close()
		connection, err := listener.Accept()
		if err != nil {
			received <- ""
			return
		}
		defer connection.Close()
		buffer := make([]byte, 1<<16)
		read, _ := connection.Read(buffer)
		received <- string(buffer[:read])
	}()
	return received
}

func TestDeliveryWritesOneProtocolLine(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	received := listenOnce(t, socket)

	session := registry.Session{Name: "api", Socket: socket}
	message := inbox.Message{ID: "1789544450793-34464b10aaaa", From: "web", To: "api", Text: "pull and rerun the smoke"}

	result := New().Deliver(context.Background(), session, message)
	if result.State != inbox.Delivered || result.Via != "socket" {
		t.Fatalf("result = %+v, want delivered via socket", result)
	}

	select {
	case line := <-received:
		if !strings.HasSuffix(line, "\n") {
			t.Error("the line was not terminated, so the receiver never parses it")
		}
		var envelope struct {
			Type     string `json:"type"`
			Priority string `json:"priority"`
			Message  struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"message"`
		}
		if err := json.Unmarshal([]byte(strings.TrimSpace(line)), &envelope); err != nil {
			t.Fatalf("the delivered line is not the protocol envelope: %v", err)
		}
		if envelope.Type != "user" || envelope.Message.Role != "user" || envelope.Priority != "next" {
			t.Errorf("envelope = %+v, want a user message at priority next", envelope)
		}
		if !strings.Contains(envelope.Message.Content, message.Text) {
			t.Errorf("the text did not survive delivery: %q", envelope.Message.Content)
		}
		if !strings.Contains(envelope.Message.Content, "rewake send web") {
			t.Errorf("the reply command is missing: %q", envelope.Message.Content)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("nothing arrived on the socket")
	}
}

// A write that succeeds tells us nothing when the receiver drops the line for
// being too long: reporting that as delivered loses the message silently.
func TestOversizeMessageIsRefusedBeforeSending(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "api.sock")
	received := listenOnce(t, socket)

	session := registry.Session{Name: "api", Socket: socket}
	message := inbox.Message{ID: inboxID, From: "web", To: "api", Text: strings.Repeat("x", 2<<20)}

	result := New().Deliver(context.Background(), session, message)
	if result.State != inbox.Failed {
		t.Fatalf("result = %+v, want failed", result)
	}
	if !strings.Contains(result.Detail, "limit") {
		t.Errorf("the refusal does not explain the limit: %q", result.Detail)
	}
	select {
	case line := <-received:
		t.Fatalf("an oversize message was sent anyway (%d bytes)", len(line))
	case <-time.After(300 * time.Millisecond):
	}
}

const inboxID = "1789544450793-34464b10aaaa"

func TestMissingSocketIsPendingNotFailed(t *testing.T) {
	session := registry.Session{Name: "api", Socket: filepath.Join(t.TempDir(), "missing.sock")}
	result := New().Deliver(context.Background(), session, inbox.Message{ID: inboxID, From: "web", Text: "hello"})

	if result.State != inbox.Pending {
		t.Fatalf("result = %+v, want pending: the socket appears a moment after the process", result)
	}
}
