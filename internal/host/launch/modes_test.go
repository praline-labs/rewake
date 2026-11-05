package launch

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/core/workmode"
)

func TestModeConfigIsStrictAndMergedWithoutMergingTemplates(t *testing.T) {
	for _, bad := range []string{
		`[pi]
start_mode="plan"`,
		`[pi.modes.plan]
effort="high"`,
		`[pi.modes.review]
prompt="custom"`,
		`[pi]
unknown=true`,
		`[pi]
templateStart=true`,
		`[alias.writer.pi.modes.build]
unknown="x"`,
	} {
		s := readSet(t, bad, true)
		if s.Err() == nil {
			t.Errorf("accepted %s", bad)
		}
	}
	s := typesRoom(t, map[string]string{
		"home/.config/rewake/aliases.toml": `[pi]
start_mode="advisor"
[pi.modes.review]
model="p/m"
effort="high"
[alias.writer]
harness="agent"
[alias.writer.pi]
start_mode="review"
[alias.writer.pi.modes.review]
effort="low"`,
		"work/.rewake/aliases.toml": `[pi.modes.review]
effort="medium"
[alias.writer]
harness="agent"
[alias.writer.pi]
start_mode="build"
[alias.writer.pi.modes.build]
effort="minimal"`,
	})
	if err := s.Err(); err != nil {
		t.Fatal(err)
	}
	got := s.Modes("writer")
	if got.StartMode != "build" || got.Start("review") != "build" || !got.TemplateStart || got.Modes.Review != (workmode.Fields{Model: "p/m", Effort: "medium"}) || got.Modes.Build.Effort != "minimal" {
		t.Fatalf("merged %+v", got)
	}
	if got.Modes.Review.Effort == "low" {
		t.Fatal("replaced template leaked fields")
	}
	global := s.Modes("")
	if global.Start("review") != "review" || global.Start("code") != "build" || global.Start("") != "advisor" {
		t.Fatalf("global/type precedence: %+v", global)
	}
	if !strings.Contains(readSet(t, `[pi.modes.advisor]
effort="wrong"`, true).Err().Error(), "effort") {
		t.Fatal("missing location")
	}
}
