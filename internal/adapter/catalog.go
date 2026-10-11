/*
Package adapter is the adapter API: what an adapter offers the core, as named
capabilities, and the values they carry (docs/v2/design-api.md).

Adding a harness is meant to be one move: write a package that returns a
Harness, then add it to the constructor in adapter/catalog. Everything else —
the launch command, the guide entry, its help page, the delivery path — is
derived from the catalog value cmd/rewake builds and hands down, so a new
harness cannot be half-registered: either it is in the catalog and fully
described, or it does not exist.
*/
package adapter

import "slices"

// Protected is where harnesses keep their own metadata, which no task may
// grant a session to write (docs/grants.md): a worker that could write there
// could change its own permissions, or another session's.
type Protected struct {
	// Dirs are absolute directories: configuration, credentials and
	// sessions, under the home or wherever the harness is told to keep them.
	Dirs []string
	// Names are directories a harness keeps inside a project, matched by name
	// at any depth of a granted tree: a grant does not reach them.
	Names []string
}

// join is p with more added.
func (p Protected) join(more Protected) Protected {
	return Protected{Dirs: append(slices.Clone(p.Dirs), more.Dirs...), Names: append(slices.Clone(p.Names), more.Names...)}
}

// Catalog is the adapters one build knows, in the order the guide shows them,
// with the metadata directories of harnesses it has no adapter for. It is a
// value: cmd/rewake builds it and hands it down, and nothing holds it between
// invocations.
type Catalog struct {
	adapters []Harness
	foreign  Protected
}

// NewCatalog is the catalog of these adapters, in this order, which also
// protects foreign: where harnesses without an adapter in this build keep
// their metadata.
func NewCatalog(foreign Protected, adapters ...Harness) Catalog {
	return Catalog{adapters: slices.Clone(adapters), foreign: foreign}
}

// All returns the adapters in guide order.
func (c Catalog) All() []Harness { return slices.Clone(c.adapters) }

// Find returns the adapter with the given id.
func (c Catalog) Find(id string) (Harness, bool) {
	for _, h := range c.adapters {
		if h.ID() == id {
			return h, true
		}
	}
	return nil, false
}

// Offers is what the adapter with the given id offers, and nothing beyond
// Launch for an id the catalog does not know: a session recorded by another
// build is asked nothing it cannot be shown to do.
func (c Catalog) Offers(id string) Offer {
	if h, ok := c.Find(id); ok {
		return h.Offers()
	}
	return Offer{}
}

// IDs lists the id of every adapter, in guide order.
func (c Catalog) IDs() []string {
	out := make([]string, 0, len(c.adapters))
	for _, h := range c.adapters {
		out = append(out, h.ID())
	}
	return out
}

// Protected is where every harness keeps its metadata, those with an adapter
// and those without: a worker of one harness that could write another's
// configuration could still change what a session may do.
func (c Catalog) Protected() Protected {
	out := c.foreign
	for _, h := range c.adapters {
		out = out.join(h.ProtectedDirs())
	}
	return out
}
