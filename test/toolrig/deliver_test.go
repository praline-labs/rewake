//go:build rewakefixture

package toolrig

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/adapter/fixture"
	"github.com/praline-labs/rewake/internal/core/mail"
	"github.com/praline-labs/rewake/internal/core/role"
	"github.com/praline-labs/rewake/internal/host/delivery"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// relayEnv names the socket of a test that plays the fixture's program
// itself. The program started under the adapter is then a relay: it says
// hello as the started process, which the adapter checks the program is, and
// passes every later line between the adapter and the test unchanged, so the
// test answers each request when it chooses.
const relayEnv = "TOOLRIG_RELAY"

const (
	relayThread   = "toolrig-relay-thread"
	programThread = "toolrig-program-thread"
)

func runRelay(args []string) int {
	connect := ""
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--connect" {
			connect = args[i+1]
		}
	}
	adapterSide, err := net.Dial("unix", connect)
	if err != nil {
		fmt.Fprintln(os.Stderr, "toolrig relay:", err)
		return 1
	}
	testSide, err := net.Dial("unix", os.Getenv(relayEnv))
	if err != nil {
		fmt.Fprintln(os.Stderr, "toolrig relay:", err)
		return 1
	}
	hello, _ := json.Marshal(fixture.Frame{Op: "hello", Version: "1.0.0", PID: os.Getpid(), Thread: relayThread, Serves: []string{fixture.Wake}})
	if _, err := adapterSide.Write(append(hello, '\n')); err != nil {
		return 1
	}
	done := make(chan struct{}, 2)
	go func() { _, _ = io.Copy(testSide, adapterSide); done <- struct{}{} }()
	go func() { _, _ = io.Copy(adapterSide, testSide); done <- struct{}{} }()
	<-done
	return 0
}

// relayed is the test's end of a relay: the program's side of the exchange.
type relayed struct {
	conn   net.Conn
	reader *bufio.Reader
}

func (p relayed) write(t *testing.T, frame fixture.Frame) {
	t.Helper()
	raw, _ := json.Marshal(frame)
	if _, err := p.conn.Write(append(raw, '\n')); err != nil {
		t.Fatal(err)
	}
}

func (p relayed) read(t *testing.T) fixture.Frame {
	t.Helper()
	_ = p.conn.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := p.reader.ReadBytes('\n')
	if err != nil {
		t.Fatal(err)
	}
	var frame fixture.Frame
	if err := json.Unmarshal(line, &frame); err != nil {
		t.Fatalf("%q: %v", line, err)
	}
	return frame
}

// relayRun starts a run of the fixture under its adapter whose program is the
// test, through a relay, with Wake live.
func relayRun(t *testing.T, dir, epoch string) (adapter.LaunchPlan, relayed) {
	t.Helper()
	root := t.TempDir()
	listener, err := net.Listen("unix", filepath.Join(root, "relay.sock"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = listener.Close() })
	self, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(root, "fixture")
	script := "#!/bin/sh\nexec env '" + relayEnv + "=" + filepath.Join(root, "relay.sock") + "' '" + self + "' \"$@\"\n"
	if err := writeExecutable(launcher, script); err != nil {
		t.Fatal(err)
	}
	plan, err := fixture.New().Launch(adapter.LaunchRequest{
		Name: "api", Dir: dir, Room: "default", Role: role.Write, Epoch: epoch,
		Socket: filepath.Join(root, "f.sock"), Command: launcher,
	})
	if err != nil {
		t.Fatal(err)
	}
	// The exchange waits for the probe's answer, which the test gives.
	started := make(chan error, 1)
	go func() { started <- plan.Service.Start(context.Background(), nil) }()
	accepted := make(chan net.Conn, 1)
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			accepted <- conn
		}
	}()
	var conn net.Conn
	select {
	case conn = <-accepted:
	case err := <-started:
		t.Fatalf("the program never reached the test: %v", err)
	case <-time.After(10 * time.Second):
		t.Fatal("the program never reached the test")
	}
	t.Cleanup(func() { _ = conn.Close() })
	p := relayed{conn: conn, reader: bufio.NewReader(conn)}
	probe := p.read(t)
	if probe.Op != "probe" || probe.Capability != fixture.Wake {
		t.Fatalf("the exchange asked %+v", probe)
	}
	p.write(t, fixture.Frame{Op: "answer", ID: probe.ID, OK: true})
	if err := <-started; err != nil {
		t.Fatalf("the program: %v", err)
	}
	t.Cleanup(plan.Service.Close)
	return plan, p
}

// No notice precedes a reserved letter's readability: through the mailbox
// server the wrapper runs, the program is asked to reserve while the letter is
// not yet readable, and when the notice reaches it the letter is: what holds
// for any Reserver.
func TestNoNoticePrecedesAReservedLettersReadability(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	dir, err := state.Bootstrap(root, "default")
	if err != nil {
		t.Fatal(err)
	}
	plan, p := relayRun(t, dir, "1.1")
	reserving, ok := plan.Wake.(adapter.Reserver)
	if !ok || plan.Thread == nil {
		t.Fatal("the fixture's run reserves nothing or follows no thread")
	}
	server := &delivery.Server{Dir: dir, Name: "api", Epoch: "1.1", Thread: plan.Thread.Thread, Deliver: plan.Wake.Deliver, Reserve: reserving.Reserve}
	ctx, cancel := context.WithCancel(context.Background())
	served := make(chan struct{})
	go func() { defer close(served); server.Serve(ctx) }()
	t.Cleanup(func() { cancel(); <-served })

	task := mail.Message{ID: mail.NewID(), From: "web", FromEpoch: "2.2", To: "api", ToEpoch: "1.1", Kind: mail.Task, Text: "work", CreatedAt: time.Now()}
	if err := mail.Put(dir, task); err != nil {
		t.Fatal(err)
	}
	readable := func() bool {
		unread, err := mail.PeekUnread(dir, "api", "1.1")
		return err == nil && slices.ContainsFunc(unread, func(m mail.Message) bool { return m.ID == task.ID })
	}
	reserve := p.read(t)
	if reserve.Op != "reserve" {
		t.Fatalf("the first request was %s", reserve.Op)
	}
	if readable() {
		t.Fatal("the letter was readable before its reservation was answered")
	}
	p.write(t, fixture.Frame{Op: "answer", ID: reserve.ID, OK: true, Thread: programThread})
	deliver := p.read(t)
	if deliver.Op != "deliver" || deliver.Thread != programThread || len(deliver.Members) != 1 || deliver.Members[0].ID != task.ID {
		t.Fatalf("the notice: %+v", deliver)
	}
	if !readable() {
		t.Fatal("the notice preceded the letter's readability")
	}
	p.write(t, fixture.Frame{Op: "answer", ID: deliver.ID, OK: true})
	if release := p.read(t); release.Op != "release" {
		t.Fatalf("the reservation was not let go: %+v", release)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if status, ok, _ := mail.ReadStatus(dir, "api", task.ID); ok && status.State == mail.Delivered {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the delivery was never recorded")
		}
		time.Sleep(20 * time.Millisecond)
	}
}
