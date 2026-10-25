package ask

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

var permission = Offer{Kind: Permission, Tool: "Bash", Input: `{"command":"make"}`, Questions: []Prompt{{
	Text: "Allow Bash?", Options: []Option{{Label: "Allow once"}, {Label: "Allow for this session"}, {Label: "Refuse"}},
}}}

var question = Offer{Kind: Question, Questions: []Prompt{{
	Text: "Which?", Multi: true, Options: []Option{{Label: "Alpha"}, {Label: "Beta"}, {Label: "Gamma"}},
}}}

func TestMainsWordsAreReadAgainstTheOffer(t *testing.T) {
	for _, c := range []struct {
		offer  Offer
		words  []string
		answer string
		reason string
		fails  string
	}{
		{permission, []string{"once"}, "Allow once", "", ""},
		{permission, []string{"Session"}, "Allow for this session", "", ""},
		{permission, []string{"refuse", "not", "in", "prod"}, "Refuse", "not in prod", ""},
		{permission, []string{"once", "please"}, "", "", "only refuse takes a reason"},
		{permission, []string{"yes"}, "", "", "not a choice"},
		{question, []string{"2"}, "Beta", "", ""},
		{question, []string{"3,1,3"}, "Gamma, Alpha", "", ""},
		{question, []string{"something", "else"}, "something else", "", ""},
		{question, []string{"4"}, "", "", "not an option"},
		{question, nil, "", "", "no choice"},
	} {
		got, err := Parse(c.offer, c.words)
		if c.fails != "" {
			if err == nil || !strings.Contains(err.Error(), c.fails) {
				t.Errorf("%v: want a refusal with %q, got %+v, %v", c.words, c.fails, got, err)
			}
			continue
		}
		if err != nil || got.Answers[c.offer.Questions[0].Text] != c.answer || got.Reason != c.reason {
			t.Errorf("%v: got %+v, %v; want %q, %q", c.words, got, err, c.answer, c.reason)
		}
	}
	single := Offer{Kind: Question, Questions: []Prompt{{Text: "One?", Options: []Option{{Label: "A"}, {Label: "B"}}}}}
	if _, err := Parse(single, []string{"1,2"}); err == nil {
		t.Error("a single-choice question took two options")
	}
	two := Offer{Kind: Question, Questions: []Prompt{single.Questions[0], {Text: "Two?", Options: []Option{{Label: "C"}}}}}
	if got, err := Parse(two, []string{"2", "free text"}); err != nil || got.Answers["One?"] != "B" || got.Answers["Two?"] != "free text" {
		t.Errorf("two questions: %+v, %v", got, err)
	}
	if _, err := Parse(two, []string{"2"}); err == nil {
		t.Error("two questions took one answer")
	}
}

// Main's answer goes to the poll waiting for it, and main hears that the
// harness took it once the request is settled with it.
func TestMainsAnswerReachesThePollAndHearsItWasTaken(t *testing.T) {
	broker := NewBroker()
	id, _ := broker.Open("load-1", question)
	var poll Poll
	var pollErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		poll, pollErr = broker.Poll(context.Background(), "load-1", id, 5*time.Second)
	}()
	outcome, err := broker.Propose(id, Answer{Answers: map[string]string{"Which?": "Beta"}})
	if err != nil {
		t.Fatal(err)
	}
	wg.Wait()
	if pollErr != nil || poll.Answer == nil || poll.Answer.Answers["Which?"] != "Beta" {
		t.Fatalf("the poll got %+v, %v", poll, pollErr)
	}
	if _, err := broker.Propose(id, Answer{}); !errors.Is(err, ErrAnswered) {
		t.Fatalf("a second answer: %v", err)
	}
	if err := broker.Settle("load-1", id, Main); err != nil {
		t.Fatal(err)
	}
	if got := <-outcome; !got.Accepted {
		t.Fatalf("main's answer was not taken: %+v", got)
	}
}

// The person at the worker's screen answering first settles the request:
// a poll waiting on it wakes, main's answer after it is refused, and main's
// answer made before the harness saw it hears who won.
func TestTheFirstAnswerWins(t *testing.T) {
	broker := NewBroker()
	id, _ := broker.Open("load-1", permission)
	done := make(chan Poll, 1)
	go func() {
		poll, _ := broker.Poll(context.Background(), "load-1", id, 5*time.Second)
		done <- poll
	}()
	time.Sleep(10 * time.Millisecond)
	if err := broker.Settle("load-1", id, Local); err != nil {
		t.Fatal(err)
	}
	if poll := <-done; !poll.Settled {
		t.Fatalf("the poll did not wake to the settling: %+v", poll)
	}
	if _, err := broker.Propose(id, Answer{}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an answer after the settling: %v", err)
	}

	id, _ = broker.Open("load-1", permission)
	outcome, err := broker.Propose(id, Answer{})
	if err != nil {
		t.Fatal(err)
	}
	if err := broker.Settle("load-1", id, Local); err != nil {
		t.Fatal(err)
	}
	if got := <-outcome; got.Accepted || got.Winner != Local {
		t.Fatalf("main's answer that lost: %+v", got)
	}
}

// A request past its deadline polls expired and takes no answer; another
// load's poll or settling is refused.
func TestARequestExpiresAndBelongsToItsLoad(t *testing.T) {
	broker := NewBroker()
	now := time.Now()
	broker.now = func() time.Time { return now }
	id, deadline := broker.Open("load-1", question)
	if deadline.Sub(now) != Lifetime {
		t.Fatalf("the deadline is %s away", deadline.Sub(now))
	}
	if _, err := broker.Poll(context.Background(), "load-2", id, time.Millisecond); !errors.Is(err, ErrLoad) {
		t.Fatalf("another load's poll: %v", err)
	}
	if err := broker.Settle("load-2", id, Main); !errors.Is(err, ErrLoad) {
		t.Fatalf("another load's settling: %v", err)
	}
	now = now.Add(Lifetime)
	if poll, err := broker.Poll(context.Background(), "load-1", id, time.Second); err != nil || !poll.Expired {
		t.Fatalf("past the deadline: %+v, %v", poll, err)
	}
	if _, err := broker.Propose(id, Answer{}); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an answer past the deadline: %v", err)
	}
	now = now.Add(2 * time.Minute)
	broker.Open("load-1", question)
	if _, err := broker.Offer(id); !errors.Is(err, ErrUnknown) {
		t.Fatalf("an expired request nobody settled stays: %v", err)
	}
}

func TestTheLetterNamesTheRequestAndTheCommand(t *testing.T) {
	text := Letter("api", "3f9a", permission, time.Now().Add(Lifetime))
	first, _, _ := strings.Cut(text, "\n")
	if !strings.Contains(first, "api asks permission for Bash (request 3f9a)") || !strings.Contains(text, "rewake answer api 3f9a once") || !strings.Contains(text, `{"command":"make"}`) {
		t.Fatalf("the permission letter:\n%s", text)
	}
	// Only a call its tool calls read-only is marked so; it is still asked.
	if strings.Contains(text, "Read-only") {
		t.Fatalf("an unmarked call reads as read-only:\n%s", text)
	}
	reading := permission
	reading.ReadOnly = true
	if text := Letter("api", "3f9a", reading, time.Now()); !strings.Contains(text, "Read-only: the tool says this call only reads") || !strings.Contains(text, "rewake answer api 3f9a once") {
		t.Fatalf("the read-only letter:\n%s", text)
	}
	text = Letter("api", "3f9a", question, time.Now())
	if !strings.Contains(text, "Which?") || !strings.Contains(text, "  2 Beta") || !strings.Contains(text, "1,3") {
		t.Fatalf("the question letter:\n%s", text)
	}
}
