package cli

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/session"
)

func TestActivityLabelsAndUnknownAreExplicit(t *testing.T) {
	now := time.Now()
	snapshot := session.Unknown("epoch")
	snapshot.Fresh = true
	if got := activityText(&snapshot); got != "unknown" {
		t.Fatal(got)
	}
	compacting := true
	for _, row := range []struct {
		activity   string
		waiting    string
		dialog     bool
		compacting bool
		want       string
	}{
		{activity: "idle", want: "idle"},
		{activity: "working", want: "working"},
		{activity: "working", waiting: session.WaitingPermission, want: "waiting for a permission"},
		{activity: "working", waiting: session.WaitingQuestion, want: "waiting for an answer"},
		{activity: "working", dialog: true, want: "waiting at a dialog"},
		{activity: "idle", dialog: true, want: "waiting at a dialog"},
		{activity: "idle", compacting: true, want: "compacting"},
		{activity: "working", compacting: true, waiting: session.WaitingQuestion, want: "waiting for an answer"},
		{activity: "not_loaded", want: "unknown"},
	} {
		snapshot.Activity = &row.activity
		snapshot.Waiting = row.waiting
		snapshot.Dialog = nil
		if row.dialog {
			message := "Claude needs your permission"
			snapshot.Dialog = &message
		}
		snapshot.Compacting = nil
		if row.compacting {
			snapshot.Compacting = &compacting
		}
		snapshot.ActivityAt = &now
		snapshot.ActivityFresh = true
		if got := activityText(&snapshot); got != row.want {
			t.Fatalf("%q != %q", got, row.want)
		}
	}
	activity := "working"
	snapshot.Activity = &activity
	snapshot.Waiting = ""
	snapshot.Compacting = nil
	// A session whose interruptions go unheard says so; one that hears them
	// adds nothing.
	snapshot.Compacting = nil
	snapshot.Interruptions = session.InterruptionsObserved
	if got := activityText(&snapshot); got != "working" {
		t.Fatal(got)
	}
	snapshot.Interruptions = session.InterruptionsUnobserved
	if got := activityText(&snapshot); got != "working; interruptions unheard" {
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
		snapshot := session.Load(dir, peer.Name, peer.Epoch())
		idle := "idle"
		snapshot.Activity = &idle
		snapshot.CompactionEvents = []session.CompactionEvent{{Sequence: 2, ObservedAt: time.Now()}}
		if err := session.Save(dir, peer.Name, peer.Epoch(), snapshot); err != nil {
			t.Fatal(err)
		}
		leaveUnread(t, dir, mail.Message{From: peer.Name, FromEpoch: peer.Epoch(), To: self.Name, ToEpoch: self.Epoch(), Kind: mail.Note, Text: "Rewake: context compacted (compaction 2).", Compaction: &mail.CompactionNotice{Count: 2, ObservedAt: time.Now()}})
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
				frozen := session.Load(dir, peer.Name, peer.Epoch())
				frozen.Epoch = "old-epoch"
				activity := "idle"
				frozen.Activity = &activity
				frozen.CompactionEvents = []session.CompactionEvent{{Sequence: 99, ObservedAt: time.Now()}}
				leaveUnread(t, dir, mail.Message{From: peer.Name, FromEpoch: "old-epoch", To: self.Name, ToEpoch: self.Epoch(), Kind: mail.Note, Text: "Session is no longer available.", Departure: &mail.DepartureNotice{Identity: mail.Availability{Name: peer.Name, CWD: "/old"}, Reason: "not registered"}, SenderState: &frozen})
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
