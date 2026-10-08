package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/boottime"
	"github.com/praline-labs/rewake/internal/channel"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/sessionstate"
	"github.com/praline-labs/rewake/internal/state"
)

// What is shown (docs/mail-bridge-channel.md#what-is-shown): whoami and the
// list's Mail column carry the record's line and word, for every record a run
// may hold.

func TestTheChannelIsShown(t *testing.T) {
	now := channel.Stamp{Boot: boottime.Now(), Wall: time.Now()}
	records := []*channel.Record{
		nil,
		{Tool: channel.ToolStarting},
		{Tool: channel.ToolWorking},
		{Tool: channel.ToolFailing, Class: channel.ClassServerGone, Interval: now},
		{Tool: channel.ToolNone, Reason: "the endpoint could not start", Interval: now},
	}
	for _, record := range records {
		dir := liveSession(t, "lead")
		session, err := registry.Load(dir, "lead")
		if err != nil {
			t.Fatal(err)
		}
		if err := registry.Remove(dir, "lead"); err != nil {
			t.Fatal(err)
		}
		session.Role = role.Main.ID
		if err := registry.Publish(dir, session); err != nil {
			t.Fatal(err)
		}
		t.Setenv(state.SessionEnv, "lead")
		t.Setenv(state.EpochEnv, session.Epoch())
		if err := sessionstate.Save(dir, "lead", session.Epoch(), sessionstate.Snapshot{Channel: record, PublishedBoot: boottime.Now()}); err != nil {
			t.Fatal(err)
		}

		code, out, errOut := run("whoami")
		if code != 0 || !strings.Contains(out, channel.Label(record)+"\n") {
			t.Fatalf("whoami %v: exit %d\n%s%s", record, code, out, errOut)
		}
		code, out, _ = run("whoami", "--json")
		var model whoamiModel
		if code != 0 || json.Unmarshal([]byte(out), &model) != nil || model.Mail != channel.Label(record) || (model.Channel == nil) != (record == nil) {
			t.Fatalf("whoami --json %v: exit %d\n%s", record, code, out)
		}

		code, out, errOut = run("list")
		if code != 0 || !strings.Contains(out, "Mail") || !strings.Contains(out, "  "+channel.Word(record)+"  ") {
			t.Fatalf("list %v: exit %d\n%s%s", record, code, out, errOut)
		}
	}
}
