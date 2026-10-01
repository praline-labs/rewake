package endpoint

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/bridge"
)

// The table's generated orders (docs/mail-bridge-checks.md, the order test):
// one native call heard, asked for and confirmed, against the table filling
// with other calls, the call aging past callLife, the harness reporting the
// same call again and a second server asking for it. Whatever the order, the
// call gets at most one ticket.

type tableEvent struct {
	name  string
	after []string
}

var tableEvents = []tableEvent{
	{"observe", nil},
	{"request", nil},
	{"confirm", []string{"observe", "request"}},
	{"pressure", nil},
	{"age", nil},
	{"observe again", []string{"observe"}},
	{"request again", nil},
}

func tableOrders(events []tableEvent) [][]string {
	var all [][]string
	var walk func(done []string, left []tableEvent)
	walk = func(done []string, left []tableEvent) {
		if len(left) == 0 {
			all = append(all, append([]string(nil), done...))
			return
		}
		for i, event := range left {
			ready := true
			for _, need := range event.after {
				ready = ready && strings.Contains("\x00"+strings.Join(done, "\x00")+"\x00", "\x00"+need+"\x00")
			}
			if ready {
				rest := append(append([]tableEvent(nil), left[:i]...), left[i+1:]...)
				walk(append(done, event.name), rest)
			}
		}
	}
	walk(nil, events)
	return all
}

func TestEveryOrderOfTheTableIssuesOneTicketPerCall(t *testing.T) {
	t.Parallel()
	all := tableOrders(tableEvents)
	t.Logf("%d orders generated", len(all))
	for _, order := range all {
		t.Run(strings.Join(order, ", "), func(t *testing.T) {
			t.Parallel()
			runTableOrder(t, order)
		})
	}
}

type asked struct {
	ticket bridge.Ticket
	err    error
}

func runTableOrder(t *testing.T, order []string) {
	served, path := testEndpoint(t, bridge.CodexTransport, func(c *Config) { c.Wait = 100 * time.Millisecond })
	clock := time.Now()
	served.calls.mu.Lock()
	served.calls.now = func() time.Time { return clock }
	served.calls.mu.Unlock()
	openTurn(served, "th", "t1")
	results := make(chan asked, 2)
	pending := 0
	var tickets []bridge.Ticket
	collect := func() {
		for ; pending > 0; pending-- {
			select {
			case got := <-results:
				if got.err == nil {
					tickets = append(tickets, got.ticket)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("a request never answered")
			}
		}
	}
	request := func() {
		pending++
		go func() {
			ticket, err := ticketFor(t, path, codexRequest("th", "t1", "original", words))
			results <- asked{ticket, err}
		}()
		// Until the request is answered or waits in the table, so the
		// next event comes after it.
		for deadline := time.Now().Add(2 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
			served.calls.mu.Lock()
			entry := served.calls.byCall["original"]
			waiting := entry != nil && entry.observed == nil
			served.calls.mu.Unlock()
			if waiting || len(results) > 0 {
				break
			}
		}
		if len(results) > 0 {
			collect()
		}
	}
	confirmed := 0
	for n, event := range order {
		switch event {
		case "observe", "observe again":
			served.CodexEvent(codexEvent("item/started", "th", "t1", "original", words, nil))
			collect()
		case "request", "request again":
			request()
		case "confirm":
			collect()
			for _, ticket := range tickets {
				if Confirm(path, ticket) == nil {
					confirmed++
				}
			}
		case "pressure":
			for i := range maxCalls {
				served.CodexEvent(codexEvent("item/started", "th", "t1", fmt.Sprintf("pressure-%d-%d", n, i), words, nil))
			}
		case "age":
			served.calls.mu.Lock()
			clock = clock.Add(callLife + time.Second)
			served.calls.mu.Unlock()
		}
	}
	collect()
	if len(tickets) > 1 {
		t.Fatalf("one native call got %d tickets: %s and %s", len(tickets), tickets[0].Nonce, tickets[1].Nonce)
	}
	if confirmed > 1 {
		t.Fatalf("one ticket confirmed %d times", confirmed)
	}
	// The call heard and asked for before any second report of it is
	// served, and served once.
	if first := strings.Join(order, ","); strings.HasPrefix(first, "observe,request,") || strings.HasPrefix(first, "request,observe,") {
		if len(tickets) != 1 {
			t.Fatalf("the call heard and asked for first got %d tickets", len(tickets))
		}
	}
}
