package codex

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/harness/codex/gateway"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/role"
)

type gitReadFixture struct {
	beforeReply func()
	thread      map[string]any
	refuse      bool
	silent      bool
	change      bool
}

func gitThreadFixture(cwd string, roots []string, status string) map[string]any {
	return map[string]any{
		"id": fixtureRoot, "status": map[string]string{"type": status},
		"environments": []threadEnvironment{{ID: "local", Cwd: cwd, Roots: roots}},
	}
}

// Use the real launch role and delivery RPCs. Every recorded request is checked
// for unrelated permission fields, so a fake cannot hide a broad policy grant.
func gitDeliveryFixture(t *testing.T, part role.Role, reads ...gitReadFixture) (*serverSession, <-chan map[string]json.RawMessage) {
	t.Helper()
	return gitDeliveryFixtureWithAck(t, part, nil, reads...)
}

func gitDeliveryFixtureWithAck(t *testing.T, part role.Role, beforeAck func(), reads ...gitReadFixture) (*serverSession, <-chan map[string]json.RawMessage) {
	t.Helper()
	return gitDeliveryFixtureWithStatus(t, part, "idle", beforeAck, reads...)
}

func gitDeliveryFixtureWithStatus(t *testing.T, part role.Role, status string, beforeAck func(), reads ...gitReadFixture) (*serverSession, <-chan map[string]json.RawMessage) {
	t.Helper()
	return gitDeliveryFixtureTraffic(t, part, status, beforeAck, nil, reads...)
}

func gitDeliveryFixtureTraffic(t *testing.T, part role.Role, status string, beforeAck func(), peer func(net.Conn), reads ...gitReadFixture) (*serverSession, <-chan map[string]json.RawMessage) {
	t.Helper()
	return gitDeliveryFixtureMode(t, part, status, false, beforeAck, peer, reads...)
}

// Same-turn mode models the native atomic start-or-steer decision.
func gitDeliveryFixtureMode(t *testing.T, part role.Role, status string, sameTurn bool, beforeAck func(), peer func(net.Conn), reads ...gitReadFixture) (*serverSession, <-chan map[string]json.RawMessage) {
	t.Helper()
	codexHome(t, "")
	server := gitLaunch(t, part, "resume", "--last").Backend.(*serverSession)
	captured := make(chan map[string]json.RawMessage, 8)
	path := socketFixture(t, func(c net.Conn, r *bufio.Reader) {
		if peer != nil {
			peer(c)
		}
		readIndex, turnIndex := 0, 0
		for {
			opcode, raw, _, err := readClientFrame(r)
			if err != nil || opcode != 1 {
				return
			}
			var request struct {
				ID     json.RawMessage            `json:"id"`
				Method string                     `json:"method"`
				Params map[string]json.RawMessage `json:"params"`
			}
			if json.Unmarshal(raw, &request) != nil {
				t.Error("invalid request")
				return
			}
			var result any = map[string]any{}
			switch request.Method {
			case "initialized":
				continue
			case "initialize":
			case "thread/start":
				result = map[string]any{"thread": map[string]any{"id": fixtureRoot, "status": map[string]string{"type": status}, "canAcceptDirectInput": true}}
			case "thread/read":
				if string(request.Params["includeTurns"]) != "false" || len(request.Params) != 2 {
					t.Errorf("read requested history or unexpected fields: %s", raw)
				}
				if readIndex >= len(reads) {
					t.Error("unexpected metadata read")
					return
				}
				read := reads[readIndex]
				readIndex++
				if read.beforeReply != nil {
					read.beforeReply()
				}
				if read.silent {
					continue
				}
				if read.refuse {
					serverMessage(c, map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": "metadata unavailable"}})
					continue
				}
				if read.change {
					serverMessage(c, map[string]any{"method": "thread/closed", "params": map[string]string{"threadId": fixtureRoot}})
				}
				result = map[string]any{"thread": read.thread}
			case "turn/start":
				for key := range request.Params {
					if !slices.Contains([]string{"threadId", "clientUserMessageId", "input", "toolOutput", "runtimeWorkspaceRoots"}, key) {
						t.Errorf("unexpected turn parameter %s", key)
					}
				}
				decodedMailbox(t, request.Params)
				turnIndex++
				captured <- request.Params
				if beforeAck != nil {
					beforeAck()
				}
				turn := turnIndex
				if sameTurn {
					turn = 1
				}
				result = map[string]any{"turn": map[string]string{"id": fmt.Sprintf("delivered-turn-%d", turn)}}
			default:
				t.Errorf("unexpected RPC %s", request.Method)
			}
			serverMessage(c, map[string]any{"id": request.ID, "result": result})
		}
	})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	server.gateway = gateway.New(gateway.Config{Upstream: path, Epoch: "test"})
	downstream := filepath.Join(t.TempDir(), "gateway.sock")
	listener, err := net.Listen("unix", downstream)
	if err != nil {
		t.Fatal(err)
	}
	proxy := &http.Server{Handler: server.gateway, ReadHeaderTimeout: time.Second}
	go func() { _ = proxy.Serve(listener) }()
	t.Cleanup(func() { _ = proxy.Close(); server.gateway.Close() })
	client, err := connectRPC(ctx, downstream, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(client.close)
	if err := client.call(ctx, "thread/start", map[string]any{"threadSource": "user", "runtimeWorkspaceRoots": []string{"/work"}}, nil); err != nil {
		t.Fatal(err)
	}
	return server, captured
}

func deliverGitTask(t *testing.T, server *serverSession, captured <-chan map[string]json.RawMessage, kind inbox.Kind) ([]string, inbox.Result) {
	t.Helper()
	result := server.Deliver(context.Background(), inbox.Message{ID: "git-task", Kind: kind, GrantGit: server.gitWrite && (kind == "" || kind == inbox.Task || kind == inbox.Question), Text: "do the work"})
	if result.State != inbox.Delivered {
		t.Fatalf("delivery failed: %+v", result)
	}
	select {
	case params := <-captured:
		var roots []string
		if raw, exists := params["runtimeWorkspaceRoots"]; exists {
			if err := json.Unmarshal(raw, &roots); err != nil || roots == nil {
				t.Fatalf("invalid roots: %s", raw)
			}
		}
		return roots, result
	default:
		t.Fatal("no turn/start")
	}
	return nil, result
}

func TestTaskGitRootsPreserveExistingRootsByRole(t *testing.T) {
	for _, part := range []role.Role{role.General, role.Write, role.Main} {
		for _, kind := range []inbox.Kind{"", inbox.Task, inbox.Question, inbox.Note, inbox.Finished, inbox.Error, inbox.Stopped} {
			t.Run(part.ID+"/"+string(kind), func(t *testing.T) {
				repo := gitRepository(t)
				existing := []string{repo, t.TempDir(), t.TempDir()}
				var reads []gitReadFixture
				grant := part.GitWrite && (kind == "" || kind == inbox.Task || kind == inbox.Question)
				if grant {
					reads = append(reads, gitReadFixture{thread: gitThreadFixture(repo, existing, "idle")})
				}
				server, captured := gitDeliveryFixture(t, part, reads...)
				roots, result := deliverGitTask(t, server, captured, kind)
				if grant {
					want := append(slices.Clone(existing), filepath.Join(repo, ".git"))
					if !slices.Equal(roots, want) {
						t.Fatalf("roots=%q want=%q", roots, want)
					}
				} else if roots != nil || result.Detail != "" {
					t.Fatalf("unexpected grant: roots=%q result=%+v", roots, result)
				}
			})
		}
	}
}

func TestTaskGitRootsFollowThreadAndPreserveWorktreeMetadata(t *testing.T) {
	base := t.TempDir()
	cwd := filepath.Join(base, "checkout")
	common := filepath.Join(base, "main.git")
	private := filepath.Join(common, "worktrees", "branch")
	if err := os.MkdirAll(private, 0o700); err != nil {
		t.Fatal(err)
	}
	populateGitMetadata(t, common, true)
	populateGitMetadata(t, private, false)
	writeGitPointer(t, filepath.Join(cwd, ".git"), "gitdir: "+private+"\n")
	writeGitPointer(t, filepath.Join(private, "commondir"), "../..\n")
	existing := []string{cwd, t.TempDir(), common}
	server, captured := gitDeliveryFixture(t, role.Write, gitReadFixture{thread: gitThreadFixture(cwd, existing, "idle")})
	server.cwd = gitRepository(t) // The continued thread can belong to another repository.
	roots, _ := deliverGitTask(t, server, captured, inbox.Task)
	want := append(slices.Clone(existing), private)
	if !slices.Equal(roots, want) {
		t.Fatalf("roots=%q want=%q", roots, want)
	}
}

func TestTaskGitRootsReadAgainAfterSteerAndManualTurn(t *testing.T) {
	repo := gitRepository(t)
	gitdir := filepath.Join(repo, ".git")
	existing := []string{repo, t.TempDir()}
	granted := append(slices.Clone(existing), gitdir)
	server, captured := gitDeliveryFixture(t, role.Main,
		gitReadFixture{thread: gitThreadFixture(repo, existing, "active")},
		gitReadFixture{thread: gitThreadFixture(repo, granted, "idle")},
		gitReadFixture{thread: gitThreadFixture(repo, existing, "idle")},
	)
	roots, result := deliverGitTask(t, server, captured, inbox.Task)
	if !slices.Equal(roots, granted) || !strings.Contains(result.Detail, "subsequent turns") {
		t.Fatalf("steer roots=%q result=%+v", roots, result)
	}
	roots, result = deliverGitTask(t, server, captured, inbox.Task)
	if roots != nil || result.Detail != "" {
		t.Fatalf("repeated existing grant: roots=%q result=%+v", roots, result)
	}
	roots, _ = deliverGitTask(t, server, captured, inbox.Task)
	if !slices.Equal(roots, granted) {
		t.Fatalf("did not restore roots after manual turn: %q", roots)
	}
}

func TestTaskGitRootsUnavailableDoesNotBlockDelivery(t *testing.T) {
	for _, name := range []string{"read error", "read timeout", "absent environments", "empty environments", "remote environment", "multiple environments", "absent roots", "relative root", "relative cwd", "invalid metadata", "wrong thread"} {
		t.Run(name, func(t *testing.T) {
			repo := gitRepository(t)
			thread := gitThreadFixture(repo, []string{repo, t.TempDir()}, "idle")
			read := gitReadFixture{thread: thread}
			switch name {
			case "read error":
				read.refuse = true
			case "read timeout":
				read.silent = true
			case "absent environments":
				delete(thread, "environments")
			case "empty environments":
				thread["environments"] = []threadEnvironment{}
			case "remote environment":
				thread["environments"] = []threadEnvironment{{ID: "remote", Cwd: repo, Roots: []string{repo}}}
			case "multiple environments":
				thread["environments"] = []threadEnvironment{{ID: "local", Cwd: repo, Roots: []string{repo}}, {ID: "remote", Cwd: repo, Roots: []string{repo}}}
			case "absent roots":
				thread["environments"] = []threadEnvironment{{ID: "local", Cwd: repo}}
			case "relative root":
				thread["environments"] = []threadEnvironment{{ID: "local", Cwd: repo, Roots: []string{"relative"}}}
			case "relative cwd":
				thread["environments"] = []threadEnvironment{{ID: "local", Cwd: "relative", Roots: []string{repo}}}
			case "invalid metadata":
				thread = gitThreadFixture(t.TempDir(), []string{repo}, "idle")
				read.thread = thread
			case "wrong thread":
				thread["id"] = "another-thread"
			}
			server, captured := gitDeliveryFixture(t, role.Write, read)
			roots, result := deliverGitTask(t, server, captured, inbox.Task)
			if roots != nil || !strings.HasPrefix(result.Detail, "Git metadata access unchanged:") || strings.ContainsAny(result.Detail, "\r\n") {
				t.Fatalf("unsafe fallback: roots=%q result=%+v", roots, result)
			}
		})
	}
}

func TestTaskGitRootsRefuseClosedThreadDuringRead(t *testing.T) {
	repo := gitRepository(t)
	server, captured := gitDeliveryFixture(t, role.Write, gitReadFixture{thread: gitThreadFixture(repo, []string{repo}, "idle"), change: true})
	result := server.Deliver(context.Background(), inbox.Message{ID: "changed", Kind: inbox.Task, GrantGit: true})
	if result.State != inbox.Failed || !strings.Contains(result.Detail, "accepted conversation") {
		t.Fatalf("result=%+v", result)
	}
	select {
	case params := <-captured:
		t.Fatalf("delivered to obsolete thread: %v", params)
	default:
	}
}
