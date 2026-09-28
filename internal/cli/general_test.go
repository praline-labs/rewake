package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
)

func TestGeneralIsTheReportingRoleAndReadsLegacyWorkers(t *testing.T) {
	parsed, err := parse([]string{"--general", "codex"})
	if err != nil {
		t.Fatal(err)
	}
	part, err := chosenRole(parsed.Call)
	if err != nil || part.ID != "general" || part.GitWrite || part.Silent {
		t.Fatalf("role=%+v %v", part, err)
	}
	if role.Of("worker").ID != "general" {
		t.Fatal("legacy worker was not normalized")
	}
	dir := liveSession(t, "old")
	record, err := registry.Load(dir, "old")
	if err != nil {
		t.Fatal(err)
	}
	record.Role = "worker"
	if err := registry.Update(dir, record); err != nil {
		t.Fatal(err)
	}
	code, out, errOut := run("list", "--json")
	if code != 0 || !strings.Contains(out, `"role": "general"`) {
		t.Fatalf("legacy record=%d %s %s", code, out, errOut)
	}
	if _, err := parse([]string{"--worker", "codex"}); err == nil {
		t.Fatal("legacy flag should not create new worker records")
	}
}
