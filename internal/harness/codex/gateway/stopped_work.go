package gateway

// Interrupted turns are terminal, so they release live admission capacity. A
// bounded settled scope still recognizes a later explicit outcome with that ID.
type stoppedWork struct {
	binding Binding
	text    string
}

func (a *admittedWork) rememberStopped(id string, b Binding) {
	if _, ok := a.stopped[id]; ok {
		return
	}
	a.stopped[id] = stoppedWork{binding: b}
	a.stoppedOrder = append(a.stoppedOrder, id)
	if len(a.stoppedOrder) > 128 {
		old := a.stoppedOrder[0]
		a.stoppedOrder = a.stoppedOrder[1:]
		delete(a.stopped, old)
	}
}

func (a *admittedWork) stoppedEvent(m meta, raw []byte) {
	if m.turn == "" {
		return
	}
	id := m.thread + "/" + m.turn
	old, ok := a.stopped[id]
	if !ok {
		return
	}
	if m.method == "item/completed" && str(raw, "params", "item", "type") == "agentMessage" {
		v := field(raw, "params", "item", "text")
		if len(v) <= 1<<20 {
			old.text = decodeText(v)
			a.stopped[id] = old
		}
		return
	}
	if m.method != "turn/completed" {
		return
	}
	v := Completion{ID: id, Thread: m.thread, Epoch: old.binding.Epoch, Connection: old.binding.Connection, Generation: old.binding.Generation, Retained: true, ReadThrough: m.readThrough}
	switch m.status {
	case "completed":
		v.Kind = "finished"
		v.Text = old.text
	case "failed":
		v.Kind = "error"
		v.Text = decodeText(field(raw, "params", "turn", "error", "message"))
	default:
		return
	}
	a.forgetStopped(id)
	if s := a.threads[m.thread]; s != nil {
		s.completed[m.turn] = true
	}
	a.ready = append(a.ready, v)
}

func (a *admittedWork) forgetStopped(id string) {
	delete(a.stopped, id)
	for i, key := range a.stoppedOrder {
		if key == id {
			a.stoppedOrder = append(a.stoppedOrder[:i], a.stoppedOrder[i+1:]...)
			break
		}
	}
}
