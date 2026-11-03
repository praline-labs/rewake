package pi

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/sessiontools"
)

// toolRig is a run of role whose rewake is /r/rewake, and an extension's
// connection whose hello it answered.
func toolRig(t *testing.T, role string) (*link, frame) {
	t.Helper()
	rig := newRig(t, func(r *run) {
		r.tools = sessiontools.Run{Executable: "/r/rewake", Dir: "/state", Room: "room", Session: "api", Epoch: "7", Role: role}
	})
	conn, err := net.Dial("unix", rig.run.socket)
	if err != nil {
		t.Fatal(err)
	}
	l := newLink(conn)
	t.Cleanup(l.close)
	go l.serve(bufio.NewReader(conn), func(got frame) { l.answer(got, frame{OK: true}) })
	reply, err := l.ask(t.Context(), frame{Op: "hello", Protocol: protocolVersion, State: &sample{Sequence: 1, Idle: true}, Thread: "conversation"})
	if err != nil || !reply.OK {
		t.Fatalf("hello: %+v, %v", reply, err)
	}
	receive(t, rig.live)
	return l, reply
}

func toolNames(entries []toolEntry) []string {
	var names []string
	for _, entry := range entries {
		names = append(names, entry.Name)
	}
	return names
}

// The hello's answer is the table the run's role is offered, each tool with
// its label for pi's screen.
func TestAPiHellosAnswerIsTheRolesTable(t *testing.T) {
	_, worker := toolRig(t, "write")
	if got := toolNames(worker.Tools); !slices.Equal(got, []string{"worktree_status", "brief_read", "brief_tick", "brief_comment"}) {
		t.Fatalf("a worker is offered %v", got)
	}
	_, main := toolRig(t, "main")
	if got := toolNames(main.Tools); !slices.Equal(got, []string{"worktree_status", "worktree_list", "worktree_land"}) {
		t.Fatalf("main is offered %v", got)
	}
	if main.Tools[0].Label == "" || !json.Valid(main.Tools[0].InputSchema) {
		t.Fatalf("a tool as pi registers it: %+v", main.Tools[0])
	}
}

// A call is planned and its raw end worded here: the argv and bound, or the
// refusal the model reads; the output, or the failure with its error output.
func TestAPiCallIsPlannedAndItsEndWorded(t *testing.T) {
	l, _ := toolRig(t, "write")
	ask := func(request frame) frame {
		t.Helper()
		ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
		defer cancel()
		reply, err := l.ask(ctx, request)
		if err != nil {
			t.Fatal(err)
		}
		return reply
	}
	planned := ask(frame{Op: "tool-plan", Tool: "brief_tick", Arguments: json.RawMessage(`{"id":3,"item":1,"edition":2}`)})
	if !slices.Equal(planned.Argv, []string{"/r/rewake", "brief", "tick", "3", "1", "--edition=2"}) || planned.TimeoutMs <= 0 || planned.Refused != "" {
		t.Fatalf("the plan: %+v", planned)
	}
	if refused := ask(frame{Op: "tool-plan", Tool: "brief_tick", Arguments: json.RawMessage(`{}`)}); refused.Refused == "" || refused.Argv != nil {
		t.Fatalf("a tick without id: %+v", refused)
	}
	if refused := ask(frame{Op: "tool-plan", Tool: "worktree_land", Arguments: json.RawMessage(`{}`)}); !strings.Contains(refused.Refused, "is not a tool of this session") {
		t.Fatalf("a worker's land: %+v", refused)
	}
	zero, one := 0, 1
	if worded := ask(frame{Op: "tool-result", Tool: "brief_read", Ran: &sessiontools.Ran{ExitCode: &zero, Stdout: "the brief\n"}}); worded.IsError || worded.Text != "the brief" {
		t.Fatalf("an output: %+v", worded)
	}
	failed := ask(frame{Op: "tool-result", Tool: "brief_tick", Ran: &sessiontools.Ran{ExitCode: &one, Stderr: "edition 1 is old"}})
	if !failed.IsError || !strings.Contains(failed.Text, "edition 1 is old") {
		t.Fatalf("a failure: %+v", failed)
	}
	if killed := ask(frame{Op: "tool-result", Tool: "brief_tick", Ran: &sessiontools.Ran{Killed: true}}); !killed.IsError || !strings.Contains(killed.Text, "abort") {
		t.Fatalf("a killed command: %+v", killed)
	}
}

// An extension of the protocol before is refused with the host's reason.
func TestAnOlderExtensionIsToldToRestartTheRoom(t *testing.T) {
	rig := newRig(t, nil)
	conn, err := net.Dial("unix", rig.run.socket)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close() }()
	l := newLink(conn)
	if err := l.send(frame{Op: "hello", ID: 1, Protocol: protocolVersion - 1, State: &sample{Sequence: 1, Idle: true}, Thread: "conversation"}); err != nil {
		t.Fatal(err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(time.Second))
	reply, err := readFrame(bufio.NewReader(conn))
	if err != nil || reply.OK || reply.Error != olderProtocol {
		t.Fatalf("an older hello: %+v %v", reply, err)
	}
}
