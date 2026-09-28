package cli

import (
	"context"
	"io"
	"os"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/brief"
	"github.com/praline-labs/rewake/internal/harness"
	"github.com/praline-labs/rewake/internal/inbox"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

type roleLaunchProbe struct {
	harness.Harness
	request harness.LaunchRequest
}

func (p *roleLaunchProbe) Launch(request harness.LaunchRequest) (harness.LaunchPlan, error) {
	p.request = request
	return harness.LaunchPlan{Command: "/bin/true", Env: harness.SessionEnv(request, nil)}, nil
}

func (*roleLaunchProbe) Deliver(context.Context, registry.Session, inbox.Message) inbox.Result {
	return inbox.Result{State: inbox.Delivered}
}

func TestCLIResolvesDefaultGeneralAndPreservesExplicitCapabilities(t *testing.T) {
	for _, h := range harness.All() {
		for _, scenario := range []struct {
			name   string
			flags  []string
			want   role.Role
			prefix string
		}{
			{name: "default", want: role.General, prefix: role.General.ID},
			{name: "main prefix", flags: []string{"--name", "main"}, want: role.General, prefix: "main"},
			{name: "explicit main", flags: []string{"--main"}, want: role.Main, prefix: role.Main.ID},
			{name: "explicit general", flags: []string{"--general"}, want: role.General, prefix: role.General.ID},
			{name: "explicit write", flags: []string{"--write"}, want: role.Write, prefix: role.Write.ID},
		} {
			t.Run(h.ID()+"/"+scenario.name, func(t *testing.T) {
				dir := t.TempDir()
				if err := os.Chmod(dir, 0o700); err != nil {
					t.Fatal(err)
				}
				t.Setenv(state.DirEnv, dir)
				t.Setenv(state.RoomEnv, "isolated")
				parsed, err := parse(append(scenario.flags, h.ID()))
				if err != nil {
					t.Fatal(err)
				}
				probe := &roleLaunchProbe{Harness: h}
				if err := handleLaunch(probe)(&Context{Stdout: io.Discard, Stderr: io.Discard}, parsed.Call); err != nil {
					t.Fatal(err)
				}
				got := probe.request
				if got.Role.ID != scenario.want.ID || got.Name != scenario.prefix+"-"+h.ID() {
					t.Fatalf("wrong role, permissions or name: %+v", got)
				}
				intro := brief.Intro(got.BriefContext())
				if scenario.name == "default" || scenario.name == "main prefix" {
					for _, want := range []string{"role general", "default general", "no role flag", "end your turn", "grants no Git metadata access"} {
						if !strings.Contains(intro, want) {
							t.Fatalf("intro lacks %q: %s", want, intro)
						}
					}
					// main's own heading, not the words "main session": a
					// worker's limits name the main session it takes work from.
					if strings.Contains(intro, role.Main.Play.Heading) || strings.Contains(intro, "automatically") {
						t.Fatalf("default briefing claims main: %s", intro)
					}
				} else if !strings.Contains(intro, "selected explicitly with --"+scenario.want.ID) {
					t.Fatalf("explicit reason lost: %s", intro)
				}
			})
		}
	}
}

func TestGuideDescribesDefaultGeneralWithoutElection(t *testing.T) {
	code, out, errOut := run("guide")
	out = strings.Join(strings.Fields(out), " ")
	if code != ExitOK || !strings.Contains(out, "No role flag means general") || !strings.Contains(out, "only --main makes main") || strings.Contains(out, "elects this session main") {
		t.Fatalf("guide=%d %s %s", code, out, errOut)
	}
}
