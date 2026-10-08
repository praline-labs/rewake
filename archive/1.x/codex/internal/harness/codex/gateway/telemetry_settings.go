package gateway

import "time"

const maxSettingsProposals = 256

func (e *threadObservation) settingsUnresolved() bool {
	return len(e.proposals) > 0 || e.proposalOverflow
}

func applySettings(entry *threadObservation, model, effort []byte, now time.Time, resolveProposals bool) {
	old, current := entry.snapshot.Model, telemetryLabel(model)
	// Seeding lifecycle values does not apply an outstanding proposal. Preserve
	// any turn boundary already observed so a rejection can resume its usage.
	changed := resolveProposals && entry.settingsUnresolved() || entry.snapshot.SettingsAt != nil && (old == nil || current == nil || *old != *current)
	if changed {
		entry.awaitingNewTurn = true
		entry.snapshot.ContextUsed = nil
		entry.snapshot.ContextWindow = nil
		entry.snapshot.FilledPercent = nil
		entry.snapshot.ContextAt = nil
		entry.snapshot.ContextFresh = false
		entry.contextValid = false
	}
	entry.snapshot.Model = current
	entry.snapshot.Effort = telemetryLabel(effort)
	entry.snapshot.SettingsAt = &now
	entry.confirmedFresh = true
	if resolveProposals {
		entry.proposals = nil
		entry.proposalOverflow = false
	}
	entry.snapshot.SettingsFresh = !entry.settingsUnresolved()
}

func (c *connection) observeSettingsRequest(m meta) {
	p, ok := c.state.pending[m.id]
	if !ok {
		return
	}
	c.observations.serial++
	p.observationSerial = c.observations.serial
	p.observationGeneration = c.state.Generation
	if entry := c.observationThread(m.thread); entry != nil {
		if m.method == "thread/settings/update" {
			if !entry.settingsUnresolved() {
				entry.priorAwait = entry.awaitingNewTurn
				entry.priorTurn = entry.usageTurn
			}
			if entry.proposals == nil {
				entry.proposals = map[uint64]struct{}{}
			}
			if len(entry.proposals) < maxSettingsProposals {
				entry.proposals[p.observationSerial] = struct{}{}
			} else {
				entry.proposalOverflow = true
			}
			entry.settingsFence++
			entry.awaitingNewTurn = true
			entry.snapshot.SettingsFresh = false
			entry.snapshot.ContextFresh = false
		}
		p.observationFence = entry.settingsFence
		p.observationActivityVersion = entry.activityVersion
	}
	c.state.pending[m.id] = p
}

func rejectSettings(entry *threadObservation, p pending) {
	if _, known := entry.proposals[p.observationSerial]; !known {
		return
	}
	delete(entry.proposals, p.observationSerial)
	entry.settingsFence++
	if entry.settingsUnresolved() {
		return
	}
	// A later observed turn is already a new usage boundary. Rejection must not
	// resurrect the old turn's gate or erase another proposal's pending state.
	if entry.usageTurn == entry.priorTurn {
		entry.awaitingNewTurn = entry.priorAwait
	}
	entry.snapshot.SettingsFresh = entry.confirmedFresh
	entry.snapshot.ContextFresh = entry.contextValid && !entry.awaitingNewTurn
}

func (c *connection) observeSettingsReply(m meta, raw []byte, p pending) {
	if m.failure {
		if p.method == "thread/settings/update" {
			if entry := c.observationThread(p.target); entry != nil {
				rejectSettings(entry, p)
			}
		}
		return
	}
	if p.observationGeneration != c.state.Generation {
		return
	}
	lifecycle := (p.intent || p.fork) && p.generation == c.state.Generation && m.directKnown && m.direct
	accepted := c.state.Ready && c.state.Thread == m.thread
	candidate := p.fork && c.state.fork != nil && c.state.fork.child == m.thread
	if lifecycle && (accepted || candidate) {
		entry := c.observationEntry(m.thread)
		if entry != nil && entry.snapshot.SettingsAt == nil {
			now := observedNow(&entry.snapshot)
			applySettings(entry, field(raw, "result", "model"), field(raw, "result", "reasoningEffort"), now, false)
			entry.confirmedSerial = max(entry.confirmedSerial, p.observationSerial)
		}
		return
	}
	if p.method != "thread/read" || !p.metadataOnly || m.thread == "" || m.thread != p.target {
		return
	}
	entry := c.observationThread(m.thread)
	// Mutation/notification fences and observation request order are separate.
	// Accepting a read does not fence a newer concurrent read; a late older read
	// still cannot undo newer evidence or cross a proposal/notification boundary.
	if entry == nil || entry.settingsFence != p.observationFence || p.observationSerial <= entry.confirmedSerial {
		return
	}
	entry.confirmedSerial = p.observationSerial
	if m.status != "idle" && m.status != "active" && m.status != "systemError" {
		entry.snapshot.SettingsFresh = false
		entry.snapshot.ContextFresh = false
		entry.confirmedFresh = false
		entry.contextValid = false
		return
	}
	now := observedNow(&entry.snapshot)
	applySettings(entry, field(raw, "result", "thread", "model"), field(raw, "result", "thread", "reasoningEffort"), now, true)
}
