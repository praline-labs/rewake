package workflow

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"time"
)

// The client half of the shim: the part the wrapper proxies, standing in for
// the terminal. It negotiates, asks for a conversation, and then reports what
// rewake says about the session it belongs to.

// shimClient is the half the wrapper proxies: it asks for a conversation the
// way the terminal does, then stays up and reports what rewake sees.
func shimClient(socket string) int {
	ws, err := wsDial(socket, time.Now().Add(10*time.Second))
	if err != nil {
		fmt.Fprintf(os.Stderr, "shim: dial %s: %v\n", socket, err)
		return 4
	}
	defer ws.close()

	// Replies arrive out of band with events, so one reader owns the socket
	// and hands replies to whoever is waiting for that id.
	replies := make(chan map[string]any, 32)
	go func() {
		for {
			raw, err := ws.readMessage()
			if err != nil {
				close(replies)
				return
			}
			var message map[string]any
			if json.Unmarshal(raw, &message) != nil {
				continue
			}
			if _, isReply := message["id"]; isReply {
				if _, isRequest := message["method"]; !isRequest {
					replies <- message
				}
			}
		}
	}()
	// A reply that never comes must not hang the client for ever: a real one
	// would give up too, and a shim that hangs cannot be asked to stop.
	call := func(id int, method string, params any, into any) error {
		if err := ws.writeJSON(map[string]any{"id": id, "method": method, "params": params}); err != nil {
			return err
		}
		timeout := time.After(5 * time.Second)
		for {
			var reply map[string]any
			select {
			case value, open := <-replies:
				if !open {
					return errors.New("connection ended before a reply")
				}
				reply = value
			case <-timeout:
				return fmt.Errorf("%s got no reply", method)
			}
			number, ok := reply["id"].(float64)
			if !ok || int(number) != id {
				continue
			}
			if failure, bad := reply["error"]; bad {
				return fmt.Errorf("%s refused: %v", method, failure)
			}
			if into != nil {
				raw, err := json.Marshal(reply["result"])
				if err != nil {
					return err
				}
				return json.Unmarshal(raw, into)
			}
			return nil
		}
	}

	// clientInfo.name and .version are both required by InitializeParams, and
	// experimentalApi has to be declared because thread/start's workspace
	// roots and the thread's canAcceptDirectInput are experimental fields.
	initialize := map[string]any{
		"clientInfo":   map[string]string{"name": "rewake-shim", "version": "0.0.1"},
		"capabilities": map[string]any{"experimentalApi": true},
	}
	if err := call(1, "initialize", initialize, nil); err != nil {
		fmt.Fprintf(os.Stderr, "shim: %v\n", err)
		return 5
	}
	if err := ws.writeJSON(map[string]any{"method": "initialized", "params": map[string]any{}}); err != nil {
		return 5
	}
	// What makes the request recognizable to the gateway: a numeric id,
	// threadSource "user", and workspace roots. The conversation id is not
	// sent — a new conversation is named by the server, and the client learns
	// it from the reply.
	method := "thread/start"
	params := map[string]any{
		"threadSource":          "user",
		"config":                map[string]any{},
		"runtimeWorkspaceRoots": []string{"/work"},
	}
	if os.Getenv(shimResume) != "" {
		method = "thread/resume"
		params["threadId"] = shimThread
	}
	var reply struct {
		Thread struct {
			ID string `json:"id"`
		} `json:"thread"`
	}
	if err := call(2, method, params, &reply); err != nil {
		// Not an exit: a client whose lifecycle request was refused or
		// unanswered stays up, and the scenario observes that rewake never
		// reported readiness. Leaving here would end the session instead, and
		// the control would be watching an absence caused by the shim.
		fmt.Fprintf(os.Stderr, "shim: %v\n", err)
	}
	// The accepted conversation is whatever came back, and the scenario
	// compares telemetry against that rather than against a guess.
	if target := os.Getenv(shimAcceptedFile); target != "" {
		_ = os.WriteFile(target, []byte(reply.Thread.ID), 0o600)
	}
	go serveRequests()
	if code := sendAsAsked(); code != 0 {
		return code
	}
	return shimReportState()
}

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
		if os.Getenv(shimExitAfterTurn) != "" && workedATurn() {
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
