//go:build rewakefixture

package toolrig

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// transportFaults is the program's fault plan, read from the rig's spec as
// cmd/rewake reads REWAKE_FAULT, for the role "transport": log=<file>,
// crash=<n> and holdat=<step>@<dir>. Its steps are those of a call on the
// harness's side: observe, request, sent, answered, result.
type transportFaults struct {
	mu     sync.Mutex
	log    string
	crash  int
	holdAt string
	held   string
	steps  int
}

func parseFaults(spec string) *transportFaults {
	f := &transportFaults{}
	for _, item := range strings.Split(spec, ";") {
		who, rest, ok := strings.Cut(item, ":")
		if !ok || who != "transport" {
			continue
		}
		action, argument, _ := strings.Cut(rest, "=")
		switch action {
		case "log":
			f.log = argument
		case "crash":
			f.crash, _ = strconv.Atoi(argument)
		case "holdat":
			f.holdAt, f.held, _ = strings.Cut(argument, "@")
		}
	}
	return f
}

func (f *transportFaults) step(name string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.log != "" {
		if file, err := os.OpenFile(f.log, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600); err == nil {
			_, _ = fmt.Fprintf(file, "transport step %s\n", name)
			_ = file.Close()
		}
	}
	f.steps++
	if f.holdAt == name {
		f.holdAt = ""
		_ = os.WriteFile(filepath.Join(f.held, "held"), nil, 0o600)
		for until := time.Now().Add(time.Minute); time.Now().Before(until); time.Sleep(5 * time.Millisecond) {
			if _, err := os.Stat(filepath.Join(f.held, "go")); err == nil {
				break
			}
		}
	}
	if f.steps == f.crash {
		_ = syscall.Kill(os.Getpid(), syscall.SIGKILL)
		select {}
	}
}

// errEnded is a command whose program ended before it answered.
var errEnded = errors.New("the program ended before it answered")

// controlAsk sends one command to a program's control socket; errEnded when
// the program ended first.
func controlAsk(path string, c command) (reply, error) {
	conn, err := net.Dial("unix", path)
	if err != nil {
		return reply{}, errEnded
	}
	defer func() { _ = conn.Close() }()
	encoded, _ := json.Marshal(c)
	if _, err := conn.Write(append(encoded, '\n')); err != nil {
		return reply{}, errEnded
	}
	line, err := bufio.NewReaderSize(conn, 1<<20).ReadBytes('\n')
	if err != nil {
		return reply{}, errEnded
	}
	var answer reply
	if err := json.Unmarshal(line, &answer); err != nil {
		return reply{}, err
	}
	return answer, nil
}
