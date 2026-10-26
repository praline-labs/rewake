package modhost

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	asking "github.com/praline-labs/rewake/internal/core/ask"
	"github.com/praline-labs/rewake/internal/core/control"
)

// askServing is a worker's mod endpoint whose letters to main are kept in
// told, with a control directory and an admitted load.
func askServing(t *testing.T) (*Server, string, *[]asking.Offer) {
	t.Helper()
	var told []asking.Offer
	server, socket := servingWith(t, func(server *Server) {
		server.cfg.ControlDir = filepath.Join(filepath.Dir(server.cfg.Record), "control")
		server.cfg.Hold = 10 * time.Millisecond
		server.cfg.Asked = func(_ string, offer asking.Offer, _ time.Time) error {
			told = append(told, offer)
			return nil
		}
		if err := control.Prepare(server.cfg.ControlDir); err != nil {
			t.Fatal(err)
		}
	})
	server.StartControl(context.Background())
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	return server, socket, &told
}

func openPermission(t *testing.T, socket string) string {
	t.Helper()
	tool, input := "Bash", `{"command":"make"}`
	text := postValue(t, socket, PathAskOpen, AskOpenRequest{LoadID: "load", Kind: AskKindPermission, Tool: &tool, Input: &input, Questions: []AskQuestion{{
		Question: "Allow Bash?", Options: []AskOption{{Label: "Allow once"}, {Label: "Allow for this session"}, {Label: "Refuse"}},
	}}})
	var opened AskOpenAnswer
	if err := json.Unmarshal([]byte(text), &opened); err != nil || opened.ID == "" {
		t.Fatalf("open: %s %v", text, err)
	}
	return opened.ID
}

func answerAs(server *Server, id string, words ...string) <-chan control.Answer {
	result := make(chan control.Answer, 1)
	go func() {
		answer, err := control.Ask(context.Background(), server.cfg.ControlDir, control.Request{Action: control.Reply, From: "main", Ask: id, Choices: words}, control.Limits{Pickup: time.Second, Outcome: 20 * time.Second, Poll: time.Millisecond})
		if err != nil {
			answer.Detail = err.Error()
		}
		result <- answer
	}()
	return result
}

func pollAsk(t *testing.T, socket, id string) AskPollAnswer {
	t.Helper()
	var poll AskPollAnswer
	if err := json.Unmarshal([]byte(postValue(t, socket, PathAskPoll, AskPollRequest{LoadID: "load", ID: id})), &poll); err != nil {
		t.Fatal(err)
	}
	return poll
}

// Main's answer through the control directory wakes the mod's poll, and main
// hears it was taken once the mod settles the request with it.
func TestMainsAnswerCrossesToTheModAndBack(t *testing.T) {
	server, socket, told := askServing(t)
	id := openPermission(t, socket)
	if len(*told) != 1 || (*told)[0].Tool != "Bash" || (*told)[0].Input != `{"command":"make"}` {
		t.Fatalf("main was told %+v", *told)
	}
	result := answerAs(server, id, "refuse", "not", "now")
	poll := pollAsk(t, socket, id)
	if poll.State != AskStateAnswered || len(poll.Answers) != 1 || poll.Answers[0].Answer != "Refuse" || poll.Reason == nil || *poll.Reason != "not now" {
		t.Fatalf("the poll: %+v", poll)
	}
	postValue(t, socket, PathAskSettle, AskSettleRequest{LoadID: "load", ID: id, Winner: AskWinnerMain})
	select {
	case answer := <-result:
		if answer.Outcome != control.Done {
			t.Fatalf("main's answer: %+v", answer)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("main's answer was not answered")
	}
	if status, _, _ := post(socket, PathAskPoll, mustJSON(AskPollRequest{LoadID: "load", ID: id})); status != http.StatusNotFound {
		t.Fatalf("a settled request polls %d", status)
	}
}

// The person answering first is what main's answer hears; a word that is no
// choice is refused before anything reaches the mod.
func TestMainHearsWhoAnsweredFirst(t *testing.T) {
	server, socket, _ := askServing(t)
	id := openPermission(t, socket)
	if answer := <-answerAs(server, id, "yes"); answer.Outcome != control.Refused || answer.Reason != control.NotAChoice {
		t.Fatalf("a word that is no choice: %+v", answer)
	}
	result := answerAs(server, id, "once")
	if poll := pollAsk(t, socket, id); poll.State != AskStateAnswered {
		t.Fatalf("the poll: %+v", poll)
	}
	postValue(t, socket, PathAskSettle, AskSettleRequest{LoadID: "load", ID: id, Winner: AskWinnerLocal})
	if answer := <-result; answer.Outcome != control.Refused || answer.Reason != control.AnsweredFirst {
		t.Fatalf("main's answer that lost: %+v", answer)
	}
	if answer := <-answerAs(server, "beef", "once"); answer.Outcome != control.Refused || answer.Reason != control.NoRequest {
		t.Fatalf("an answer to no request: %+v", answer)
	}
}

// A run main does not answer for, or a load not admitted, opens nothing.
func TestARequestOpensOnlyForAWorkersAdmittedLoad(t *testing.T) {
	_, socket := serving(t)
	postValue(t, socket, PathHello, HelloRequest{LoadID: "load", SessionID: "conversation-1", HarnessVersion: "test"})
	open := AskOpenRequest{LoadID: "load", Kind: AskKindQuestion, Questions: []AskQuestion{{Question: "Which?", Options: []AskOption{{Label: "A"}}}}}
	if status, _, _ := post(socket, PathAskOpen, mustJSON(open)); status != http.StatusConflict {
		t.Fatalf("main's own run opened a request: %d", status)
	}
	_, socket, _ = askServing(t)
	open.LoadID = "another"
	if status, _, _ := post(socket, PathAskOpen, mustJSON(open)); status != http.StatusConflict {
		t.Fatalf("an unadmitted load opened a request: %d", status)
	}
}

func mustJSON(value any) []byte {
	data, _ := json.Marshal(value)
	return data
}
