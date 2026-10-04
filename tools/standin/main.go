// Command standin stands in for the model API a Claude Code harness talks to,
// so a live check can run in a scratch configuration with no login. It answers
// a turn with a call of the mail tool, which makes the harness run its
// PreToolUse hook, call the MCP tool and run PostToolUse with the result.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"
)

const usage = `standin - a stand-in for the model API, for live checks of the mail tool

The harness is pointed at it with its base-URL environment variable
(ANTHROPIC_BASE_URL) set to http://<listening address>. It serves
POST /v1/messages only (streaming and not) and /v1/messages/count_tokens.
A turn is scripted by the conversation so far: when the tool is offered it is
called -calls times, each tool result is answered with a text reply quoting
the start of the result, and a request without the tool gets a fixed text.

The first stdout line is "listening <addr>". With -log, one JSON line per
request is appended: time, path, tool names offered, what was answered. Headers
and message text are never logged. SIGINT and SIGTERM stop it.

Flags:
`

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "standin:", err)
		os.Exit(1)
	}
}

func run() error {
	fs := flag.NewFlagSet("standin", flag.ExitOnError)
	fs.Usage = func() {
		_, _ = fmt.Fprint(fs.Output(), usage)
		fs.PrintDefaults()
	}
	listen := fs.String("listen", "127.0.0.1:0", "address to listen on")
	tool := fs.String("tool", "mcp__rewake__rewake", "name of the tool to call when it is offered")
	words := fs.String("words", "whoami", "space-separated words for the tool input")
	calls := fs.Int("calls", 1, "tool calls to make in one turn before answering text")
	logPath := fs.String("log", "", "append one JSON line per request to this file")
	delay := fs.Duration("delay", 0, "delay before each answer")
	_ = fs.Parse(os.Args[1:])

	opts := Options{Tool: *tool, Words: strings.Fields(*words), Calls: *calls, Delay: *delay}
	if *logPath != "" {
		f, err := os.OpenFile(*logPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o600)
		if err != nil {
			return err
		}
		defer func() { _ = f.Close() }()
		opts.Log = f
	}

	ln, err := net.Listen("tcp", *listen)
	if err != nil {
		return err
	}
	fmt.Printf("listening %s\n", ln.Addr())

	srv := &http.Server{Handler: NewHandler(opts), ReadHeaderTimeout: 10 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shut, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if srv.Shutdown(shut) != nil {
			_ = srv.Close() // a streaming client that never reads must not hold the exit
		}
	}()
	if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
