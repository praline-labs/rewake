package main

import "testing"

// The CLI conventions of this project, checked rather than assumed: no
// arguments is usage rather than a guess, an unknown flag is a refusal rather
// than a silently ignored option, and a command after -- is the ordinary form.
func TestArgumentsAreRefusedOrUnderstood(t *testing.T) {
	for _, tc := range []struct {
		name    string
		args    []string
		wantErr bool
		command []string
		into    string
	}{
		{name: "nothing", args: nil, wantErr: true},
		{name: "unknown flag", args: []string{"--verbose", "--stdin"}, wantErr: true},
		{name: "into without a value", args: []string{"--into"}, wantErr: true},
		{name: "stdin alone", args: []string{"--stdin"}, into: defaultInto},
		{name: "command after --", args: []string{"--", "go", "test"}, command: []string{"go", "test"}, into: defaultInto},
		{name: "command without --", args: []string{"go", "test"}, command: []string{"go", "test"}, into: defaultInto},
		{name: "into with a value", args: []string{"--into", "/tmp/x", "--stdin"}, into: "/tmp/x"},
		{name: "into joined", args: []string{"--into=/tmp/y", "--stdin"}, into: "/tmp/y"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			opts, command, err := parseArgs(tc.args)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("accepted %v", tc.args)
				}
				return
			}
			if err != nil {
				t.Fatalf("refused %v: %v", tc.args, err)
			}
			if opts.into != tc.into {
				t.Errorf("into=%q, want %q", opts.into, tc.into)
			}
			if len(command) != len(tc.command) {
				t.Fatalf("command=%v, want %v", command, tc.command)
			}
			for i := range command {
				if command[i] != tc.command[i] {
					t.Fatalf("command=%v, want %v", command, tc.command)
				}
			}
		})
	}
}

// A stream and a command are two inputs, and taking both means one of them is
// ignored — which is how a caller ends up summarizing a stale pipe while
// believing it ran the suite.
func TestAStreamAndACommandTogetherAreRefused(t *testing.T) {
	for _, args := range [][]string{
		{"--stdin", "--", "go", "test"},
		{"--stdin", "go", "test"},
	} {
		if _, _, err := parseArgs(args); err == nil {
			t.Errorf("accepted %v", args)
		}
	}
}
