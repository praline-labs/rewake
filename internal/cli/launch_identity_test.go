package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/spacehost"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// A launch is a worker only for the space host that started it, while the
// space opens or once it is open on this run: REWAKE_SPACE set by anyone
// else would start a worker past main.
func TestAWorkerIsBelievedOnlyFromItsOpeningSpacesHost(t *testing.T) {
	base := t.TempDir()
	if err := os.Chmod(base, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, base)
	dir := launched(t, base, "red")
	boot, err := registry.CurrentBoot()
	if err != nil {
		t.Fatal(err)
	}
	runOf := func(pid int) string {
		start, err := proc.StartTime(pid)
		if err != nil {
			t.Fatal(err)
		}
		return registry.RunEpoch(pid, start, boot)
	}
	parent, self := runOf(os.Getppid()), runOf(os.Getpid())
	if err := os.MkdirAll(state.SpacesPath(dir), 0o700); err != nil {
		t.Fatal(err)
	}
	ids := map[string]string{"good": strings.Repeat("a1", 16), "stranger": strings.Repeat("b2", 16), "open": strings.Repeat("c3", 16), "mine": strings.Repeat("d4", 16)}
	for name, record := range map[string][2]string{"good": {"opening", parent}, "stranger": {"opening", self}, "open": {"open", parent}, "mine": {"open", parent}} {
		text := strings.Replace(spaceRecord(ids[name], name, record[0], self, proc.Namespace(), "/work/"+name, ""), `"host": "6161.1750"`, `"host": "`+record[1]+`"`, 1)
		if name == "mine" {
			// The host recorded this run before the launch read the record.
			text = strings.Replace(text, `"run": "5151.1800"`, `"run": "`+self+`"`, 1)
		}
		if err := os.WriteFile(state.SpacePath(dir, ids[name]), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(spacehost.WorkerSpaceEnv, "red/"+ids["good"])
	id, err := identify(Call{})
	if err != nil || id.name != "good" || id.room != "red" || id.part.ID != role.Write.ID || id.directory != "" {
		t.Fatalf("the space's worker: %+v %v", id, err)
	}
	if _, set := os.LookupEnv(spacehost.WorkerSpaceEnv); set {
		t.Fatal("the variable reached the harness's environment")
	}
	t.Setenv(spacehost.WorkerSpaceEnv, "red/"+ids["mine"])
	if id, err := identify(Call{}); err != nil || id.name != "mine" {
		t.Fatalf("the space open on this run: %+v %v", id, err)
	}
	for _, value := range []string{"red/" + ids["stranger"], "red/" + ids["open"], "red", "red/" + strings.Repeat("f6", 16), "Red/" + ids["good"]} {
		t.Setenv(spacehost.WorkerSpaceEnv, value)
		if id, err := identify(Call{}); err == nil || !strings.Contains(err.Error(), "rewake space open") {
			t.Errorf("%s: %+v %v", value, id, err)
		}
	}
}
