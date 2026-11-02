package switching

import (
	"context"
	"errors"
	"net"
	"os"
	"time"

	"github.com/praline-labs/rewake/internal/infra/build"
	"github.com/praline-labs/rewake/internal/infra/proc"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// poll is how often a participant looks at the room's mark, and says its
// answer again when it changed.
const poll = 250 * time.Millisecond

// Watch has p answer every switch of room that asks, until ctx ends or the
// returned stop is called. It reads the mark only; what p answers and what
// its drop does are its own.
func Watch(ctx context.Context, room string, p Participant) (stop func()) {
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		watch(ctx, room, p)
	}()
	return func() { cancel(); <-done }
}

func watch(ctx context.Context, room string, p Participant) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	var joined *state.SwitchMark
	var conn *line
	var said answer
	defer func() {
		if conn != nil {
			_ = conn.conn.Close()
		}
	}()
	for {
		if mark, live := build.LiveSwitch(room); live && mark.Phase == state.PhaseAsking && (joined == nil || !joined.Same(mark)) {
			if conn != nil {
				_ = conn.conn.Close()
			}
			// The mark is published before its leader serves the socket: a
			// join that found nothing listening is tried again next poll.
			joined, conn, said = nil, join(ctx, room, mark, p), answer{}
			if conn != nil {
				joined = &mark
			}
		}
		if conn != nil {
			kind, why := p.Answer()
			if kind != said.Answer || why != said.Why {
				said = hello(p, kind, why)
				if conn.send(said) != nil {
					_ = conn.conn.Close()
					conn = nil
				}
			}
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

// join connects to the attempt's leader and follows its orders; nil when the
// leader does not answer, which leaves this attempt without this process.
func join(ctx context.Context, room string, mark state.SwitchMark, p Participant) *line {
	conn, err := (&net.Dialer{Timeout: time.Second}).DialContext(ctx, "unix", mark.Socket)
	if err != nil {
		return nil
	}
	l := newLine(conn)
	go func() {
		for {
			var o order
			if l.receive(&o) != nil {
				return
			}
			if o.Do != Drop {
				continue
			}
			// The drop names the mark as the leader wrote it on dropping.
			current, err := state.ReadSwitchMark(room)
			if err != nil || !current.Same(mark) {
				_ = l.send(hello(p, Busy, "the switch's mark is gone"))
				continue
			}
			if why := p.Drop(parkOf(room, current)); why != "" {
				_ = l.send(hello(p, Busy, why))
			}
		}
	}()
	return l
}

func hello(p Participant, kind, why string) answer {
	start, _ := proc.StartTime(os.Getpid())
	return answer{Version: state.SwitchVersion, Pid: os.Getpid(), Start: start, Who: p.Who(), Answer: kind, Why: why}
}

// ErrLost is a parked image that can neither follow its switch nor resume:
// the room's build is neither its own nor the switch's target.
var ErrLost = errors.New("the room's build is neither this image's nor its switch's")

// Wait is a parked image waiting for its leader: Go to exec the staged copy,
// Resume to adopt its run again on its own build. A leader that is gone or
// silent leaves it to the room's build: its own is Resume, the target Go —
// the record alone decides, at once, while the mark still keeps launches out
// and the image's lease can be taken before it expires.
func Wait(park Park, who string) (string, error) {
	if decided, ok := ask(park, who); ok {
		return decided, nil
	}
	// A leader still running while it drops may yet commit; one that is gone
	// cannot, and waiting out its mark would let a launch take the room first.
	for {
		mark, live := build.LiveSwitch(park.Room)
		if !live || !build.Draining(mark) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	own, err := build.ID()
	if err != nil {
		return "", err
	}
	theirs, err := state.ReadBuildRecord(park.Room)
	switch {
	case err != nil:
		return "", err
	case theirs == own:
		return Resume, nil
	case theirs == park.Target:
		return Go, nil
	}
	return "", ErrLost
}

// ask says parked to the leader and reads its order, until the drop's
// deadline.
func ask(park Park, who string) (string, bool) {
	conn, err := net.DialTimeout("unix", park.Socket, time.Second)
	if err != nil {
		return "", false
	}
	defer func() { _ = conn.Close() }()
	l := newLine(conn)
	if l.send(hello(CannotHandOver(who, ""), Parked, "")) != nil {
		return "", false
	}
	_ = conn.SetReadDeadline(time.UnixMilli(park.Deadline))
	var o order
	if l.receive(&o) != nil || (o.Do != Go && o.Do != Resume) {
		return "", false
	}
	return o.Do, true
}
