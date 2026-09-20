package gateway

import (
	"context"
	"encoding/json"
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestReservedMailboxWireShapeAndOutcomes(t *testing.T) {
	for _, outcome := range []string{"accepted", "refused", "lost-ack"} {
		t.Run(outcome, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			bindUI(t, g, ui, native)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			r, err := g.Reserve(ctx)
			if err != nil {
				t.Fatal(err)
			}
			defer r.Close()
			notice := MailboxNotice{Notice: "Rewake: 1200 new messages"}
			for i := 0; i < 1200; i++ {
				notice.Members = append(notice.Members, MailboxMember{ID: fmt.Sprint(i), From: "sender", FromEpoch: "sender-epoch", To: "worker", ToEpoch: "recipient-epoch"})
			}
			done := make(chan error, 1)
			go func() {
				_, err := r.Deliver(ctx, "fixed-batch", notice, []string{"/existing", "/explicit-git"})
				done <- err
			}()
			raw := readWithin(t, native)
			var request struct {
				ID     string                     `json:"id"`
				Method string                     `json:"method"`
				Params map[string]json.RawMessage `json:"params"`
			}
			if json.Unmarshal(raw, &request) != nil {
				t.Fatal("invalid request")
			}
			p := request.Params
			if request.Method != "turn/start" || len(p) != 5 || string(p["threadId"]) != `"A"` || string(p["clientUserMessageId"]) != `"fixed-batch"` || string(p["input"]) != "[]" || string(p["runtimeWorkspaceRoots"]) != `["/existing","/explicit-git"]` {
				t.Fatalf("invalid envelope: %s", raw)
			}
			var output map[string]string
			if json.Unmarshal(p["toolOutput"], &output) != nil || len(output) != 2 || output["name"] != "rewake_mailbox_notice" {
				t.Fatalf("invalid standalone output: %s", p["toolOutput"])
			}
			var got MailboxNotice
			if len(output["output"]) <= 64<<10 || json.Unmarshal([]byte(output["output"]), &got) != nil || !reflect.DeepEqual(got, notice) {
				t.Fatal("fixed member metadata truncated or subjected to short-preview bound")
			}
			switch outcome {
			case "accepted":
				write(t, native, []byte(fmt.Sprintf(`{"id":%q,"result":{"turn":{"id":"work"}}}`, request.ID)))
			case "refused":
				write(t, native, []byte(fmt.Sprintf(`{"id":%q,"error":{"code":-32602,"message":"unsupported"}}`, request.ID)))
			case "lost-ack": // Upstream received the request; no ACK means no replay.
			}
			err = <-done
			if (err == nil) != (outcome == "accepted") {
				t.Fatalf("unexpected delivery outcome: %v", err)
			}
		})
	}
}
