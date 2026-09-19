package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/inbox"
	"github.com/iiiokojiadbi/rewake/internal/sessionstate"
)

func TestActivityLabelsAndUnknownAreExplicit(t *testing.T) {
	now := time.Now()
	snapshot := sessionstate.Unknown("epoch")
	snapshot.Fresh = true
	if got := activityText(&snapshot); got != "unknown" {
		t.Fatal(got)
	}
	for _, row := range []struct {
		activity string
		waiting  []string
		want     string
	}{
		{"idle", []string{}, "idle"},
		{"working", []string{}, "working"},
		{"working", []string{"approval"}, "waiting for approval"},
		{"working", []string{"input"}, "waiting for input"},
		{"working", []string{"approval", "input"}, "waiting for approval and input"},
		{"working", nil, "working (waiting state unknown)"},
		{"system_error", []string{}, "system error"},
		{"not_loaded", []string{}, "not loaded"},
	} {
		snapshot.Activity = &row.activity
		snapshot.WaitingFor = row.waiting
		snapshot.ActivityAt = &now
		snapshot.ActivityFresh = true
		if got := activityText(&snapshot); got != row.want {
			t.Fatalf("%q != %q", got, row.want)
		}
	}
	activity := "working"
	snapshot.Activity = &activity
	snapshot.WaitingFor = []string{}
	progress := true
	snapshot.Compacting = &progress
	if got := activityText(&snapshot); got != "working; compacting" {
		t.Fatal(got)
	}
	snapshot.Stale()
	if got := activityText(&snapshot); !strings.Contains(got, "stale") {
		t.Fatal("cached activity looked fresh")
	}
}

func TestCompactionRetrievalShowsLatestActivityWithoutEventTail(t *testing.T) {
	for _, asJSON := range []bool{false, true} {
		dir, self, peer := stateCaller(t, "main")
		snapshot := sessionstate.Load(dir, peer.Name, peer.Epoch())
		idle := "idle"
		snapshot.Activity = &idle
		snapshot.CompactionEvents = []sessionstate.CompactionEvent{{Sequence: 2, ObservedAt: time.Now()}}
		if err := sessionstate.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
			t.Fatal(err)
		}
		leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, Text: "Primary compaction completed.", Compaction: &inbox.CompactionNotice{Count: 2, ObservedAt: time.Now()}})
		args := []string{"inbox"}
		if asJSON {
			args = append(args, "--json")
		}
		code, out, errOut := run(args...)
		if code != ExitOK {
			t.Fatal(code, errOut)
		}
		if strings.Contains(out, "compactionEvents") {
			t.Fatal("internal notification tail leaked into CLI state")
		}
		if asJSON {
			var model inboxModel
			if json.Unmarshal([]byte(out), &model) != nil {
				t.Fatal("bad JSON")
			}
			if *model.Messages[0].Telemetry.Activity != "idle" {
				t.Fatal("notice forced old event-time activity")
			}
		} else if !strings.HasPrefix(out, peer.Name+": idle | context") {
			t.Fatal("current retrieval activity absent")
		}
	}
}

func TestDepartureFrozenStateIsMainOnlyAndNeverUsesReplacement(t *testing.T) {
	for _, role := range []string{"main", "general", "write"} {
		for _, asJSON := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/json=%v", role, asJSON), func(t *testing.T) {
				dir, self, peer := stateCaller(t, role)
				frozen := sessionstate.Load(dir, peer.Name, peer.Epoch())
				frozen.Epoch = "old-epoch"
				activity := "idle"
				frozen.Activity = &activity
				frozen.CompactionEvents = []sessionstate.CompactionEvent{{Sequence: 99, ObservedAt: time.Now()}}
				leaveUnread(t, dir, inbox.Message{From: peer.Name, FromEpoch: "old-epoch", To: self.Name, ToEpoch: self.Epoch(), Kind: inbox.Note, Text: "Session is no longer available.", Departure: &inbox.DepartureNotice{Identity: inbox.Availability{Name: peer.Name, CWD: "/old"}, Reason: "not registered"}, SenderState: &frozen})
				args := []string{"inbox"}
				if asJSON {
					args = append(args, "--json")
				}
				code, out, errOut := run(args...)
				if code != ExitOK {
					t.Fatal(code, errOut)
				}
				if strings.Contains(out, "senderState") || strings.Contains(out, "compactionEvents") {
					t.Fatal("stored private state leaked")
				}
				if role != "main" {
					if strings.Contains(out, "telemetry") || strings.Contains(out, " | context") {
						t.Fatal("worker received telemetry")
					}
					return
				}
				if asJSON {
					var model inboxModel
					if json.Unmarshal([]byte(out), &model) != nil {
						t.Fatal("bad JSON")
					}
					got := model.Messages[0].Telemetry
					if got == nil || got.Epoch != "old-epoch" || *got.Activity != "idle" || got.Fresh || got.ActivityFresh {
						t.Fatal("departed state was replaced or shown fresh")
					}
				} else if !strings.HasPrefix(out, peer.Name+": idle (stale) | context") {
					t.Fatal("old activity not shown explicitly stale")
				}
			})
		}
	}
}
