package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// What is shown (docs/mail-bridge-channel.md#what-is-shown): whoami and the
// list's Mail column carry the record's line and word, and a run that took
// gates as closed says so in both, for every record a run may hold.

func TestTheChannelAndAssumedGatesAreShown(t *testing.T) {
	now := channel.Stamp{Boot: boottime.Now(), Wall: time.Now()}
	records := []*channel.Record{
		nil,
		{Harness: channel.Claude, Tool: channel.ToolStarting},
		{Harness: channel.Claude, Tool: channel.ToolWorking},
		{Harness: channel.Codex, Tool: channel.ToolFailing, Class: channel.ClassServerGone, Interval: now},
		{Harness: channel.Claude, Tool: channel.ToolNone, Reason: "gate G7: not settled", Interval: now},
	}
	cases := 0
	for _, record := range records {
		for _, assumed := range [][]string{nil, {harness.GateG7}, {harness.GateG2, harness.GateG8}} {
			dir := liveSession(t, "lead")
			session, err := registry.Load(dir, "lead")
			if err != nil {
				t.Fatal(err)
			}
			if err := registry.Remove(dir, "lead"); err != nil {
				t.Fatal(err)
			}
			session.Role, session.AssumedGates = role.Main.ID, assumed
			if err := registry.Publish(dir, session); err != nil {
				t.Fatal(err)
			}
			t.Setenv(state.SessionEnv, "lead")
			t.Setenv(state.EpochEnv, session.Epoch())
			if err := sessionstate.Save(dir, "lead", session.Epoch(), sessionstate.Snapshot{Channel: record, PublishedBoot: boottime.Now()}); err != nil {
				t.Fatal(err)
			}
			gatesLine := ""
			if len(assumed) > 0 {
				gatesLine = "Gates taken as closed for verification: " + strings.Join(assumed, ", ") + "."
			}

			code, out, errOut := run("whoami")
			if code != 0 || !strings.Contains(out, channel.Label(record)+"\n") || strings.Contains(out, "Gates taken") != (gatesLine != "") || !strings.Contains(out, gatesLine) {
				t.Fatalf("whoami %v %v: exit %d\n%s%s", record, assumed, code, out, errOut)
			}
			code, out, _ = run("whoami", "--json")
			var model whoamiModel
			if code != 0 || json.Unmarshal([]byte(out), &model) != nil || model.Mail != channel.Label(record) || !equalSlices(model.AssumedGates, assumed) || (model.Channel == nil) != (record == nil) {
				t.Fatalf("whoami --json %v %v: exit %d\n%s", record, assumed, code, out)
			}

			code, out, errOut = run("list")
			if code != 0 || !strings.Contains(out, "Mail") || !strings.Contains(out, "  "+channel.Word(record)+"  ") || strings.Contains(out, "lead: Gates taken") != (gatesLine != "") {
				t.Fatalf("list %v %v: exit %d\n%s%s", record, assumed, code, out, errOut)
			}
			cases++
		}
	}
	t.Logf("%d displays", cases)
}

func equalSlices(a, b []string) bool {
	return strings.Join(a, "\x00") == strings.Join(b, "\x00")
}
