package workflow

import (
	"bufio"
	"fmt"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// The Claude-shaped stand-in: what the real Claude Code adapter starts when a
// scenario puts it on PATH as `claude`. It does only what a session's
// wrapper can see of Claude Code without its mod: it answers --version, and
// it binds and listens on the messaging socket the launch names, as Claude
// Code does for its whole life, so a later image of the wrapper meets a live
// socket at that path (docs/traps.md, November 3, 2026). Each line written to
// that socket goes to the session's delivered file. It loads no mod, so no
// turn of it is ever reported: a scenario runs it as main.

// claudeStandInVersion is what --version answers: the version the adapter's
// docs were last probed on, above its minimum.
const claudeStandInVersion = "2.1.310 (Claude Code)"

// installClaudeStandIn puts the stand-in on the case's PATH as claude. Only
// the scenarios that run it install it: every other case has no claude.
func installClaudeStandIn(iso *Isolation) error {
	script := "#!/bin/sh\nexec env " + shimHarness + "=claude \"$RW_SHIM_TEST_EXE\" -test.run=TestShimHelper -- \"$@\"\n"
	return replaceScript(filepath.Join(iso.ShimDir, "claude"), script)
}

// runClaudeStandIn is the stand-in's process.
func runClaudeStandIn(args []string) int {
	args = harnessArgs(args)
	if len(args) == 1 && args[0] == "--version" {
		fmt.Println(claudeStandInVersion)
		return 0
	}
	path := ""
	for i, arg := range args {
		if arg == "--messaging-socket-path" && i+1 < len(args) {
			path = args[i+1]
		}
	}
	if path == "" {
		fmt.Fprintln(os.Stderr, "claude stand-in: no --messaging-socket-path")
		return 2
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "claude stand-in: "+err.Error())
		return 1
	}
	defer func() { _ = listener.Close() }()
	go acceptClaudeLines(listener, os.Getenv(shimDeliveredFile))
	if ready := os.Getenv(shimReadyFile); ready != "" {
		_ = os.WriteFile(ready, nil, 0o600)
	}
	ended := make(chan os.Signal, 1)
	signal.Notify(ended, syscall.SIGTERM, syscall.SIGHUP)
	exit := os.Getenv(shimExitFile)
	for {
		select {
		case <-ended:
			return 0
		case <-time.After(20 * time.Millisecond):
		}
		if _, err := os.Stat(exit); exit != "" && err == nil {
			return 0
		}
	}
}

// acceptClaudeLines writes every line any connection sends to file.
func acceptClaudeLines(listener net.Listener, file string) {
	for {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		go func() {
			defer func() { _ = conn.Close() }()
			scanner := bufio.NewScanner(conn)
			scanner.Buffer(nil, 1<<20)
			for scanner.Scan() {
				out, err := os.OpenFile(file, os.O_WRONLY|os.O_APPEND|os.O_CREATE, 0o600)
				if err != nil {
					return
				}
				_, _ = out.Write(append(scanner.Bytes(), '\n'))
				_ = out.Close()
			}
		}()
	}
}
