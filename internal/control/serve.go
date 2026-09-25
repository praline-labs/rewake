package control

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Serve answers the requests put into a run's control directory until ctx
// ends: the served side for a harness whose wrapper carries requests out
// itself, as on Codex, where it holds the app-server connection. It follows
// the same steps as the Claude Code module (docs/remote-control.md): take a
// request, mark it taken, check it is still in place, act, and write the answer
// once. One request at a time: act runs on this goroutine, and the asker's lock
// lets no second request in meanwhile. withdrawn is given the answer Serve
// gives in act's place, once it is written — a final answer, which the served
// side records as the outcome as it does act's own (docs/remote-control-letter.md).
func Serve(ctx context.Context, dir string, poll time.Duration, act func(context.Context, Request) Answer, withdrawn func(Request, Answer)) {
	tick := time.NewTicker(poll)
	defer tick.Stop()
	last := ""
	for {
		last = serveOnce(ctx, dir, last, act, withdrawn)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// serveOnce takes the request in place, if any, and answers the id it handled
// last, so a request still in place while its asker reads the answer is not
// looked at again. The taken mark keeps it from being carried out twice beyond
// that.
func serveOnce(ctx context.Context, dir, last string, act func(context.Context, Request) Answer, withdrawn func(Request, Answer)) string {
	asked, ok := pendingRequest(dir)
	if !ok || asked.ID == last {
		return last
	}
	if exists(TakenPath(dir, asked.ID)) {
		return asked.ID
	}
	if os.WriteFile(TakenPath(dir, asked.ID), nil, 0o600) != nil {
		return asked.ID
	}
	afterMark(dir, asked.ID)
	if still, ok := pendingRequest(dir); !ok || still.ID != asked.ID {
		answer := Answer{ID: asked.ID, Outcome: Refused, Reason: Withdrawn}
		writeAnswer(dir, answer)
		withdrawn(asked, answer)
		return asked.ID
	}
	answer := act(ctx, asked)
	answer.ID = asked.ID
	writeAnswer(dir, answer)
	return asked.ID
}

// afterMark runs between marking a request taken and checking it is still in
// place; a test puts the asker's withdrawal of that instant there.
var afterMark = func(_, _ string) {}

// pendingRequest reads the request in place. One whose id is not of the
// accepted shape is ignored: file names are built from it.
func pendingRequest(dir string) (Request, bool) {
	raw, err := os.ReadFile(RequestPath(dir))
	if err != nil {
		return Request{}, false
	}
	var request Request
	if json.Unmarshal(raw, &request) != nil || !ValidID.MatchString(request.ID) {
		return Request{}, false
	}
	return request, true
}

// writeAnswer puts the answer in place whole. A leftover temporary file is
// cleared by the next asker with everything else.
func writeAnswer(dir string, answer Answer) {
	encoded, err := json.Marshal(answer)
	if err != nil {
		return
	}
	temporary := filepath.Join(dir, "."+answer.ID+".result.tmp")
	if os.WriteFile(temporary, encoded, 0o600) != nil {
		return
	}
	if os.Rename(temporary, AnswerPath(dir, answer.ID)) != nil {
		_ = os.Remove(temporary)
	}
}
