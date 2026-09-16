package role

import "testing"

func TestTheDefaultReportsItsTurns(t *testing.T) {
	if Default().Silent {
		t.Error("the default role does not report its turns, so no worker would")
	}
}

func TestTheMainSessionReportsNothing(t *testing.T) {
	found, ok := Find("main")
	if !ok || !found.Silent {
		t.Errorf("main = %+v, %v; want a role that reports nothing", found, ok)
	}
}

// A record without a role predates roles, and one with a role this build does
// not know comes from a newer one. Both are read as the default.
func TestUnknownOrMissingRolesAreTheDefault(t *testing.T) {
	for _, id := range []string{"", "reviewer-from-the-future"} {
		if got := Of(id); got.ID != Default().ID {
			t.Errorf("Of(%q) = %s, want the default", id, got.ID)
		}
	}
}

func TestEveryRoleIsDescribed(t *testing.T) {
	seen := map[string]bool{}
	for _, candidate := range All() {
		if candidate.ID == "" || candidate.Summary == "" || candidate.Brief == "" {
			t.Errorf("role %+v is not fully described", candidate)
		}
		if seen[candidate.ID] {
			t.Errorf("role %s is listed twice", candidate.ID)
		}
		seen[candidate.ID] = true
	}
}

func TestTheZeroRoleReports(t *testing.T) {
	if (Role{}).Silent {
		t.Error("a role left unset would switch reports off")
	}
}
