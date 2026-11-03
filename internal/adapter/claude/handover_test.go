package claude

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/claude/modhost"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// takes stands in for the core: it fails the first publications it is told
// to, and keeps every end it was handed.
type takes struct {
	mu      sync.Mutex
	failing int
	reads   uint64
	ends    []adapter.TurnEnd
}

func (c *takes) handler() adapter.TurnHandler {
	return adapter.TurnHandler{
		Capture: func() *mail.ReadBoundary {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.reads++
			return &mail.ReadBoundary{Mailbox: "api", Epoch: "e1", Through: c.reads}
		},
		Publish: func(_ context.Context, end adapter.TurnEnd) error {
			c.mu.Lock()
			defer c.mu.Unlock()
			c.ends = append(c.ends, end)
			if c.failing > 0 {
				c.failing--
				return errors.New("the mailbox is busy")
			}
			return nil
		},
	}
}

// postMod asks the run's mod endpoint one request on a connection of its own.
func postMod(t *testing.T, socket, path string, body any) (int, string) {
	t.Helper()
	client := http.Client{Timeout: 10 * time.Second, Transport: &http.Transport{
		DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, "unix", socket)
		},
	}}
	data, _ := json.Marshal(body)
	answer, err := client.Post("http://rewake"+path, "application/json", bytes.NewReader(data))
	if err != nil {
		t.Fatalf("%s: %v", path, err)
	}
	defer func() { _ = answer.Body.Close() }()
	text, _ := io.ReadAll(answer.Body)
	return answer.StatusCode, string(text)
}

// handedRun is a run's service as the host plans it, its turns going to c and
// its live sets to sets.
func handedRun(t *testing.T, request adapter.LaunchRequest, c *takes, sets chan<- []adapter.Capability) *modRun {
	t.Helper()
	plan, err := New().Launch(request)
	if err != nil {
		t.Fatal(err)
	}
	run, ok := plan.Service.(*modRun)
	if !ok {
		t.Fatalf("the run's service is %T", plan.Service)
	}
	run.ReportTurns(c.handler())
	run.ReportLive(func(set []adapter.Capability) { sets <- set })
	return run
}

// An idle Claude Code run is handed over to the next image: the mod's next
// poll is answered 200, a resent end is the same event, and the lane listens
// again with its gate open. Before that, an end the core has not taken and a
// notice whose late word may still come each keep it from going: the first
// capture of the one and the receipts of the other live only in this image.
func TestAnIdleClaudeRunIsHandedOverAndTheModsNextPollAnswered(t *testing.T) {
	room := shortRoom(t)
	privateHome(t)
	request := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "runs", "api.e1"), Role: role.Write, Work: func() string { return "work" },
	}
	if err := state.EnsureSubdir(request.RunDir); err != nil {
		t.Fatal(err)
	}
	core := &takes{failing: 1}
	sets := make(chan []adapter.Capability, 16)
	before := handedRun(t, request, core, sets)
	if err := before.Start(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	before.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	socket := modSocketPath(request)
	var hello modhost.HelloAnswer
	status, text := postMod(t, socket, modhost.PathHello, modhost.HelloRequest{Protocol: modhost.Protocol, LoadID: "load-1", SessionID: "conversation-1", HarnessVersion: "2.1.310"})
	if status != http.StatusOK || json.Unmarshal([]byte(text), &hello) != nil {
		t.Fatalf("the hello: %d %s", status, text)
	}

	answer := "done"
	end := modhost.TurnEndRequest{LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1", EndID: "end-1", Reason: modhost.EndReasonAnswered, Answer: &answer}
	if status, _ := postMod(t, socket, modhost.PathTurnEnd, end); status != http.StatusServiceUnavailable {
		t.Fatalf("the end the core failed: %d", status)
	}
	if why := before.Busy(); !strings.Contains(why, "not yet taken") {
		t.Fatalf("with an end the core has not taken, Busy says %q", why)
	}
	if _, err := before.HandOver(); err == nil {
		t.Fatal("an end the core has not taken was handed over")
	}
	if status, _ := postMod(t, socket, modhost.PathTurnEnd, end); status != http.StatusOK {
		t.Fatalf("the end's retry: %d", status)
	}
	if why := before.Busy(); why != "" {
		t.Fatalf("with every end taken, Busy says %q", why)
	}

	// A notice the session took in silence: a late word may take it back.
	receiver := startReceiver(t, filepath.Join(room, "sock"))
	before.lane.socket = receiver.socket
	if result := before.Deliver(context.Background(), mail.Message{ID: "m1", Kind: mail.Task, Text: "work"}); result.State != mail.Delivered {
		t.Fatalf("the notice: %+v", result)
	}
	if why := before.Busy(); !strings.Contains(why, "late word") {
		t.Fatalf("with a notice in its late word's window, Busy says %q", why)
	}
	before.mu.Lock()
	for _, sent := range before.sent {
		sent.at = time.Now().Add(-2 * mail.LateWordWindow)
	}
	before.mu.Unlock()
	if why := before.Busy(); why != "" {
		t.Fatalf("past the late word's window, Busy says %q", why)
	}

	handed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	// The exec: the old image's descriptors go, the socket's path and the
	// copy handed over stay.
	aside := socket + ".aside"
	if err := os.Rename(socket, aside); err != nil {
		t.Fatal(err)
	}
	before.Close()
	if err := os.Rename(aside, socket); err != nil {
		t.Fatal(err)
	}
	for len(sets) > 0 {
		<-sets
	}

	after := handedRun(t, request, core, sets)
	if err := after.Adopt(context.Background(), func(note string) { t.Error(note) }, handed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	shown := "shown before"
	poll := modhost.AliveRequest{LoadID: "load-1", Generation: hello.Generation, Work: &shown}
	if status, text := postMod(t, socket, modhost.PathModAlive, poll); status != http.StatusOK {
		t.Fatalf("the mod's next poll: %d %s", status, text)
	}
	if status, _ := postMod(t, socket, modhost.PathTurnEnd, end); status != http.StatusOK {
		t.Fatalf("the end resent after the handover: %d", status)
	}
	core.mu.Lock()
	ends := append([]adapter.TurnEnd{}, core.ends...)
	core.mu.Unlock()
	if last := ends[len(ends)-1]; last.Event != ends[0].Event {
		t.Fatalf("the resent end is %s, first %s: the core's done journal answers it by its event", last.Event, ends[0].Event)
	}
	if got := fmt.Sprint(<-sets, <-sets); !strings.Contains(got, string(adapter.TurnBoundaryCapability)) {
		t.Fatalf("the adopted run's live sets: %s", got)
	}
	select {
	case <-after.Opened():
	case <-time.After(time.Second):
		t.Fatal("the adopted lane's gate waits as at a session's start")
	}
	if info, err := os.Stat(after.reply); err != nil || info.Mode()&os.ModeSocket == 0 {
		t.Fatalf("the reply socket after the handover: %v", err)
	}
}

// handedShapes pins each version of the run's handed state to its shape,
// the endpoint's included: a switch of the room's build admits only a build
// that takes the same handover.claude, so a changed shape raises
// handoverVersion.
var handedShapes = map[int]string{
	1: "136c3efd43a1edacab1737b21292751682ae4a0f157a4db611675ec7d3748816",
	2: "8d96b4942f34b075a344f6789ac8cdbcf3802fd33dcfc66e20b2c489c53670f2",
}

func TestTheClaudeHandoverChangesWithItsShape(t *testing.T) {
	sum := sha256.Sum256([]byte(shapeOf(reflect.TypeFor[handedState]())))
	if digest := hex.EncodeToString(sum[:]); digest != handedShapes[handoverVersion] {
		t.Fatalf("handedState's shape is %s, and handover.claude %d pins %q: a changed shape raises handoverVersion and pins its own", digest, handoverVersion, handedShapes[handoverVersion])
	}
}

// shapeOf is a type as JSON carries it: each exported field's name, shape
// and tag, nested types included.
func shapeOf(t reflect.Type) string {
	switch t.Kind() {
	case reflect.Pointer:
		return "*" + shapeOf(t.Elem())
	case reflect.Slice:
		return "[]" + shapeOf(t.Elem())
	case reflect.Struct:
		var fields []string
		for index := range t.NumField() {
			if field := t.Field(index); field.IsExported() {
				fields = append(fields, fmt.Sprintf("%s %s %q", field.Name, shapeOf(field.Type), field.Tag.Get("json")))
			}
		}
		return "{" + strings.Join(fields, "; ") + "}"
	}
	return t.Kind().String()
}

// An end that comes after the handover's snapshot is not captured in the
// image that goes: it is refused, the mod sends it again, and the next image
// captures it first, so its boundary is the one the core takes once. After an
// exec that failed, the run takes effects again.
func TestAnEndAfterTheSnapshotIsCapturedByTheNextImage(t *testing.T) {
	room := shortRoom(t)
	privateHome(t)
	request := adapter.LaunchRequest{
		Name: "api", Epoch: "e1", Dir: room, RoomDir: room, Socket: filepath.Join(room, "sock", "api.e1.sock"),
		RunDir: filepath.Join(room, "runs", "api.e1"), Role: role.Write, Work: func() string { return "work" },
	}
	if err := state.EnsureSubdir(request.RunDir); err != nil {
		t.Fatal(err)
	}
	core := &takes{}
	sets := make(chan []adapter.Capability, 16)
	before := handedRun(t, request, core, sets)
	if err := before.Start(context.Background(), func(string) {}); err != nil {
		t.Fatal(err)
	}
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	before.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	socket := modSocketPath(request)
	if status, text := postMod(t, socket, modhost.PathHello, modhost.HelloRequest{Protocol: modhost.Protocol, LoadID: "load-1", SessionID: "conversation-1", HarnessVersion: "2.1.310"}); status != http.StatusOK {
		t.Fatalf("the hello: %d %s", status, text)
	}
	answer := "earlier answer"
	end := modhost.TurnEndRequest{LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1", EndID: "end-1", Reason: modhost.EndReasonAnswered, Answer: &answer}
	turn := modhost.TurnStartRequest{LoadID: "load-1", Conversation: "conversation-1", TurnID: "turn-1"}

	failed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := postMod(t, socket, modhost.PathTurnStart, turn); status != http.StatusServiceUnavailable {
		t.Fatalf("a turn's start after the snapshot: %d", status)
	}
	failed.Resume()
	closeAll(failed.Files)
	if status, text := postMod(t, socket, modhost.PathTurnStart, turn); status != http.StatusOK {
		t.Fatalf("a turn's start after an exec that failed: %d %s", status, text)
	}

	handed, err := before.HandOver()
	if err != nil {
		t.Fatal(err)
	}
	if status, _ := postMod(t, socket, modhost.PathTurnEnd, end); status != http.StatusServiceUnavailable {
		t.Fatalf("an end after the snapshot: %d", status)
	}
	aside := socket + ".aside"
	if err := os.Rename(socket, aside); err != nil {
		t.Fatal(err)
	}
	before.Close()
	if err := os.Rename(aside, socket); err != nil {
		t.Fatal(err)
	}
	after := handedRun(t, request, core, sets)
	if err := after.Adopt(context.Background(), func(string) {}, handed); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(after.Close)
	if status, text := postMod(t, socket, modhost.PathTurnEnd, end); status != http.StatusOK {
		t.Fatalf("the end sent again to the next image: %d %s", status, text)
	}
	core.mu.Lock()
	defer core.mu.Unlock()
	if len(core.ends) != 1 || core.reads != 1 {
		t.Fatalf("the core was handed %d ends over %d captures: an end after the snapshot was captured by the image that went", len(core.ends), core.reads)
	}
}
