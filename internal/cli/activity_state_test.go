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
