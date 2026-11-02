package mainhost

import (
	"context"
	"net"
	"os"
	"syscall"

	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/switching"
	"github.com/praline-labs/rewake/internal/infra/build"
)

// adopt takes on the run an earlier image handed over and runs the loop
// from where it stood: main's terminal and its door served again, the
// shell's channel read on from what was left of it, both children waited as
// the children they still are. lost is why the run cannot go on in the
// room — a switch lost, or the room's lease refused — and the host then
// closes everything as a close asked for does: nothing of the run is left
// behind with nobody to end it.
func (h *Host) adopt(handed *Handover, lost string) int {
	h.parent, h.mode, h.crashed, h.dirty = handed.Parent, handed.Mode, handed.Crashed, handed.Dirty
	h.gen, h.colors, h.size, h.released, h.count = handed.Gen, handed.Colors, handed.Size, handed.Released, handed.Count
	ref := handed.Ref
	h.ref, h.phase = &ref, running
	for _, sig := range handed.Signals {
		h.replay = append(h.replay, syscall.Signal(sig))
	}
	relay, listener, err := stream.Adopt(handed.Stream, HostSocket(h.cfg.Dir), h.colors, markWatcher(func() { h.post(ev{kind: kMark}) }))
	if err != nil {
		return h.abandon(handed, "main's terminal could not be taken on after a switch of the room's build: "+err.Error())
	}
	m := &mainRun{pid: handed.Main.PID, start: handed.Main.Start, pty: relay.Terminal(), relay: relay, done: make(chan struct{})}
	h.main = m
	h.waitMain(m)
	p, l, err := shellOf(handed.Shell)
	if err != nil {
		_ = relay.Grid().Close()
		_ = listener.Close()
		return h.abandon(handed, "rewake's shell could not be heard after a switch of the room's build: "+err.Error())
	}
	go p.keepErr()
	if handed.Shell.Digested {
		close(p.digested)
	} else {
		go p.takeDigest(h.cfg.Shell)
	}
	h.shell = p
	h.follow(p, l, false)
	h.watchParent()
	h.watchUpdates()
	lease, err := build.Follow(h.cfg.Base, h.cfg.Room)
	if lost == "" && err != nil {
		lost = "main's host could not take the room's lease after a switch: " + err.Error()
	}
	if lost != "" {
		_ = listener.Close()
		h.replay = append(h.replay, syscall.SIGTERM)
		return h.loop()
	}
	m.lease, m.listener = lease, listener
	m.stopSwitching = switching.Watch(context.Background(), h.cfg.Dir, participant{h})
	go relay.Serve(listener, map[byte]func(net.Conn){modeControl: h.door})
	return h.loop()
}

// abandon ends what was handed over when main's terminal cannot be: main
// hears its terminal hang up with the master gone, the shell is ended, and
// the person's terminal given back.
func (h *Host) abandon(handed *Handover, why string) int {
	_ = syscall.Kill(-handed.Main.PID, syscall.SIGHUP)
	_ = syscall.Kill(handed.Shell.PID, syscall.SIGKILL)
	h.restoreTerminal()
	_, _ = os.Stderr.WriteString("rewake: " + why + "\n")
	return 1
}
