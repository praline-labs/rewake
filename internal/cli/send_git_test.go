package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func TestExplicitGitGrantRequiresMainAndEligibleTask(t *testing.T) {
	for _, scenario := range []string{"task", "question", "no flag", "text mentions Git", "writer", "general", "shell", "old main", "notify", "ineligible", "unsupported", "false flag"} {
		t.Run(scenario, func(t *testing.T) {
			senderRole := "main"
			if scenario == "writer" {
				senderRole = "write"
			}
			if scenario == "general" {
				senderRole = "general"
			}
			dir, self, peer := stateCaller(t, senderRole)
			grantingMain(t, dir, self, os.Getpid())
			peer.Role = "write"
			if scenario == "ineligible" {
				peer.Role = "general"
			}
			if scenario != "unsupported" {
				stubGrants(t, peer.Harness, true, true)
			}
			if err := registry.Update(dir, peer); err != nil {
				t.Fatal(err)
			}
			if scenario == "shell" {
				t.Setenv(state.SessionEnv, "")
			}
			if scenario == "old main" {
				t.Setenv(state.EpochEnv, "old")
			}
			args := []string{"send", peer.Name, "Commit using Git", "--wait=0", "--json"}
			requested := scenario != "no flag" && scenario != "text mentions Git"
			if requested {
				args = append(args, "--grant-git")
			}
			if scenario == "false flag" {
				args[len(args)-1] = "--grant-git=false"
			}
			if scenario == "question" {
				args = append(args, "--question")
			}
			if scenario == "notify" {
				args = append(args, "--notify")
			}
			code, out, stderr := run(args...)
			allowed := scenario == "task" || scenario == "question" || !requested
			files, _ := filepath.Glob(filepath.Join(state.InboxPath(dir, peer.Name), "*.json"))
			if !allowed {
				// The call was right and the target cannot take it: exit 1,
				// as for a directory grant; any other refusal is a wrong call.
				want := ExitUsage
				if scenario == "unsupported" {
					want = ExitFailed
				}
				if code != want || len(files) != 0 || !strings.Contains(stderr, "--grant-git") {
					t.Fatalf("unauthorized grant: %d %s %s files=%v", code, out, stderr, files)
				}
				if (scenario == "unsupported" || scenario == "ineligible") && !strings.Contains(stderr, "--write session whose harness takes") {
					t.Fatalf("refusal names no next action: %s", stderr)
				}
				return
			}
			if code != ExitPending || len(files) != 1 {
				t.Fatalf("send failed: %d %s %s", code, out, stderr)
			}
			raw, err := os.ReadFile(files[0])
			if err != nil {
				t.Fatal(err)
			}
			var m inbox.Message
			if json.Unmarshal(raw, &m) != nil || m.GrantGit != requested || m.FromEpoch != self.Epoch() {
				t.Fatal("durable explicit decision lost")
			}
			if strings.Contains(out, `"grantGit": true`) != requested {
				t.Fatal("send JSON lost explicit decision")
			}
			if requested && scenario == "task" {
				if err := state.EnsureSubdir(state.UnreadPath(dir, peer.Name)); err != nil {
					t.Fatal(err)
				}
				if err := os.Link(files[0], filepath.Join(state.UnreadPath(dir, peer.Name), m.ID+".json")); err != nil {
					t.Fatal(err)
				}
				t.Setenv(state.SessionEnv, peer.Name)
				t.Setenv(state.EpochEnv, peer.Epoch())
				code, out, stderr = run("inbox", "--message", m.ID, "--json")
				if code != ExitOK || !strings.Contains(out, `"grantGit": true`) {
					t.Fatalf("selected read lost decision: %d %s %s", code, out, stderr)
				}
				if len(inbox.Waiters(dir, peer.Name, peer.Epoch())) != 1 {
					t.Fatal("grant task lost normal causal read receipt")
				}
			}
		})
	}
}

// The help promises the take-back only for what rewake journals: the metadata
// of the session's own checkout, which --grant-git alone opens, stays.
func TestTheHelpSaysAGitGrantStays(t *testing.T) {
	_, out, _ := run("send", "--help")
	out = strings.Join(strings.Fields(out), " ")
	if !strings.Contains(out, "--grant-git alone opens the Git metadata of the session's own checkout for the rest of the thread: rewake does not journal it, so neither the report nor a later message takes it back") {
		t.Errorf("send --help does not say a standalone --grant-git stays:\n%s", out)
	}
	if strings.Contains(out, "a settled task's grant is taken back") {
		t.Errorf("send --help promises every grant back:\n%s", out)
	}
}
