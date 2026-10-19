//go:build rewaketest

package cli

import (
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// The test build's direct launch: the role flags the workflow suite starts
// its workers with.

func TestMainIsALaunchFlag(t *testing.T) {
	result, err := testTable().parse([]string{"--main", "--name", "lead", agentID, "--main"})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	part, err := chosenRole(result.Call)
	if err != nil || part.ID != role.Main.ID {
		t.Errorf("role = %v, %v; want main", part.ID, err)
	}
	// After the harness name it belongs to the harness.
	if len(result.Call.Raw) != 1 || result.Call.Raw[0] != "--main" {
		t.Errorf("raw = %q, want the harness's own --main kept", result.Call.Raw)
	}
}

func TestOmittedRoleStaysUnsetUntilDefaultResolution(t *testing.T) {
	result, err := testTable().parse([]string{agentID})
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if part, _ := chosenRole(result.Call); part.ID != "" {
		t.Errorf("role = %s, want omission preserved for the default-general reason", part.ID)
	}
}

func TestMainIsRefusedOnOtherCommands(t *testing.T) {
	code, _, errOut := run("--main", "list")
	if code != ExitUsage || !strings.Contains(errOut, "list does not take --main") {
		t.Errorf("exit = %d, stderr = %q; want a refusal", code, errOut)
	}
}

func TestWriteIsALaunchRole(t *testing.T) {
	result, err := testTable().parse([]string{"--write", aHarness(t)})
	if err != nil {
		t.Fatal(err)
	}
	part, err := chosenRole(result.Call)
	if err != nil || part.ID != "write" || part.Silent {
		t.Fatalf("role=%+v err=%v", part, err)
	}
	code, _, errOut := run("--write", "list")
	if code != ExitUsage || !strings.Contains(errOut, "list does not take --write") {
		t.Errorf("write accepted on list: %d %s", code, errOut)
	}
}

func TestAWriterCannotAlsoBeMain(t *testing.T) {
	result, err := testTable().parse([]string{"--write", "--main", aHarness(t)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := chosenRole(result.Call); err == nil {
		t.Fatal("conflicting roles were accepted")
	}
}

func TestLaunchRefusesInvalidNamePrefixesWithoutStartingHarness(t *testing.T) {
	dir := t.TempDir()
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv(state.DirEnv, dir)
	for _, h := range testCatalog().All() {
		t.Setenv("PATH", launchPath(t, h))
		for _, prefix := range []string{"", "Upper", "../escape", strings.Repeat("x", 32)} {
			code, _, errOut := run("--write", "--name", prefix, h.ID())
			if code != ExitUsage || !strings.Contains(errOut, "prefix") || !strings.Contains(errOut, "full help:") {
				t.Fatalf("prefix=%q code=%d error=%s", prefix, code, errOut)
			}
		}
		code, _, errOut := run("--name=", h.ID())
		if code != ExitUsage || !strings.Contains(errOut, "nonempty prefix") {
			t.Fatalf("empty equals flag: %d %s", code, errOut)
		}
	}
}

func TestExplicitPrefixConflictNamesFinalAddress(t *testing.T) {
	for _, h := range testCatalog().All() {
		t.Run(h.ID(), func(t *testing.T) {
			address := "taken-" + h.ID()
			liveSession(t, address)
			// The launch comes from a shell outside any session.
			outsideAnySession(t)
			t.Setenv("PATH", launchPath(t, h))
			code, _, errOut := run("--room", state.DefaultRoom, "--write", "--name", "taken", h.ID())
			if code != ExitUsage || !strings.Contains(errOut, address) || !strings.Contains(errOut, "prefix") {
				t.Fatalf("conflict=%d %s", code, errOut)
			}
		})
	}
}
