package parts

import (
	"fmt"
	"maps"
	"slices"
)

// Step is one part the plan restarts: its class and why.
type Step struct {
	Part  string `json:"part"`
	Class string `json:"class"`
	Why   string `json:"why"`
}

// Line is the step as rewake restart and the land tool say it.
func (s Step) Line() string { return s.Part + ": " + s.Why + " → " + Action(s.Class, s.Part) }

// Action is what a class of restart asks of the person or main.
func Action(class, part string) string {
	switch class {
	case Live:
		return "rewake restart " + part
	case Shell:
		return "rewake restart shell"
	case Sessions:
		return "new sessions take it"
	}
	return "restart the room: end main and start it again"
}

// ClassOf is a part's class, Room for one this rewake does not know.
func ClassOf(p Part) string {
	switch p.Class {
	case Live, Shell, Sessions:
		return p.Class
	}
	return Room
}

// Same says two entries of a part are one version of it: their digests
// agree where both have one, and their files where both have one, with one
// of the two to compare at least. Equal sources with another file is a
// change: a compiled shell whose embedded face alone changed keeps its own
// sources' digest.
//
// rewake is judged by its digest alone where both entries have one: a
// release rebuilds it with its version and time inside, so its file always
// differs, while equal sources are the same program; the file stays the
// build's identity for the room's lease, which this does not touch.
func Same(a, b Part) bool {
	if a.Name == Rewake && a.Digest != "" && b.Digest != "" {
		return a.Digest == b.Digest
	}
	compared := false
	if a.Digest != "" && b.Digest != "" {
		if a.Digest != b.Digest {
			return false
		}
		compared = true
	}
	if a.File != "" && b.File != "" {
		if a.File != b.File {
			return false
		}
		compared = true
	}
	return compared
}

// Plan is what makes the running parts the installed ones, in the installed
// order: a part that changed restarts by its class, and so does one whose
// contracts would not agree once the others have moved. A part not running
// has no step; nor has one compiled into another, which its file covers.
func Plan(running []Part, installed Manifest) []Step {
	w := newWorld(running, installed)
	for _, i := range w.order {
		r, ok := w.run[i.Name]
		switch {
		case !ok:
		case r.Requires == nil && len(i.Requires) > 0:
			w.move(i.Name, "its contracts are unknown")
		case !Same(r, i):
			w.move(i.Name, "changed")
		}
	}
	w.settle()
	return w.steps
}

// Blocks says why restarting part alone breaks a contract with a part that
// stays as it runs, or "" when it does not.
func Blocks(running []Part, installed Manifest, part string) string {
	w := newWorld(running, installed)
	if _, ok := w.inst[part]; !ok {
		return ""
	}
	w.move(part, "")
	for _, a := range w.present() {
		for c, v := range a.Requires {
			b, ok := w.speaker(c)
			if !ok || b.Speaks[c] == v || a.Name != part && b.Name != part {
				continue
			}
			if a.Name == part {
				return fmt.Sprintf("the installed %s requires %s %s, and the running %s speaks %s", part, c, v, b.Name, b.Speaks[c])
			}
			return fmt.Sprintf("the running %s requires %s %s, and the installed %s speaks %s", a.Name, c, v, part, b.Speaks[c])
		}
	}
	return ""
}

// world is the parts as they would run once the steps so far are taken.
type world struct {
	order []Part
	inst  map[string]Part
	run   map[string]Part
	steps []Step
}

func newWorld(running []Part, installed Manifest) *world {
	w := &world{inst: map[string]Part{}, run: map[string]Part{}}
	parts := withinFolded(installed.Parts)
	w.order = parts
	for _, p := range parts {
		w.inst[p.Name] = p
	}
	for _, p := range withinFolded(running) {
		w.run[p.Name] = p
	}
	return w
}

// withinFolded is the parts with each one compiled into another folded into
// it: it moves with its container, whose file covers it, so the container
// speaks and requires its contracts too, and a restart of the container is
// judged by them. One whose container is not listed is left out, as having
// no restart of its own.
func withinFolded(list []Part) []Part {
	var out []Part
	at := map[string]int{}
	for _, p := range list {
		if p.Within == "" {
			at[p.Name] = len(out)
			out = append(out, p)
		}
	}
	for _, p := range list {
		if i, ok := at[p.Within]; ok && p.Within != "" {
			c := out[i]
			c.Speaks, c.Requires = merged(c.Speaks, p.Speaks), merged(c.Requires, p.Requires)
			out[i] = c
		}
	}
	return out
}

// merged is a's contracts with b's added, a new map; nil when both are, so a
// part that said no contracts still says none.
func merged(a, b map[string]string) map[string]string {
	if a == nil && b == nil {
		return nil
	}
	out := make(map[string]string, len(a)+len(b))
	maps.Copy(out, a)
	maps.Copy(out, b)
	return out
}

func (w *world) moving(name string) bool {
	return slices.ContainsFunc(w.steps, func(s Step) bool { return s.Part == name })
}

func (w *world) move(name, why string) {
	if !w.moving(name) {
		w.steps = append(w.steps, Step{Part: name, Class: ClassOf(w.inst[name]), Why: why})
	}
}

// after is a part as it would run: the installed one when it moves, follows
// its files or is taken by each new session; else the running one.
func (w *world) after(name string) (Part, bool) {
	i, installed := w.inst[name]
	if installed && w.moving(name) {
		return i, true
	}
	if r, ok := w.run[name]; ok {
		return r, true
	}
	if installed && (i.Class == Live || i.Class == Sessions) {
		return i, true
	}
	return Part{}, false
}

func (w *world) present() []Part {
	var out []Part
	for _, i := range w.order {
		if p, ok := w.after(i.Name); ok {
			out = append(out, p)
		}
	}
	return out
}

func (w *world) speaker(contract string) (Part, bool) {
	for _, p := range w.present() {
		if _, ok := p.Speaks[contract]; ok {
			return p, true
		}
	}
	return Part{}, false
}

// settle moves parts until every requirer finds its contract spoken at its
// value, or no move can make it so: the requirer when its installed one
// asks what is spoken, else the speaker when its installed one speaks what
// is asked.
func (w *world) settle() {
	for range len(w.order) + 1 {
		moved := false
		for _, a := range w.present() {
			for _, c := range sortedKeys(a.Requires) {
				v := a.Requires[c]
				b, ok := w.speaker(c)
				if !ok || b.Speaks[c] == v {
					continue
				}
				switch {
				case !w.moving(a.Name) && w.canMove(a.Name) && w.inst[a.Name].Requires[c] == b.Speaks[c]:
					w.move(a.Name, fmt.Sprintf("it requires %s %s, and the installed %s speaks %s", c, v, b.Name, b.Speaks[c]))
				case !w.moving(b.Name) && w.canMove(b.Name) && w.inst[b.Name].Speaks[c] == v:
					w.move(b.Name, fmt.Sprintf("the installed %s requires %s %s, and the running %s speaks %s", a.Name, c, v, b.Name, b.Speaks[c]))
				default:
					continue
				}
				moved = true
			}
		}
		if !moved {
			return
		}
	}
}

// canMove: a part moves to its installed self only when it runs and is
// installed.
func (w *world) canMove(name string) bool {
	_, installed := w.inst[name]
	_, running := w.run[name]
	return installed && running
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return keys
}
