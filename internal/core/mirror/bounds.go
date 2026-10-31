package mirror

import "slices"

// The model's bounds (docs/face.md, "The model"): what it drops, and in what
// order, to stay within its items and its bytes.

// trim holds the model within its bounds, counting every byte it keeps:
// items and settles being put together alike. The oldest items no longer in
// flight go first, then the oldest settles being put together, each leaving
// its item a gap, and last the oldest items in flight, so a stream that never
// settles cannot outgrow the bounds either.
func (m *Model) trim() {
	for len(m.order)+len(m.parts) > MaxModelItems || m.bytes > MaxModelBytes {
		i := slices.IndexFunc(m.order, func(at *entry) bool { return !inFlight(at.item.Status) })
		switch {
		case i >= 0:
			m.drop(i)
		case len(m.parts) > 0:
			m.dropOldestAssembly()
		case len(m.order) > 0:
			m.drop(0)
		default:
			return
		}
	}
}

func (m *Model) drop(i int) {
	at := m.order[i]
	m.order = slices.Delete(m.order, i, i+1)
	delete(m.byID, at.item.ID)
	m.bytes -= at.size
	m.dropped++
	m.send(Line{Op: LineOpDrop, ID: &at.item.ID, Dropped: &m.dropped})
}

// dropOldestAssembly drops the settle put together longest ago; its item, if
// any, is left a gap with what streamed.
func (m *Model) dropOldestAssembly() {
	var oldest *assembly
	var id string
	for at, a := range m.parts {
		if oldest == nil || a.born < oldest.born {
			oldest, id = a, at
		}
	}
	m.forget(id)
	m.gap(m.byID[id])
}
