package mainhost

import (
	"os"
	"sync"
	"syscall"
	"time"

	"github.com/praline-labs/rewake/internal/host/stream"
	"github.com/praline-labs/rewake/internal/host/switching"
)

// Main's host joins a switch of the room's build (host/switching) as
// "main's host" (docs/shell.md#a-switch-of-the-rooms-build). Its drop is an
// event of the loop: the producers of events stop, each only up to its post;
// what they posted, and every signal caught, is taken as usual; a host the
// drain left not ready resumes and says why; otherwise it execs its own
// image parked, with main's terminal, the shell's channel and the loop's
// state handed over (handover.go). Main's wrapper and the shell are this
// process's children all through, and see nothing of it.

// The drop's bounds: the relay's each step, and the stop of the producers
// and the door's last answers.
const (
	handBound   = 2 * time.Second
	stillWithin = 2 * time.Second
)

// participant is main's host as a switch asks it.
type participant struct{ h *Host }

func (p participant) Who() string { return "main's host" }

// Answer is ready only while nothing is under way the exec would cut: the
// loop's own reason, published after each event, and the bounds running.
func (p participant) Answer() (string, string) {
	why := p.h.busy.Load()
	switch {
	case why == nil:
		return switching.Busy, "main's host is starting"
	case *why != "":
		return switching.Busy, *why
	case p.h.timers.Load() > 0:
		return switching.Busy, "main's host is waiting out one of its bounds"
	}
	return switching.Ready, ""
}

// Drop asks the loop to hand the host over; it returns only when it did not.
func (p participant) Drop(park switching.Park) string {
	if kind, why := p.Answer(); kind != switching.Ready {
		return why
	}
	reply := make(chan string, 1)
	select {
	case p.h.events <- ev{kind: kDrop, park: &park, drop: reply}:
	case <-time.After(time.Second):
		return "main's host did not take the switch"
	}
	return <-reply
}

// publish says, for Answer, why the host cannot be handed over now.
func (h *Host) publish() {
	why := h.why()
	h.busy.Store(&why)
}

// why is the loop's reason the host cannot be handed over now, "" when it
// can: a shell drawing, alone, ready and read, and nothing owed.
func (h *Host) why() string {
	switch {
	case h.phase == alone:
		return "main runs alone, without rewake's shell"
	case h.phase == closing || h.ending:
		return "rewake is closing"
	case h.phase == swapping || h.candidate != nil || len(h.waiting) > 0:
		return "a restart of rewake's shell is under way"
	case h.phase != running || h.shell == nil || !h.shell.ready || len(h.unreaped) != 1:
		return "rewake's shell is starting"
	case h.ref == nil || h.main == nil:
		return "main's session is not registered yet"
	case len(h.reloads) > 0:
		return "a reload of rewake's face is under way"
	case !closed(h.shell.digested):
		return "the parts of rewake's shell are being read"
	}
	return ""
}

// take is the loop's handling of one event: a child's exit reaped first.
func (h *Host) take(e ev) bool {
	switch e.kind {
	case kSignal:
		return h.signal(e.sig)
	case kDrop:
		return h.drop(*e.park, e.drop)
	case kMainExited:
		e.code = reap(h.main.pid)
	case kExit:
		if p := h.unreaped[e.gen]; p != nil {
			reap(p.pid)
			p.reaped = true
			_ = p.errPipe.Close()
			delete(h.unreaped, e.gen)
		}
	}
	return h.handle(e)
}

// stilled is what a drop stopped, and what came in meanwhile.
type stilled struct {
	handing *stream.Handing
	queue   []ev
}

// drop hands the host over, or answers why not; true ends the host.
func (h *Host) drop(park switching.Park, reply chan string) bool {
	h.publish()
	if h.dropping {
		reply <- "main's host is already being handed over"
		return false
	}
	if kind, why := (participant{h}).Answer(); kind != switching.Ready {
		reply <- why
		return false
	}
	s, why := h.still()
	if why != "" {
		return h.refuse(s, reply, why)
	}
	// The drain: what came is taken as usual, and every handler of the
	// door past its post has its answer once the loop is through them.
	for len(s.queue) > 0 || len(h.events) > 0 || len(h.signals) > 0 {
		if len(s.queue) == 0 {
			h.gather(s)
			continue
		}
		e := s.queue[0]
		s.queue = s.queue[1:]
		if h.take(e) {
			h.resume(s)
			reply <- "rewake is closing"
			return true
		}
	}
	h.publish()
	if kind, why := (participant{h}).Answer(); kind != switching.Ready {
		return h.refuse(s, reply, why)
	}
	if !waitGroup(&h.doors.answering, stillWithin) {
		return h.refuse(s, reply, "an answer at main's host's door is still being written")
	}
	// Nothing posts now but a child's waiter, whose exit stays unreaped.
	if len(h.events) > 0 || len(h.signals) > 0 || hasExited(h.main.pid) || hasExited(h.shell.pid) {
		return h.refuse(s, reply, "main's host had more to take")
	}
	err := h.handOff(park, s.handing)
	return h.refuse(s, reply, "main's host could not be handed over: "+err.Error())
}

// still stops the producers of events, each only up to its post, and keeps
// what they posted meanwhile: a door's handler that posted waits on the
// loop's answer, so none is waited for past its post.
func (h *Host) still() (*stilled, string) {
	h.dropping = true
	h.doors.stop()
	handing, err := h.main.relay.HandOver(handBound)
	if err != nil {
		h.doors.resume()
		h.dropping = false
		return nil, "main's terminal could not be handed over: " + err.Error()
	}
	s := &stilled{handing: handing}
	h.stopUpdates()
	h.shell.pause()
	parent := make(chan struct{})
	go func() {
		h.stopParent()
		close(parent)
	}()
	deadline := time.Now().Add(stillWithin)
	for !h.doors.still() || !closed(h.shell.paused) || !closed(parent) {
		if time.Now().After(deadline) {
			return s, "main's host did not stop within " + stillWithin.String()
		}
		h.gather(s)
	}
	return s, ""
}

// gather keeps what comes for the drain, waiting a moment for it.
func (h *Host) gather(s *stilled) {
	select {
	case e := <-h.events:
		s.queue = append(s.queue, e)
	case sig := <-h.signals:
		s.queue = append(s.queue, ev{kind: kSignal, sig: sig})
	case <-time.After(time.Millisecond):
	}
}

// refuse resumes what still stopped, answers why, and takes what was kept;
// true ends the host.
func (h *Host) refuse(s *stilled, reply chan string, why string) bool {
	h.resume(s)
	reply <- why
	if s == nil {
		return false
	}
	for _, e := range s.queue {
		if h.take(e) {
			return true
		}
	}
	return false
}

// resume serves and reads again after a drop that did not exec.
func (h *Host) resume(s *stilled) {
	if s == nil || !h.dropping {
		return
	}
	h.dropping = false
	_ = s.handing.Resume()
	h.doors.resume()
	h.shell.resume(h)
	h.watchParent()
	h.watchUpdates()
}

// pause stops the shell's reader before its next post.
func (p *shellProc) pause() {
	p.mu.Lock()
	p.stopping, p.paused = true, make(chan struct{})
	p.mu.Unlock()
	_ = p.conn.SetReadDeadline(time.Unix(1, 0))
}

// resume reads the shell again: a reader that paused starts anew, one that
// had not yet goes on.
func (p *shellProc) resume(h *Host) {
	if closed(p.exited) {
		_ = p.conn.SetReadDeadline(time.Now().Add(drainWithin))
	} else {
		_ = p.conn.SetReadDeadline(time.Time{})
	}
	p.mu.Lock()
	p.stopping = false
	paused := closed(p.paused)
	p.mu.Unlock()
	if paused {
		go h.readMessages(p)
	}
}

// waitGroup waits for wg within d.
func waitGroup(wg *sync.WaitGroup, d time.Duration) bool {
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return true
	case <-time.After(d):
		return false
	}
}

// closed says c is closed; a nil channel never is.
func closed(c chan struct{}) bool {
	if c == nil {
		return false
	}
	select {
	case <-c:
		return true
	default:
		return false
	}
}

// inherit says whether file goes to the next image, clearing or setting its
// close-on-exec, and answers its descriptor; it goes through the raw
// descriptor, as Fd would make it blocking for good.
func inherit(file syscall.Conn, across bool) (int, error) {
	raw, err := file.SyscallConn()
	if err != nil {
		return 0, err
	}
	flag := uintptr(syscall.FD_CLOEXEC)
	if across {
		flag = 0
	}
	var fd int
	var errno syscall.Errno
	if err := raw.Control(func(d uintptr) {
		fd = int(d)
		_, _, errno = syscall.Syscall(syscall.SYS_FCNTL, d, syscall.F_SETFD, flag)
	}); err != nil {
		return 0, err
	}
	if errno != 0 {
		return 0, os.NewSyscallError("fcntl", errno)
	}
	return fd, nil
}
