package cli

import (
	"strings"
	"testing"

	"github.com/iiiokojiadbi/rewake/internal/registry"
)

func TestSessionTableOneRowAndSharedIdentityHeaders(t *testing.T) {
	dir, self, peer := stateCaller(t, "main")
	_, out, _ := run("list")
	if strings.Count(out, "Room:") != 1 || strings.Count(out, "Directory:") != 1 || strings.Count(out, "Harness:") != 1 || strings.Count(out, self.Name) != 1 || strings.Count(out, peer.Name) != 1 {
		t.Fatal(out)
	}
	if len(strings.Split(strings.TrimSpace(out), "\n")) != 6 || strings.Contains(out, ": working") {
		t.Fatal("list repeated per-agent headers: " + out)
	}
	for _, heading := range []string{"Session", "Role", "Status", "Model", "Effort", "Context", "Compactions", "Age"} {
		if !strings.Contains(out, heading) {
			t.Fatal("missing column " + heading)
		}
	}
	peer.CWD = "/second directory\nquoted"
	peer.Harness = "different"
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	_, out, _ = run("list")
	if strings.Contains(out, "Directory:") || strings.Contains(out, "Harness:") || !strings.Contains(out, `"/second directory\nquoted"`) || len(strings.Split(strings.TrimSpace(out), "\n")) != 4 {
		t.Fatal("mixed identities lost or broke rows: " + out)
	}
}
