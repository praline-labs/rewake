package codex

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/role"
)

func TestRemoteContinuationsDoNotInjectPermissions(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	for _, mode := range []string{"resume", "fork"} {
		for _, part := range []role.Role{role.General, role.Main, role.Write} {
			t.Run(mode+"/"+part.ID, func(t *testing.T) {
				args := []string{mode, "--last", "-C", repo, "--", "caller prompt"}
				plan := gitLaunch(t, part, args...)
				// Exercise a real child argv, without starting a server or a model.
				executable, err := os.Executable()
				if err != nil {
					t.Fatal(err)
				}
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()
				command := exec.CommandContext(ctx, executable, append([]string{"-test.run=^TestContinuationTUIHelper$", "--"}, plan.Args...)...)
				command.Env = append(os.Environ(), "RW_CONTINUATION_TUI=1")
				raw, err := command.Output()
				if err != nil {
					t.Fatal(err)
				}
				var actual []string
				if err := json.Unmarshal(raw, &actual); err != nil {
					t.Fatal(err)
				}
				if !slices.Equal(actual, plan.Args) {
					t.Fatalf("child args=%q plan=%q", actual, plan.Args)
				}
				if len(gitRoots(actual)) != 0 {
					t.Fatalf("forbidden --add-dir: %q", actual)
				}
				// The briefing is the only generated config override, on either child.
				for _, childArgs := range [][]string{actual, plan.Backend.(*serverSession).args} {
					config := serverConfigArgs(childArgs)
					if len(config) != 2 || !strings.HasPrefix(config[1], introKey+"=") {
						t.Fatalf("unexpected config overrides: %q", config)
					}
				}
				for _, flag := range []string{"--sandbox", "-s", "--ask-for-approval", "-a", "--approve-for-me", "--not-so-yolo", "--dangerously-bypass-approvals-and-sandbox", "--yolo"} {
					if slices.Contains(actual, flag) {
						t.Fatalf("forbidden %s: %q", flag, actual)
					}
				}
				if !slices.Equal(actual[:4], args[:4]) || !slices.Equal(actual[len(actual)-2:], args[4:]) {
					t.Fatalf("caller arguments changed: %q", actual)
				}
				warned := strings.Contains(strings.Join(plan.Notes, " "), "resumed thread gets Git metadata access with each rewake task")
				if warned {
					t.Fatalf("role=%s notes=%q", part.ID, plan.Notes)
				}
			})
		}
	}
}

func TestContinuationTUIHelper(t *testing.T) {
	if os.Getenv("RW_CONTINUATION_TUI") != "1" {
		return
	}
	offset := slices.Index(os.Args, "--")
	if offset < 0 {
		t.Fatal("missing helper delimiter")
	}
	if err := json.NewEncoder(os.Stdout).Encode(os.Args[offset+1:]); err != nil {
		t.Fatal(err)
	}
	os.Exit(0)
}

func TestContinuationLeavesExplicitPermissionsToTheCaller(t *testing.T) {
	codexHome(t, "")
	repo := gitRepository(t)
	flags := [][]string{
		{"--add-dir", repo},
		{"--add-dir=" + repo},
		{"-s", "workspace-write"},
		{"-sworkspace-write"},
		{"--sandbox=read-only"},
		{"-a", "never"},
		{"-anever"},
		{"--ask-for-approval=never"},
		{"--approve-for-me"},
		{"--not-so-yolo"},
		{"--yolo"},
		{"--dangerously-bypass-approvals-and-sandbox"},
	}
	for _, key := range []string{"approval_policy", "approvals_reviewer", "sandbox_mode", "default_permissions", "permissions.custom", "network.enabled", rootsKey, `"sandbox_workspace_write".writable_roots`} {
		value := key + " = true"
		flags = append(flags,
			[]string{"-c", value}, []string{"--config", value},
			[]string{"--config=" + value}, []string{"-c" + value}, []string{"-c=" + value},
		)
	}
	for _, mode := range []string{"resume", "fork"} {
		for _, flag := range flags {
			t.Run(mode+"/"+strings.Join(flag, " "), func(t *testing.T) {
				args := append([]string{mode, "--last", "-C", repo}, flag...)
				plan := gitLaunch(t, role.Write, args...)
				if !slices.Equal(plan.Args[:len(args)], args) {
					t.Fatalf("caller arguments changed: %q", plan.Args)
				}
				if !strings.Contains(strings.Join(plan.Notes, " "), "the remote TUI rejects them") {
					t.Fatalf("missing warning: %q", plan.Notes)
				}
			})
		}
	}
}

func TestContinuationDetectionSkipsOptionValuesAndPrompts(t *testing.T) {
	for _, test := range []struct {
		args []string
		want bool
	}{
		{[]string{"resume"}, true},
		{[]string{"fork", "--last"}, true},
		{[]string{"-m", "resume", "fork"}, true},
		{[]string{"--model=resume", "fork"}, true},
		{[]string{"-c", "model='resume'", "resume"}, true},
		{[]string{"-C", "resume"}, false},
		{[]string{"-Cfork"}, false},
		{[]string{"--add-dir", "resume"}, false},
		{[]string{"--model", "fork"}, false},
		{[]string{"--image", "resume", "fork"}, false},
		{[]string{"--", "resume"}, false},
		{[]string{"caller prompt", "resume"}, false},
		{[]string{"resume", "--", "--add-dir", "-c", "sandbox_mode='read-only'"}, true},
	} {
		got, permission := continuationOptions(test.args)
		if got != test.want {
			t.Errorf("args=%q got=%v want=%v", test.args, got, test.want)
		}
		if slices.Contains(test.args, "--") && permission {
			t.Errorf("prompt read as permissions: %q", test.args)
		}
	}
}

// The launch command takes --worktree before a launch is planned; one that
// reaches the plan anyway would start a terminal that refuses it beside
// --remote, so the plan refuses it first and names the launch that works.
func TestAWorktreeFlagReachingThePlanIsRefused(t *testing.T) {
	codexHome(t, "")
	for _, args := range [][]string{{"--worktree"}, {"--worktree=a"}} {
		_, err := New().Launch(harness.LaunchRequest{Dir: t.TempDir(), Args: args})
		if err == nil || !strings.Contains(err.Error(), "rewake codex --worktree") {
			t.Fatalf("%q reached the plan: %v", args, err)
		}
	}
}
