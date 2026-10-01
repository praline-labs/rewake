package cutover

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

func roomDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := state.RoomDir(root, state.DefaultRoom)
	if err != nil {
		t.Fatal(err)
	}
	return dir
}

func leaveRecord(t *testing.T, dir string, session registry.Session) {
	t.Helper()
	raw, _ := json.Marshal(session)
	write(t, state.SessionPath(dir, session.Name), string(raw), 0o600)
}

// The name's own records are checked by their own pids and starts: a record
// naming another namespace, or none, is out of sight; an earlier-build run
// they name still running blocks; one that has ended does not, nor a run of
// this build, which Publish judges.
func TestTheNamesRecordsAreChecked(t *testing.T) {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	here := proc.Namespace()
	for _, c := range []struct {
		name    string
		session registry.Session
		waits   string
		want    Kind
	}{
		{"another namespace", registry.Session{Name: "api", ServicePID: 4194000, ServiceStart: 7, PIDNamespace: "pid:[1]"}, "", OutOfSight},
		{"no namespace", registry.Session{Name: "api", ServicePID: 4194000, ServiceStart: 7}, "", OutOfSight},
		{"an earlier-build wrapper still runs", registry.Session{Name: "api", ServicePID: os.Getpid(), ServiceStart: start, PIDNamespace: here}, "", EarlierRun},
		{"an earlier-build run its waits name still runs", registry.Session{}, registry.RunEpoch(os.Getpid(), start, ""), EarlierRun},
		{"an earlier-build run has ended", registry.Session{Name: "api", ServicePID: 4194000, ServiceStart: 7, PIDNamespace: here}, "4194000.7", 0},
		{"a run of this build", registry.Session{Name: "api", ServicePID: os.Getpid(), ServiceStart: start, Boot: "0f0e0d0c-0b0a-4908-8706-050403020100", PIDNamespace: here}, "", 0},
	} {
		t.Run(c.name, func(t *testing.T) {
			dir := roomDir(t)
			if c.session.Name != "" {
				leaveRecord(t, dir, c.session)
			}
			if c.waits != "" {
				if err := os.MkdirAll(filepath.Join(state.AwaitingPath(dir, "api"), c.waits), 0o700); err != nil {
					t.Fatal(err)
				}
			}
			found := NameRecords(dir, "api")
			if c.want == 0 && len(found) != 0 || c.want != 0 && (len(found) != 1 || found[0].Kind != c.want) {
				t.Errorf("found %+v, want kind %d", found, c.want)
			}
		})
	}
}

// Check joins the look and the name's records into one refusal, naming the
// name.
func TestCheckRefusesNamingTheName(t *testing.T) {
	dir := roomDir(t)
	if err := Check(nil, dir, "api"); err != nil {
		t.Fatalf("a name with nothing left refused: %v", err)
	}
	err := Check([]Process{{PID: 7, Kind: UnknownExecutable, Detail: "x"}}, dir, "api")
	refusal, ok := err.(*RefusalError)
	if !ok || refusal.Name != "api" || len(refusal.Blocking) != 1 {
		t.Fatalf("refusal %v", err)
	}
}
