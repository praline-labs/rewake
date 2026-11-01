package mirror

// turnTo moves the turn. Any turn event ends what was being put together of
// that turn, and a turn's end leaves a block still streaming as a gap: its
// settle was lost. A compaction inside a working turn is part of it: the
// turn keeps its start and what it has in flight, and working again goes on
// with it, so its Done counts the compaction in.
func (m *Model) turnTo(e Event) {
	inTurn := m.turn == TurnStateWorking || (m.turn == TurnStateCompacting && m.since > 0)
	compacting := e.State == TurnStateCompacting && inTurn
	for id, a := range m.parts {
		if a.turn == e.Turn {
			m.forget(id)
			m.gap(m.byID[id])
		}
	}
	// A turn that stopped working leaves nothing in flight: a call the engine
	// refused before any hook saw it never gets its result.
	if e.State != TurnStateWorking && !compacting {
		for _, at := range m.order {
			if inFlight(at.item.Status) && (at.item.Turn == nil || *at.item.Turn == e.Turn) {
				m.gap(at)
			}
		}
	}
	line := Line{Op: LineOpTurn, Turn: &e.State}
	now := int(m.now().UnixMilli())
	switch {
	case e.State == TurnStateWorking:
		if !inTurn {
			m.since, m.worked = now, nil
		}
		line.Since = ptr(m.since)
		m.retry = max(e.Retry, 0)
		if m.retry > 0 {
			line.Retry = ptr(m.retry)
		}
	case compacting:
		// The turn keeps its start through the compaction.
	case e.State == TurnStateIdle && inTurn && m.since > 0:
		m.worked = &worked{took: now - m.since, at: now, stopped: e.Status == ItemStatusError}
		m.since = 0
		line.Took, line.At = ptr(m.worked.took), ptr(m.worked.at)
	default:
		m.since = 0
	}
	if e.State != TurnStateWorking {
		m.retry = 0
	}
	m.turn = e.State
	if e.State == TurnStateIdle && e.Status == ItemStatusError {
		line.Stopped = ptr(true)
	}
	m.send(line)
}
