package modhost

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/sessiontools"
)

// toolServing is a worker's endpoint whose run names the rewake it was
// started by, with the hello of a load said, whose word on entry is enter.
func toolServing(t *testing.T, enter bool) (string, HelloAnswer) {
	t.Helper()
	_, socket := servingWith(t, func(s *Server) {
		s.cfg.Role = "write"
		s.cfg.Tools = sessiontools.Run{Executable: "/r/rewake", Dir: "/state", Room: "room", Session: "write-1", Epoch: "7", Role: "write"}
	})
	hello := HelloRequest{Protocol: Protocol, LoadID: "load", SessionID: "conversation-1", HarnessVersion: "2.1.310", LoadSeq: 1}
	if enter {
		hello.Enter = &enter
	}
	var answer HelloAnswer
	if err := json.Unmarshal([]byte(postValue(t, socket, PathHello, hello)), &answer); err != nil {
		t.Fatal(err)
	}
	return socket, answer
}

func names(tools []ToolEntry) []string {
	var all []string
	for _, tool := range tools {
		all = append(all, tool.Name)
	}
	return all
}

// The hello's answer is the table the role is offered, with worktree_enter
// only for a load whose run may enter a checkout, each under its own digest.
func TestAHellosAnswerIsTheRolesTable(t *testing.T) {
	_, without := toolServing(t, false)
	_, with := toolServing(t, true)
	if got := names(without.Tools); !slices.Equal(got, []string{"worktree_status", "brief_read", "brief_tick", "brief_comment"}) {
		t.Fatalf("a worker without entry is offered %v", got)
	}
	if got := names(with.Tools); got[len(got)-1] != "worktree_enter" || len(got) != 5 {
		t.Fatalf("a worker with entry is offered %v", got)
	}
	if without.ToolsDigest == "" || without.ToolsDigest == with.ToolsDigest {
		t.Fatalf("the digests %q and %q", without.ToolsDigest, with.ToolsDigest)
	}
	var schema map[string]any
	if err := json.Unmarshal(with.Tools[0].InputSchema, &schema); err != nil || schema["type"] != "object" {
		t.Fatalf("a tool's schema is %s", with.Tools[0].InputSchema)
	}
}

// A plan is the command's argv, its variables in order and its bound; a call
// the table refuses, or arguments that are no object, are refused with the
// text the model reads; a load the run never admitted is refused outright.
func TestAPlanIsTheCommandOrTheRefusal(t *testing.T) {
	socket, _ := toolServing(t, true)
	plan := func(tool, arguments string) ToolPlan {
		t.Helper()
		var answer ToolPlan
		request := ToolPlanRequest{LoadID: "load", Tool: tool, Arguments: json.RawMessage(arguments)}
		if err := json.Unmarshal([]byte(postValue(t, socket, PathToolPlan, request)), &answer); err != nil {
			t.Fatal(err)
		}
		return answer
	}
	tick := plan("brief_tick", `{"id":94,"item":2,"edition":1}`)
	if tick.Refused != nil || !slices.Equal(tick.Argv, []string{"/r/rewake", "brief", "tick", "94", "2", "--edition=1"}) {
		t.Fatalf("brief_tick planned %+v", tick)
	}
	if tick.TimeoutMs == nil || *tick.TimeoutMs <= 0 {
		t.Fatalf("brief_tick has no bound: %+v", tick)
	}
	if !slices.IsSortedFunc(tick.Env, func(a, b EnvVar) int { return strings.Compare(a.Name, b.Name) }) {
		t.Fatalf("the variables are out of order: %v", tick.Env)
	}
	enter := plan("worktree_enter", `{}`)
	if !slices.Contains(enter.Env, EnvVar{Name: "REWAKE_WORKTREE_ENTER", Value: "1"}) {
		t.Fatalf("worktree_enter runs without entry: %v", enter.Env)
	}
	if refused := plan("worktree_land", `{}`); refused.Refused == nil || refused.Argv != nil {
		t.Fatalf("a worker's worktree_land planned %+v", refused)
	}
	if refused := plan("brief_tick", `[1]`); refused.Refused == nil || *refused.Refused != "brief_tick's arguments are no JSON object" {
		t.Fatalf("an array's plan: %+v", refused)
	}
	body, _ := json.Marshal(ToolPlanRequest{LoadID: "stranger", Tool: "worktree_status", Arguments: json.RawMessage(`{}`)})
	if status, _, err := post(socket, PathToolPlan, body); err != nil || status != http.StatusBadRequest {
		t.Fatalf("a stranger's plan: %d %v", status, err)
	}
}

func result(t *testing.T, socket, tool string, ran ToolRan) ToolAnswer {
	t.Helper()
	var answer ToolAnswer
	request := ToolResultRequest{LoadID: "load", Tool: tool, Ran: ran}
	if err := json.Unmarshal([]byte(postValue(t, socket, PathToolResult, request)), &answer); err != nil {
		t.Fatal(err)
	}
	return answer
}

// A raw end is worded here: an output as the answer, a failure with its
// error output, an unstarted command as one that did not end; worktree_enter's
// output names the checkout the mod moves into, and an output that names none
// is an error that moves nothing.
func TestARawEndIsWordedByTheWrapper(t *testing.T) {
	socket, _ := toolServing(t, true)
	zero, one := 0, 1
	if got := result(t, socket, "brief_read", ToolRan{ExitCode: &zero, Stdout: "the brief\n"}); got.IsError || got.Text != "the brief" {
		t.Fatalf("an output: %+v", got)
	}
	failed := result(t, socket, "brief_read", ToolRan{ExitCode: &one, Stderr: "no such brief"})
	if !failed.IsError || !strings.Contains(failed.Text, "no such brief") || !strings.Contains(failed.Text, "exit 1") {
		t.Fatalf("a failure: %+v", failed)
	}
	unstarted := "spawn ENOENT"
	if got := result(t, socket, "brief_read", ToolRan{Error: &unstarted}); !got.IsError || !strings.Contains(got.Text, unstarted) {
		t.Fatalf("an unstarted command: %+v", got)
	}
	made := `{"checkout":{"path":"/trees/fix"}}`
	if got := result(t, socket, "worktree_enter", ToolRan{ExitCode: &zero, Stdout: made}); got.IsError || got.EnterWorktree == nil || *got.EnterWorktree != "/trees/fix" {
		t.Fatalf("a checkout made: %+v", got)
	}
	if got := result(t, socket, "worktree_enter", ToolRan{ExitCode: &zero, Stdout: "{}"}); !got.IsError || got.EnterWorktree != nil {
		t.Fatalf("an answer naming no checkout: %+v", got)
	}
}

// The move's wording: in the checkout already, entered, refused, failed, or
// no answer, each failure saying the checkout stays the worker's.
func TestTheMoveIsWordedAsClaudeCodeAnswered(t *testing.T) {
	socket, _ := toolServing(t, true)
	native := func(inside bool, moved *NativeCall) ToolAnswer {
		t.Helper()
		var answer ToolAnswer
		request := ToolNativeRequest{LoadID: "load", Tool: "worktree_enter", Made: "made", Path: "/trees/fix", Inside: inside, Moved: moved}
		if err := json.Unmarshal([]byte(postValue(t, socket, PathToolNative, request)), &answer); err != nil {
			t.Fatal(err)
		}
		return answer
	}
	yes, gone := true, "the socket is gone"
	cases := []struct {
		answer ToolAnswer
		error  bool
		says   string
	}{
		{native(true, nil), false, "made\nThis session already works in it."},
		{native(false, &NativeCall{}), false, "made\nEntered /trees/fix"},
		{native(false, &NativeCall{Text: ptr("Switched.")}), false, "made\nSwitched."},
		{native(false, &NativeCall{Deny: ptr("policy")}), true, "Claude Code refused to enter it: policy. The worktree at /trees/fix is this worker's and stays"},
		{native(false, &NativeCall{IsError: &yes, Text: ptr("busy")}), true, "Claude Code could not enter it: busy. The worktree"},
		{native(false, &NativeCall{Error: &gone}), true, gone},
		{native(false, nil), true, "no answer"},
	}
	for i, c := range cases {
		if c.answer.IsError != c.error || !strings.Contains(c.answer.Text, c.says) {
			t.Errorf("case %d: %+v, want %v %q", i, c.answer, c.error, c.says)
		}
	}
}

// A poll naming another table is answered at once with the run's; one naming
// none gets it at the hold's end; one naming the run's gets none.
func TestAPollOfAnotherTableIsAnsweredWithTheRuns(t *testing.T) {
	_, socket, _, _ := heartbeating(t, 300*time.Millisecond)
	status, generation, _ := sayHello(t, socket, "load-1", 1)
	if status != http.StatusOK {
		t.Fatalf("the hello: %d", status)
	}
	// The endpoint names no role for its tools: a worker's table.
	digest := sessiontools.Digest(sessiontools.Offered("", false))
	alive := func(named *string) (AliveAnswer, time.Duration) {
		t.Helper()
		begun := time.Now()
		var answer AliveAnswer
		text := postValue(t, socket, PathModAlive, AliveRequest{LoadID: "load-1", Generation: generation, ToolsDigest: named})
		if err := json.Unmarshal([]byte(text), &answer); err != nil {
			t.Fatal(err)
		}
		return answer, time.Since(begun)
	}
	other := "other"
	answer, took := alive(&other)
	if answer.ToolsDigest == nil || *answer.ToolsDigest != digest || len(answer.Tools) == 0 || took > 200*time.Millisecond {
		t.Fatalf("a poll of another table: %+v after %v", answer, took)
	}
	if answer, _ := alive(nil); answer.ToolsDigest == nil || *answer.ToolsDigest != digest {
		t.Fatalf("a poll naming no table: %+v", answer)
	}
	if answer, _ := alive(&digest); answer.ToolsDigest != nil || answer.Tools != nil {
		t.Fatalf("a poll of the run's table: %+v", answer)
	}
}
