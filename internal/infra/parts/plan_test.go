package parts

import (
	"slices"
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
				{Name: Face, Class: Shell, Within: ShellPart, Digest: "fa2", Speaks: map[string]string{"face": "6"}},
			}},
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
