package mirror

import (
	"testing"
)

// orderOf is the model's items by id, in the order a snapshot sends them.
func orderOf(m *Model) []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0, len(m.order))
	for _, at := range m.order {
		ids = append(ids, at.item.ID)
	}
	return ids
}

// A prompt the session holds while it works is queued until it takes it,
// then settles where it was taken: a face sees its drop and its add again,
// the count of drops unchanged, and a snapshot has it in that place.
func TestAQueuedPromptSettlesWhereItWasTaken(t *testing.T) {
	m := New()
	c := m.subscribe()
	lines(t, c)
	m.Apply(
		Event{Kind: EventTurn, Turn: "t", State: TurnStateWorking},
		Event{Kind: EventPrompt, ID: "q", Turn: "t", Text: "also this", Origin: "face", Status: ItemStatusQueued},
		Event{Kind: EventTool, ID: "c", Turn: "t", Name: "Bash"},
		Event{Kind: EventResult, ID: "c", Turn: "t", Text: "ok", Status: ItemStatusDone},
	)
	if item := itemOf(t, m, "q"); item.Status != ItemStatusQueued {
		t.Fatalf("a prompt sent while the turn works: %+v", item)
	}
	lines(t, c)
	m.Apply(Event{Kind: EventTaken, ID: "q"})
	got := lines(t, c)
	if ops(got) != "drop add" || got[0].Dropped == nil || *got[0].Dropped != 0 || got[1].Item.Status != ItemStatusSettled {
		t.Fatalf("the take on the wire: %s %+v", ops(got), got)
	}
	if order := orderOf(m); len(order) != 2 || order[0] != "c" || order[1] != "q" {
		t.Fatalf("the order after the take: %v", order)
	}
	// A second take, of an item no longer queued, changes nothing.
	m.Apply(Event{Kind: EventTaken, ID: "q"}, Event{Kind: EventTaken, ID: "gone"})
	if got := ops(lines(t, c)); got != "" {
		t.Fatalf("a take of what is not queued: %s", got)
	}
}

// A queued prompt the session gave back stays in place, refused; one left
// at the turn's end, or across a resync, is a gap a take does not revive.
func TestAQueuedPromptGivenBackOrLostStaysSo(t *testing.T) {
	m := New()
	m.Apply(
		Event{Kind: EventTurn, Turn: "t", State: TurnStateWorking},
		Event{Kind: EventPrompt, ID: "back", Turn: "t", Text: "a", Status: ItemStatusQueued},
		Event{Kind: EventPrompt, ID: "left", Turn: "t", Text: "b", Status: ItemStatusQueued},
		Event{Kind: EventTaken, ID: "back", Status: ItemStatusRefused},
		Event{Kind: EventTurn, Turn: "t", State: TurnStateIdle},
		Event{Kind: EventTaken, ID: "left"},
	)
	if item := itemOf(t, m, "back"); item.Status != ItemStatusRefused {
		t.Fatalf("a prompt given back: %+v", item)
	}
	if item := itemOf(t, m, "left"); item.Status != ItemStatusGap {
		t.Fatalf("a prompt left at the turn's end: %+v", item)
	}
	if order := orderOf(m); order[0] != "back" || order[1] != "left" {
		t.Fatalf("nothing moved: %v", order)
	}
}
