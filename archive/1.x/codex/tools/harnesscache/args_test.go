package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
)

// The CLI conventions of this project: no arguments is the guide, an unknown
// flag or a stray word is a refusal, and a flag another command owns is one
// too.
func TestArgumentsAreRefusedOrUnderstood(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
		command string
	}{
		{name: "nothing", args: nil, command: "help"},
		{name: "help", args: []string{"--help"}, command: "help"},
		{name: "fetch", args: []string{"fetch", "codex", "0.155.1"}, command: "fetch"},
		{name: "fetch joined flag", args: []string{"fetch", "codex", "latest", "--cache=/tmp/c"}, command: "fetch"},
		{name: "list", args: []string{"list", "--json"}, command: "list"},
		{name: "run with args", args: []string{"run", "codex", "installed", "--write", "o", "--", "--version"}, command: "run"},
		{name: "unknown command", args: []string{"get", "codex", "1.2.3"}, wantErr: true},
		{name: "unknown flag", args: []string{"fetch", "codex", "1.2.3", "--verbose"}, wantErr: true},
		{name: "stray word", args: []string{"list", "codex"}, wantErr: true},
		{name: "missing version", args: []string{"fetch", "codex"}, wantErr: true},
		{name: "unknown harness", args: []string{"fetch", "vim", "1.2.3"}, wantErr: true},
		{name: "write on fetch", args: []string{"fetch", "codex", "1.2.3", "--write", "x"}, wantErr: true},
		{name: "json on run", args: []string{"run", "codex", "1.2.3", "--json"}, wantErr: true},
		{name: "args after -- on fetch", args: []string{"fetch", "codex", "1.2.3", "--", "x"}, wantErr: true},
		{name: "remove latest", args: []string{"remove", "codex", "latest"}, wantErr: true},
		{name: "cache without value", args: []string{"list", "--cache"}, wantErr: true},
		{name: "comma in cache", args: []string{"fetch", "codex", "1.2.3", "--cache", "/tmp/a,b"}, wantErr: true},
		{name: "comma in write", args: []string{"run", "codex", "1.2.3", "--write", "/tmp/a,b"}, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted %v", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused %v: %v", tc.args, err)
			}
			if got.command != tc.command {
				t.Fatalf("command %q, want %q", got.command, tc.command)
			}
		})
	}
}

func TestARefusalExitsTwoAndPointsAtTheHelp(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := run(context.Background(), []string{"fetch", "codex"}, &stdout, &stderr); code != exitCall {
		t.Fatalf("exit %d, want %d", code, exitCall)
	}
	if !strings.Contains(stderr.String(), "full help:") {
		t.Fatalf("the refusal does not name the help: %s", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("a refusal printed on standard output: %s", stdout.String())
	}
}
