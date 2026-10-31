package mirror

import "slices"

// taken settles a queued prompt where the session took it, the end of the
// conversation, sent as its drop and its add again with the count of drops
// unchanged; one given back stays in place, refused. Anything else — an item
// dropped, a gap, a prompt not queued — is left as it is.
func (m *Model) taken(at *entry, back bool) {
	if at == nil || at.item.Status != ItemStatusQueued {
		return
	}
	if back {
		at.item.Status = ItemStatusRefused
		m.changed(at)
		return
	}
	if i := slices.Index(m.order, at); i >= 0 {
		m.order = append(slices.Delete(m.order, i, i+1), at)
	}
	at.item.Status = ItemStatusSettled
	m.send(Line{Op: LineOpDrop, ID: &at.item.ID, Dropped: ptr(m.dropped)})
	item := at.whole()
	m.send(Line{Op: LineOpAdd, Item: &item})
}
