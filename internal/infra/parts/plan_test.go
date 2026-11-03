package parts

import (
	"slices"
	"strings"
	"testing"
)

// installed is an install of the owner's kind: rewake built, the shell and the
// face run from a checkout, pi beside them.
func installed(edit func(map[string]*Part)) Manifest {
	parts := map[string]*Part{
		Rewake:    {Name: Rewake, Class: Room, Digest: "go1", File: "bin1", Speaks: map[string]string{"host": "1", "mirror": "1"}, Requires: map[string]string{"pi": "1.4.0"}},
		ShellPart: {Name: ShellPart, Class: Shell, Digest: "sh1", Requires: map[string]string{"host": "1", "face": "5"}},
		Face:      {Name: Face, Class: Live, Digest: "fa1", Speaks: map[string]string{"face": "5"}, Requires: map[string]string{"mirror": "1"}},
		"pi":      {Name: "pi", Class: Sessions, Digest: "npm:pi@1.4.0", Speaks: map[string]string{"pi": "1.4.0"}},
	}
	if edit != nil {
		edit(parts)
	}
	m := Manifest{Manifest: Format}
	for _, name := range []string{Rewake, ShellPart, Face, "pi"} {
		if p := parts[name]; p != nil {
			m.Parts = append(m.Parts, *p)
		}
	}
	return m
}

// running is what a room started on installed(nil) runs: rewake by its
// file, the shell as its hello and digest said.
func running(edit func(map[string]*Part)) []Part {
	base := installed(nil)
	r, _ := base.Find(Rewake)
	s, _ := base.Find(ShellPart)
	r.Digest = ""
	parts := map[string]*Part{Rewake: &r, ShellPart: &s}
	if edit != nil {
		edit(parts)
	}
	var out []Part
	for _, name := range []string{Rewake, ShellPart} {
		if p := parts[name]; p != nil {
			out = append(out, *p)
		}
	}
	return out
}

func TestPlan(t *testing.T) {
	cases := []struct {
		name      string
		running   []Part
		installed Manifest
		want      []Step
	}{
		{"nothing changed", running(nil), installed(nil), nil},
		{
			"the shell's sources changed", running(nil),
			installed(func(p map[string]*Part) { p[ShellPart].Digest = "sh2" }),
			[]Step{{ShellPart, Shell, "changed"}},
		},
		{
			"rewake was rebuilt", running(nil),
			installed(func(p map[string]*Part) { p[Rewake].Digest, p[Rewake].File = "go2", "bin2" }),
			[]Step{{Rewake, Room, "changed"}},
		},
		{
			"rewake was rebuilt, its contracts kept: the room switches to it", running(nil),
			installed(func(p map[string]*Part) { p[Rewake].Class, p[Rewake].Digest, p[Rewake].File = Switch, "go2", "bin2" }),
			[]Step{{Rewake, Switch, "changed"}},
		},
		{
			"rewake was rebuilt with a contract changed: a switch cannot carry it", running(nil),
			installed(func(p map[string]*Part) {
				p[Rewake].Class, p[Rewake].Digest, p[Rewake].File = Switch, "go2", "bin2"
				p[Rewake].Speaks = map[string]string{"host": "1", "mirror": "1", "handover": "2"}
			}),
			[]Step{{Rewake, Room, "changed; a switch cannot carry it: the installed rewake speaks handover 2, and the room's speaks none"}},
		},
		{
			"the face changed within its contract: it reloads itself", running(nil),
			installed(func(p map[string]*Part) { p[Face].Digest = "fa2" }), nil,
		},
		{
			"the face's contract was raised", running(nil),
			installed(func(p map[string]*Part) {
				p[Face].Digest, p[Face].Speaks = "fa2", map[string]string{"face": "6"}
				p[ShellPart].Digest, p[ShellPart].Requires = "sh2", map[string]string{"host": "1", "face": "6"}
			}),
			[]Step{{ShellPart, Shell, "changed"}},
		},
		{
			"the shell's digest matches but it loaded an older face contract",
			running(func(p map[string]*Part) { p[ShellPart].Requires = map[string]string{"host": "1", "face": "4"} }),
			installed(nil),
			[]Step{{ShellPart, Shell, "it requires face 4, and the installed face speaks 5"}},
		},
		{
			"a shell that said no contracts",
			running(func(p map[string]*Part) { p[ShellPart].Requires = nil }),
			installed(nil),
			[]Step{{ShellPart, Shell, "its contracts are unknown"}},
		},
		{
			"a new pi needs the rewake that requires it", running(nil),
			installed(func(p map[string]*Part) {
				p["pi"].Digest, p["pi"].Speaks = "npm:pi@1.5.0", map[string]string{"pi": "1.5.0"}
				p[Rewake].Requires = map[string]string{"pi": "1.5.0"}
			}),
			[]Step{{Rewake, Room, "it requires pi 1.4.0, and the installed pi speaks 1.5.0"}},
		},
		{
			"a shell of a new channel needs the room", running(nil),
			installed(func(p map[string]*Part) {
				p[ShellPart].Digest, p[ShellPart].Requires = "sh2", map[string]string{"host": "2", "face": "5"}
				p[Rewake].Speaks = map[string]string{"host": "2", "mirror": "1"}
			}),
			[]Step{{ShellPart, Shell, "changed"}, {Rewake, Room, "the installed shell requires host 2, and the running rewake speaks 1"}},
		},
		{
			"main runs plain: no shell to restart",
			running(func(p map[string]*Part) { delete(p, ShellPart) }),
			installed(func(p map[string]*Part) { p[ShellPart].Digest = "sh2" }),
			nil,
		},
		{
			"a compiled shell compared by its file, its face within it",
			[]Part{{Name: Rewake, File: "bin1", Speaks: map[string]string{"host": "1"}, Requires: map[string]string{}}, {Name: ShellPart, File: "exe1", Requires: map[string]string{"host": "1", "face": "5"}}},
			Manifest{Parts: []Part{
				{Name: Rewake, Class: Room, Digest: "go1", File: "bin1", Speaks: map[string]string{"host": "1"}},
				{Name: ShellPart, Class: Shell, Digest: "sh1", File: "exe2", Requires: map[string]string{"host": "1", "face": "5"}},
				{Name: Face, Class: Shell, Within: ShellPart, Digest: "fa2", Speaks: map[string]string{"face": "5"}},
			}},
			[]Step{{ShellPart, Shell, "changed"}},
		},
		{
			// Found in review: the shell's own sources are as they were, and
			// only the face compiled into its file changed.
			"a compiled shell whose embedded face alone changed",
			compiled("sh1", "exe1", "1"),
			release("bin1", "1", "sh1", "exe2", "fa2", "1"),
			[]Step{{ShellPart, Shell, "changed"}},
		},
		{
			"a class this rewake does not know is a room restart",
			append(running(nil), Part{Name: "plug", Digest: "p1", Requires: map[string]string{}}),
			Manifest{Parts: append(installed(nil).Parts, Part{Name: "plug", Class: "later", Digest: "p2"})},
			[]Step{{"plug", Room, "changed"}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Plan(c.running, c.installed); !slices.Equal(got, c.want) {
				t.Fatalf("Plan =\n  %v\nwant\n  %v", got, c.want)
			}
		})
	}
}

// compiled is a room running a released rewake of file bin1 and a compiled
// shell, its face within it requiring mirror.
func compiled(digest, file, mirror string) []Part {
	return []Part{
		{Name: Rewake, Class: Room, File: "bin1", Speaks: map[string]string{"host": "1", "mirror": "1"}, Requires: map[string]string{}},
		{Name: ShellPart, Class: Shell, Digest: digest, File: file, Requires: map[string]string{"host": "1", "face": "5"}},
		{Name: Face, Class: Shell, Within: ShellPart, Speaks: map[string]string{"face": "5"}, Requires: map[string]string{"mirror": mirror}},
	}
}

// release is a release's manifest: rewake speaking mirror, the shell and the
// face compiled into it requiring mirror.
func release(rewakeFile, speaks, shellDigest, shellFile, faceDigest, requires string) Manifest {
	return Manifest{Manifest: Format, Parts: []Part{
		{Name: Rewake, Class: Room, Digest: "go-" + rewakeFile, File: rewakeFile, Speaks: map[string]string{"host": "1", "mirror": speaks}},
		{Name: ShellPart, Class: Shell, Digest: shellDigest, File: shellFile, Requires: map[string]string{"host": "1", "face": "5"}},
		{Name: Face, Class: Shell, Within: ShellPart, Digest: faceDigest, Speaks: map[string]string{"face": "5"}, Requires: map[string]string{"mirror": requires}},
	}}
}

// A shell's restart is judged by the contracts of the face compiled into it
// as well (found in review): a release whose face requires mirror 2 refuses
// rewake restart shell while the room's rewake speaks 1.
func TestBlocksTheFaceWithinTheShell(t *testing.T) {
	newMirror := release("bin2", "2", "sh1", "exe2", "fa2", "2")
	if why := Blocks(compiled("sh1", "exe1", "1"), newMirror, ShellPart); why != "the installed shell requires mirror 2, and the running rewake speaks 1" {
		t.Fatalf("Blocks = %q", why)
	}
	newFace := release("bin1", "1", "sh1", "exe2", "fa2", "1")
	if why := Blocks(compiled("sh1", "exe1", "1"), newFace, ShellPart); why != "" {
		t.Fatalf("a face within the room's contracts blocks: %q", why)
	}
}

func TestBlocks(t *testing.T) {
	newChannel := installed(func(p map[string]*Part) {
		p[ShellPart].Digest, p[ShellPart].Requires = "sh2", map[string]string{"host": "2", "face": "5"}
		p[Rewake].Digest, p[Rewake].Speaks = "go2", map[string]string{"host": "2", "mirror": "1"}
	})
	if why := Blocks(running(nil), newChannel, ShellPart); why != "the installed shell requires host 2, and the running rewake speaks 1" {
		t.Fatalf("Blocks = %q", why)
	}
	newShell := installed(func(p map[string]*Part) { p[ShellPart].Digest = "sh2" })
	if why := Blocks(running(nil), newShell, ShellPart); why != "" {
		t.Fatalf("a shell within the room's contracts is blocked: %q", why)
	}
	if why := Blocks(running(nil), Manifest{}, ShellPart); why != "" {
		t.Fatalf("no manifest blocks: %q", why)
	}
}

// A shell-only release rebuilds rewake with its own version, so its file
// differs; equal sources are no room restart. A side without a digest is
// judged by the file as before.
func TestASameRewakeOfAnotherReleaseNeedsNoRestart(t *testing.T) {
	r := running(func(p map[string]*Part) { p[Rewake].Digest = "go1" })
	next := installed(func(p map[string]*Part) {
		p[Rewake].File = "bin2"
		p[ShellPart].Digest = "sh2"
	})
	if got, want := Plan(r, next), []Step{{ShellPart, Shell, "changed"}}; !slices.Equal(got, want) {
		t.Errorf("a shell-only release plans %v, want %v", got, want)
	}
	if got := Plan(running(nil), next); !slices.Contains(got, Step{Rewake, Room, "changed"}) {
		t.Errorf("a running rewake without its digest plans %v, want rewake by its file", got)
	}
}

// What a step advises is what the command does: a restart it names is of a
// part the command restarts, in place or the room's rewake by a switch.
func TestNoActionAdvisesARestartTheCommandRefuses(t *testing.T) {
	for _, class := range []string{Live, Shell, Sessions, Switch, Room} {
		for _, part := range []string{ShellPart, Face, Rewake, "pi"} {
			action := Action(class, part)
			named, advised := strings.CutPrefix(action, "rewake restart ")
			if advised && named != Rewake && !InPlace(named) {
				t.Errorf("%s of %s advises %q, which the command refuses", class, part, action)
			}
		}
	}
}
