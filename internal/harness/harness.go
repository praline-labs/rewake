/*
Package harness holds what rewake knows about each coding-agent CLI it can run.

Adding a harness is meant to be one move: write a package that returns a
Harness, then add it to the slice in All. Everything else — the launch command,
the guide entry, its help page, the delivery path — is derived from that value,
so a new harness cannot be half-registered: either it is in the slice and fully
described, or it does not exist.
*/
package harness

// registered lists every harness rewake supports, in the order they appear in
// the guide. This slice is the single place a new harness is added.
var registered []Harness

// Register adds a harness to the catalog. Called from each harness package's
// init so the catalog cannot drift from what is compiled in.
func Register(h Harness) {
	registered = append(registered, h)
}

// All returns the catalog in guide order.
func All() []Harness {
	out := make([]Harness, len(registered))
	copy(out, registered)
	return out
}

// Find returns the harness with the given id.
func Find(id string) (Harness, bool) {
	for _, h := range registered {
		if h.ID() == id {
			return h, true
		}
	}
	return nil, false
}

// IDs lists the ids of every registered harness.
func IDs() []string {
	out := make([]string, 0, len(registered))
	for _, h := range registered {
		out = append(out, h.ID())
	}
	return out
}
