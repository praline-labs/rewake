package mirror

import (
	"encoding/json"
	"strconv"
	"strings"
	"testing"
)

// lines decodes what a face's queue holds.
func lines(t *testing.T, c *client) []Line {
	t.Helper()
	raw, _ := c.take()
	out := make([]Line, 0, len(raw))
	for _, line := range raw {
		var decoded Line
		if err := json.Unmarshal(line, &decoded); err != nil {
			t.Fatalf("a line does not decode: %v: %.200s", err, line)
		}
		out = append(out, decoded)
	}
	return out
}

func ops(ls []Line) string {
	words := make([]string, 0, len(ls))
	for _, l := range ls {
		words = append(words, string(l.Op))
	}
	return strings.Join(words, " ")
}

func itemOf(t *testing.T, m *Model, id string) Item {
	t.Helper()
	m.mu.Lock()
	defer m.mu.Unlock()
	at := m.byID[id]
	if at == nil {
		t.Fatalf("no item %q", id)
	}
	return at.whole()
}

// A reply streams as appends and settles whole from its parts; a delta lost
// on the way is healed by the settle.
func TestABlockStreamsThenSettlesWholeFromItsParts(t *testing.T) {
	m := New()
	c := m.subscribe()
	if got := ops(lines(t, c)); got != "hello begin end" {
		t.Fatalf("an empty model's snapshot: %s", got)
	}
	m.Apply(
		Event{Kind: EventPrompt, ID: "p1", Turn: "t1", Text: "say hi", Origin: "person"},
		Event{Kind: EventTurn, Turn: "t1", State: TurnStateWorking},
		Event{Kind: EventDelta, ID: "b1", Turn: "t1", Block: ItemKindText, Text: "Hel"},
		Event{Kind: EventDelta, ID: "b1", Turn: "t1", Block: ItemKindText, Text: "lo"},
		Event{Kind: EventSettle, ID: "b1", Turn: "t1", Block: ItemKindText, Part: 0, Of: 2, Text: "Hello, "},
		Event{Kind: EventSettle, ID: "b1", Turn: "t1", Block: ItemKindText, Part: 1, Of: 2, Text: "world"},
		Event{Kind: EventTurn, Turn: "t1", State: TurnStateIdle},
	)
	if got := ops(lines(t, c)); got != "add turn add append set turn" {
		t.Fatalf("the changes: %s", got)
	}
	if item := itemOf(t, m, "b1"); item.Text != "Hello, world" || item.Status != ItemStatusSettled {
		t.Fatalf("the settled block: %+v", item)
	}
}

// A settle whose part goes missing, a lost marker and a turn's end each leave
// what streamed as a gap, never as settled.
func TestALostSettleLeavesAGap(t *testing.T) {
	for name, tail := range map[string][]Event{
		"part out of order": {{Kind: EventSettle, ID: "b", Turn: "t", Part: 1, Of: 2, Text: "x"}},
		"lost marker":       {{Kind: EventLost, ID: "b"}},
		"turn ended":        {{Kind: EventSettle, ID: "b", Turn: "t", Part: 0, Of: 2, Text: "x"}, {Kind: EventTurn, Turn: "t", State: TurnStateIdle}},
	} {
		t.Run(name, func(t *testing.T) {
			m := New()
			m.Apply(Event{Kind: EventDelta, ID: "b", Turn: "t", Text: "streamed"})
			m.Apply(tail...)
			if item := itemOf(t, m, "b"); item.Status != ItemStatusGap || item.Text != "streamed" {
				t.Fatalf("the block: %+v", item)
			}
		})
	}
}

// A resync marks whatever was in flight, leaves the turn unknown until the
// next turn event, and says how much was lost.
func TestAResyncMarksWhatWasInFlight(t *testing.T) {
	m := New()
	m.Apply(
		Event{Kind: EventTool, ID: "call", Turn: "t", Name: "Bash"},
		Event{Kind: EventDelta, ID: "b", Turn: "t", Text: "x"},
		Event{Kind: EventResync, Lost: 7},
	)
	if itemOf(t, m, "call").Status != ItemStatusGap || itemOf(t, m, "b").Status != ItemStatusGap {
		t.Fatal("a resync left an item in flight")
	}
	if m.turn != TurnStateUnknown || !strings.HasPrefix(itemOf(t, m, "notice-1").Text, "7 events") {
		t.Fatalf("turn %s", m.turn)
	}
}

// A call whose result never comes is a gap once its turn ends.
func TestACallWithoutItsResultIsAGapAtItsTurnsEnd(t *testing.T) {
	m := New()
	m.Apply(
		Event{Kind: EventTool, ID: "call", Turn: "t", Name: "Glob"},
		Event{Kind: EventTurn, Turn: "t", State: TurnStateIdle},
	)
	if got := itemOf(t, m, "call").Status; got != ItemStatusGap {
		t.Fatalf("the call: %s", got)
	}
}

// A tool call carries its input and its end.
func TestAToolCallCarriesItsInputAndEnd(t *testing.T) {
	m := New()
	m.Apply(
		Event{Kind: EventTool, ID: "call", Turn: "t", Name: "Bash"},
		Event{Kind: EventInput, ID: "call", Text: `{"command":"ls"}`},
		Event{Kind: EventResult, ID: "call", Text: "a\nb", Status: ItemStatusError},
	)
	item := itemOf(t, m, "call")
	if item.Text != `{"command":"ls"}` || item.Output == nil || *item.Output != "a\nb" || item.Status != ItemStatusError || *item.Name != "Bash" {
		t.Fatalf("the call: %+v", item)
	}
}

// An item never passes its bound, however long its text grows, and keeps
// its head and its latest tail.
func TestAnItemStaysWithinItsBound(t *testing.T) {
	m := New()
	chunk := strings.Repeat("\"\x01é", 1000)
	m.Apply(Event{Kind: EventDelta, ID: "b", Text: "HEAD"})
	for range 400 {
		m.Apply(Event{Kind: EventDelta, ID: "b", Text: chunk})
	}
	m.Apply(Event{Kind: EventDelta, ID: "b", Text: "TAIL"})
	m.Apply(Event{Kind: EventTool, ID: "call"}, Event{Kind: EventResult, ID: "call", Text: strings.Repeat("\x02", 3*MaxFieldBytes)})
	for _, id := range []string{"b", "call"} {
		item := itemOf(t, m, id)
		line := encode(Line{Op: LineOpItem, Item: &item})
		if len(line) > MaxItemBytes || item.Cut == nil {
			t.Fatalf("%s: %d bytes, cut %v", id, len(line), item.Cut)
		}
	}
	if text := itemOf(t, m, "b").Text; !strings.HasPrefix(text, "HEAD") || !strings.HasSuffix(text, "TAIL") {
		t.Fatal("the cut lost the head or the tail")
	}
}

// The model drops its oldest settled items past either bound, counts them,
// and its snapshot stays within the bound in lines each within theirs.
func TestTheModelAndItsSnapshotStayWithinBounds(t *testing.T) {
	m := New()
	big := strings.Repeat("x", MaxItemBytes)
	for i := range 1500 {
		text := "short"
		if i%50 == 0 {
			text = big
		}
		m.Apply(Event{Kind: EventPrompt, ID: "p" + strconv.Itoa(i), Text: text})
	}
	if len(m.order) > MaxModelItems || m.bytes > MaxModelBytes || m.dropped < 500 {
		t.Fatalf("%d items, %d bytes, %d dropped", len(m.order), m.bytes, m.dropped)
	}
	for range 30 {
		m.Apply(Event{Kind: EventPrompt, ID: "big" + strconv.Itoa(m.dropped), Text: big})
	}
	c := m.subscribe()
	total := 0
	raw, _ := c.take()
	for _, line := range raw {
		if len(line) > MaxLineBytes {
			t.Fatalf("a snapshot line of %d bytes", len(line))
		}
		total += len(line)
	}
	if total > MaxModelBytes+64*1024 || m.bytes > MaxModelBytes {
		t.Fatalf("a snapshot of %d bytes, a model of %d", total, m.bytes)
	}
}

// A face that falls past its bound is cut off, and the model goes on.
func TestAFacePastItsBoundIsCutOff(t *testing.T) {
	m := New()
	c := m.subscribe()
	for i := range MaxClientOps + 10 {
		m.Apply(Event{Kind: EventPrompt, ID: strconv.Itoa(i), Text: "x"})
	}
	if _, served := c.take(); served {
		t.Fatal("a face past its bound is still served")
	}
	if len(m.order) != MaxModelItems {
		t.Fatalf("the model stopped at %d items", len(m.order))
	}
}
