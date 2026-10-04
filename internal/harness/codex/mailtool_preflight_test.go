package codex

import (
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
)

// The name check's app-server (docs/mail-bridge-launch-codex.md): a fake
// server answering config/read from a recorded reply, or failing in each way a
// server can, and the check's process ended either way.

// TestPreflightHelper is the fake app-server, run as the check's program.
func TestPreflightHelper(_ *testing.T) {
	mode := os.Getenv("RW_PREFLIGHT_MODE")
	if mode == "" {
		return
	}
	var socket string
	for i, arg := range os.Args {
		if arg == "--listen" && i+1 < len(os.Args) {
			socket = strings.TrimPrefix(os.Args[i+1], "unix://")
		}
	}
	cwd, _ := os.Getwd()
	mark, _ := json.Marshal(map[string]any{"args": os.Args, "cwd": cwd, "env": os.Environ(), "pid": os.Getpid()})
	_ = os.WriteFile(os.Getenv("RW_PREFLIGHT_MARK"), mark, 0o600)
	switch mode {
	case "exit":
		os.Exit(3)
	case "hang":
		time.Sleep(time.Minute)
		os.Exit(0)
	case "ignore-term":
		signal.Ignore(syscall.SIGTERM)
		time.Sleep(time.Minute)
		os.Exit(0)
	}
	listener, err := net.Listen("unix", socket)
	if err != nil {
		os.Exit(4)
	}
	reply, _ := os.ReadFile(os.Getenv("RW_PREFLIGHT_REPLY"))
	servePreflight(listener, mode, reply)
}

// servePreflight answers as an app-server would the check's questions:
// config/read with reply, or failing as mode says.
func servePreflight(listener net.Listener, mode string, reply []byte) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, rw, err := w.(http.Hijacker).Hijack()
		if err != nil {
			return
		}
		defer func() { _ = conn.Close() }()
		sum := sha1.Sum([]byte(r.Header.Get("Sec-WebSocket-Key") + "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"))
		_, _ = fmt.Fprintf(rw, "HTTP/1.1 101 Switching Protocols\r\nUpgrade: websocket\r\nConnection: Upgrade\r\nSec-WebSocket-Accept: %s\r\n\r\n", base64.StdEncoding.EncodeToString(sum[:]))
		_ = rw.Flush()
		for {
			_, data, _, err := readClientFrame(rw)
			if err != nil {
				return
			}
			var request struct {
				ID     json.RawMessage `json:"id"`
				Method string          `json:"method"`
			}
			if json.Unmarshal(data, &request) != nil || request.ID == nil {
				continue
			}
			answer := map[string]any{"id": request.ID, "result": map[string]any{}}
			switch request.Method {
			case "config/read":
				switch mode {
				case "error":
					answer = map[string]any{"id": request.ID, "error": map[string]any{"code": -32600, "message": sentinel}}
				case "garbage":
					answer["result"] = sentinel
				case "close":
					return
				case "stuck":
					continue
				default:
					answer["result"] = json.RawMessage(reply)
				}
			case "configRequirements/read":
				answer["result"] = json.RawMessage(`{"requirements":null}`)
				if mode == "requires" {
					answer["result"] = json.RawMessage(`{"requirements":{"mcpServers":{"x":{}}}}`)
				}
			}
			serverMessage(conn, answer)
		}
	})
	_ = http.Serve(listener, handler)
}

// recordedReply is config/read's answer as 0.159.0's schema shapes it, with
// a user layer that names rewake or not.
func recordedReply(named bool) string {
	servers := `{"other":{"command":"` + sentinel + `"}}`
	if named {
		servers = `{"rewake":{"command":"` + sentinel + `","env":{"K":"` + sentinel + `"}}}`
	}
	return `{"config":{"model":"x","mcp_servers":` + servers + `},"origins":{},"layers":[` +
		`{"name":{"type":"user","file":"/home/u/.codex/config.toml"},"version":"1","config":{"mcp_servers":` + servers + `}},` +
		`{"name":{"type":"sessionFlags"},"version":"1","config":{"model":"x"}}]}`
}

func TestThePreflightServerAnswersOrFails(t *testing.T) {
	previous := preflightBound
	preflightBound = 2 * time.Second
	t.Cleanup(func() { preflightBound = previous })
	bin := t.TempDir()
	program := filepath.Join(bin, "codex")
	text := "#!/bin/sh\nexec \"$RW_PREFLIGHT_EXE\" -test.run=TestPreflightHelper -- \"$@\"\n"
	if err := os.WriteFile(program, []byte(text), 0o700); err != nil {
		t.Fatal(err)
	}
	failed := func(outcome string) func(harness.ToolDecision, error) string {
		return func(_ harness.ToolDecision, err error) string {
			var check *harness.CheckFailedError
			if !errors.As(err, &check) || check.Outcome != outcome || check.Template != preflightTemplate {
				return "want the check failed: " + outcome
			}
			return ""
		}
	}
	for _, tc := range []struct {
		mode  string
		named bool
		judge func(harness.ToolDecision, error) string
	}{
		{"answer", false, func(d harness.ToolDecision, err error) string {
			if err != nil || !d.Inject {
				return "want the tool"
			}
			return ""
		}},
		{"answer", true, func(_ harness.ToolDecision, err error) string {
			var taken *harness.NameTakenError
			if !errors.As(err, &taken) || taken.Where.Scope != harness.ScopeUser || taken.Where.Path != "/home/u/.codex/config.toml" {
				return "want the name taken in the user layer"
			}
			return ""
		}},
		{"requires", false, func(d harness.ToolDecision, err error) string {
			if err != nil || d.Inject || !strings.Contains(d.Reason, "gate G4") {
				return "want no tool for gate G4"
			}
			return ""
		}},
		{"error", false, failed(harness.OutcomeUnrecognized)},
		{"garbage", false, failed(harness.OutcomeUnrecognized)},
		{"close", false, failed(harness.OutcomeBound)},
		{"stuck", false, failed(harness.OutcomeBound)},
		{"exit", false, failed(harness.OutcomeExit(3))},
		{"hang", false, failed(harness.OutcomeBound)},
		{"ignore-term", false, failed(harness.OutcomeBound)},
	} {
		t.Run(tc.mode+map[bool]string{true: " named"}[tc.named], func(t *testing.T) {
			dir := t.TempDir()
			reply, mark := filepath.Join(dir, "reply"), filepath.Join(dir, "mark")
			if err := os.WriteFile(reply, []byte(recordedReply(tc.named)), 0o600); err != nil {
				t.Fatal(err)
			}
			root, err := os.MkdirTemp("", "rwp")
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.RemoveAll(root) })
			args := []string{"-c", `model="x"`, "--config=" + `tools.web=true`}
			decision, err := codexHarness{}.CheckMailTool(harness.ToolCheckRequest{
				Args: args, Command: program, StateRoot: root,
				Assumed: []string{harness.GateG2},
				Env: []string{
					"PATH=" + os.Getenv("PATH"), "RW_PREFLIGHT_MODE=" + tc.mode, "RW_PREFLIGHT_EXE=" + os.Args[0],
					"RW_PREFLIGHT_REPLY=" + reply, "RW_PREFLIGHT_MARK=" + mark,
				},
			})
			if problem := tc.judge(decision, err); problem != "" {
				t.Fatalf("%s, got %+v %v", problem, decision, err)
			}
			if err != nil && strings.Contains(err.Error(), sentinel) {
				t.Fatalf("the diagnostic carries the answer: %v", err)
			}
			var seen struct {
				Args []string `json:"args"`
				Cwd  string   `json:"cwd"`
				Env  []string `json:"env"`
				Pid  int      `json:"pid"`
			}
			raw, readErr := os.ReadFile(mark)
			if readErr != nil || json.Unmarshal(raw, &seen) != nil {
				t.Fatalf("the server never ran: %v", readErr)
			}
			ran := strings.Join(seen.Args[indexOf(seen.Args, "--")+1:], " ")
			if !strings.HasPrefix(ran, `-c model="x" -c tools.web=true app-server --listen unix://`+root+"/chk") || strings.Contains(ran, "rewake") {
				t.Fatalf("the check's server ran as %q", ran)
			}
			if wd, _ := os.Getwd(); seen.Cwd != wd || !contains(seen.Env, remoteControlOff) {
				t.Fatalf("the check's server ran in %s with remote control on: %v", seen.Cwd, contains(seen.Env, remoteControlOff))
			}
			if syscall.Kill(seen.Pid, 0) == nil {
				t.Fatalf("the check's server %d outlived the check", seen.Pid)
			}
			if entries, _ := os.ReadDir(root); len(entries) != 0 {
				t.Fatalf("the check left %d entries in its state root", len(entries))
			}
		})
	}
}

// An open G2 leaves the tool out before any server is asked, naming the
// gate for a version the table names and the unknown version's cause
// otherwise (docs/mail-bridge-version.md).
func TestAnOpenG2OrAnUnresolvedDirectoryAsksNoServer(t *testing.T) {
	known := harness.Version{Value: "0.159.0"}
	for _, tc := range []struct {
		args    []string
		version *harness.Version
		gates   []string
		reason  string
	}{
		{nil, &known, nil, "gate G2"},
		{nil, nil, nil, "harness version unknown (not read)"},
		{nil, &harness.Version{Unknown: harness.VersionNotRead}, nil, "harness version unknown (not read)"},
		{[]string{"-C"}, nil, []string{harness.GateG2}, "working directory"},
	} {
		decision, err := codexHarness{}.CheckMailTool(harness.ToolCheckRequest{
			Args: tc.args, Command: filepath.Join(t.TempDir(), "absent"), StateRoot: t.TempDir(),
			Version: tc.version, Assumed: tc.gates,
		})
		if err != nil || decision.Inject || !strings.Contains(decision.Reason, tc.reason) {
			t.Fatalf("%q: got %+v %v, want no tool for %s", tc.args, decision, err, tc.reason)
		}
	}
}

func indexOf(values []string, want string) int {
	for index, value := range values {
		if value == want {
			return index
		}
	}
	return -1
}

func contains(values []string, want string) bool { return indexOf(values, want) >= 0 }
