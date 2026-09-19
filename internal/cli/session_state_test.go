package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/registry"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
	"github.com/iiiokojiadbi/rewake/internal/state"
)

func stateCaller(t *testing.T, role string) (string, registry.Session, registry.Session) {
	t.Helper()
	dir := liveSession(t, "caller")
	self, err := registry.Load(dir, "caller")
	if err != nil {
		t.Fatal(err)
	}
	self.Role = role
	if err := registry.Update(dir, self); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.SessionEnv, self.Name)
	t.Setenv(state.EpochEnv, self.Epoch())
	peer := otherRun(t, dir, "worker-native")
	saveState(t, dir, peer.Name, peer.Epoch(), 2)
	saveState(t, dir, self.Name, self.Epoch(), 3)
	return dir, self, peer
}

func saveState(t *testing.T, dir, name, epoch string, count uint64) {
	t.Helper()
	now := time.Now()
	used, window := int64(121200), int64(272123)
	filled := 42
	model, effort := "fixture-model", "high"
	activity := "working"
	snapshot := sessionstate.Snapshot{Activity: &activity, ActivityAt: &now, ActivityFresh: true, WaitingFor: []string{}, Selection: "ready", Fresh: true, ContextFresh: true, SettingsFresh: true, PublishedAt: &now, ObservedAt: &now, ContextAt: &now, SettingsAt: &now, ContextUsed: &used, ContextWindow: &window, FilledPercent: &filled, Compactions: &count, Coverage: "observed", Model: &model, Effort: &effort}
	if err := sessionstate.Save(dir, name, epoch, snapshot); err != nil {
		t.Fatal(err)
	}
}

func TestSessionStateVisibilityInListTextAndJSON(t *testing.T) {
	for _, role := range []string{"main", "write", "general"} {
		t.Run(role, func(t *testing.T) {
			_, self, _ := stateCaller(t, role)
			for _, asJSON := range []bool{false, true} {
				args := []string{"list"}
				if asJSON {
					args = append(args, "--json")
				}
				code, out, errOut := run(args...)
				if code != ExitOK {
					t.Fatalf("%d %s", code, errOut)
				}
				visible := strings.Contains(out, "context 42% used / 272K") || strings.Contains(out, `"telemetry"`)
				if visible != (role == "main") {
					t.Fatalf("role=%s output=%s", role, out)
				}
				if role == "main" && !asJSON && (!strings.Contains(out, "model \"fixture-model\"") || !strings.Contains(out, "effort \"high\"") || !strings.Contains(out, self.Name+": working | context")) {
					t.Fatal("main state or configured settings missing")
				}
				if role == "main" && asJSON && !strings.Contains(out, `"contextWindowTokens": 272123`) {
					t.Fatal("JSON rounded window")
				}
			}
			t.Setenv(state.EpochEnv, "stale-wrapper")
			_, out, _ := run("list", "--json")
			if strings.Contains(out, `"telemetry"`) {
				t.Fatal("stale epoch granted orchestrator visibility")
			}
			t.Setenv(state.SessionEnv, "")
			_, out, _ = run("list", "--json")
			if strings.Contains(out, `"telemetry"`) {
				t.Fatal("shell granted orchestrator visibility")
			}
		})
	}
}

func TestInboxStatePrecedesEveryKindAndPreservesText(t *testing.T) {
	kinds := []inbox.Kind{inbox.Task, inbox.Question, inbox.Note, inbox.Finished, inbox.Error, inbox.Stopped}
	for _, role := range []string{"main", "write", "general"} {
		for _, asJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", role, asJSON), func(t *testing.T) {
				dir, self, peer := stateCaller(t, role)
				text := "Original text\n\n" + strings.Repeat("unchanged ", 1000)
				for _, kind := range kinds {
					leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: kind, Text: text})
				}
				args := []string{"inbox"}
				if asJSON {
					args = append(args, "--json")
				}
				code, out, errOut := run(args...)
				if code != ExitOK {
					t.Fatalf("%d %s", code, errOut)
				}
				switch {
				case asJSON:
					var result struct {
						Messages []map[string]json.RawMessage `json:"messages"`
					}
					if err := json.Unmarshal([]byte(out), &result); err != nil {
						t.Fatal(err)
					}
					if len(result.Messages) != len(kinds) {
						t.Fatal("message count changed")
					}
					for _, message := range result.Messages {
						_, present := message["telemetry"]
						if present != (role == "main") {
							t.Fatal("JSON visibility wrong")
						}
						var got string
						_ = json.Unmarshal(message["text"], &got)
						if got != text {
							t.Fatal("agent text changed")
						}
					}
				case role == "main":
					header := peer.Name + ": working | context 42% used / 272K | compactions 2\n\nfrom " + peer.Name
					if strings.Count(out, header) != len(kinds) || strings.Count(out, text) != len(kinds) {
						t.Fatal("header order, blank line or text changed")
					}
				case strings.Contains(out, " | context") || strings.Count(out, text) != len(kinds):
					t.Fatal("worker received telemetry or altered text")
				}
			})
		}
	}
}

func TestSenderEpochPreventsReusedNameTelemetry(t *testing.T) {
	dir, self, peer := stateCaller(t, "main")
	saveState(t, dir, peer.Name, "old-epoch", 1)
	leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: "old-epoch", To: self.Name, ToEpoch: self.Epoch(), Text: "old sender"})
	_, out, _ := run("inbox", "--json")
	var result struct {
		Messages []messageView `json:"messages"`
	}
	if err := json.Unmarshal([]byte(out), &result); err != nil {
		t.Fatal(err)
	}
	snapshot := result.Messages[0].Telemetry
	if snapshot == nil || snapshot.Compactions == nil || *snapshot.Compactions != 1 || snapshot.Fresh {
		t.Fatal("old message acquired current session state")
	}
}

func TestDirectQuestionAnswerUsesSameVisibilityAndEpoch(t *testing.T) {
	for _, role := range []string{"main", "write", "general"} {
		for _, asJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/%v", role, asJSON), func(t *testing.T) {
				dir, self, peer := stateCaller(t, role)
				rawUnread(t, dir, self.Name, map[string]any{"from": peer.Name, "fromEpoch": peer.Epoch(), "toEpoch": self.Epoch(), "kind": "finished", "inReplyTo": []string{"question"}, "text": "Original answer"})
				var out bytes.Buffer
				ctx := &Context{Stdout: &out, JSON: asJSON}
				err := answerQuestion(ctx, sent{dir: dir, self: self, epoch: self.Epoch(), target: peer, model: sendModel{ID: "question"}, deadline: time.Now().Add(time.Second)})
				if err != nil {
					t.Fatal(err)
				}
				visible := strings.Contains(out.String(), " | context") || strings.Contains(out.String(), `"telemetry"`)
				if visible != (role == "main") || !strings.Contains(out.String(), "Original answer") {
					t.Fatal("direct answer visibility or body changed")
				}
				if role == "main" && !asJSON && !strings.HasPrefix(out.String(), peer.Name+": working | context 42% used / 272K | compactions 2\n\nanswer from ") {
					t.Fatal("direct answer metadata not first")
				}
			})
		}
	}
}

func TestAvailabilityHeaderIncludesListStateAndFrozenIdentity(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		dir, self, peer := stateCaller(t, "main")
		identity := &inbox.Availability{Name: peer.Name, Role: peer.Role, Harness: peer.Harness, Room: peer.Room, CWD: peer.CWD, Existing: true}
		leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, Text: "Session already available in this room.", Availability: identity})
		args := []string{"inbox"}
		if asJSON {
			args = append(args, "--json")
		}
		code, out, errOut := run(args...)
		if code != ExitOK {
			t.Fatalf("%d %s", code, errOut)
		}
		if asJSON {
			var model inboxModel
			if json.Unmarshal([]byte(out), &model) != nil {
				t.Fatal("bad JSON")
			}
			got := model.Messages[0]
			if got.Availability == nil || got.Availability.CWD != peer.CWD || got.Telemetry == nil || got.Telemetry.Model == nil || *got.Telemetry.Model != "fixture-model" {
				t.Fatal("availability omitted list identity/state")
			}
		} else if !strings.HasPrefix(out, peer.Name+": working | context 42% used / 272K | compactions 2 | model \"fixture-model\" | effort \"high\"\n\nfrom ") {
			t.Fatal("availability header omitted model/effort or blank line")
		}
	}
}
