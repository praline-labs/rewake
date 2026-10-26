package cli

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/registry"
	"github.com/praline-labs/rewake/internal/core/role"
)

// A role this build does not know reads as general, the reporting role.
func TestGeneralIsTheReportingRoleAndAnUnknownRoleReadsAsIt(t *testing.T) {
	if part, found := role.Find("general"); !found || part.Silent {
		t.Fatalf("role=%+v", part)
	}
	if role.Of("worker").ID != "general" {
		t.Fatal("an unknown role did not read as general")
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
		t.Fatalf("a record of an unknown role=%d %s %s", code, out, errOut)
	}
}
