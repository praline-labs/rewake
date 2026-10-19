package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

func TestGeneralIsTheReportingRoleAndReadsLegacyWorkers(t *testing.T) {
	if part, found := role.Find("general"); !found || part.Silent {
		t.Fatalf("role=%+v", part)
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
	if _, err := testTable().parse([]string{"--worker", aHarness(t)}); err == nil {
		t.Fatal("legacy flag should not create new worker records")
	}
}
