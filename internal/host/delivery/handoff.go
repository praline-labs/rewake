package delivery

import "github.com/praline-labs/rewake/internal/core/mail"

// A wrapper about to exec a new image of itself asks its server to stop
// (Server.Handoff), the probe of .scratch/reexec-probe.md. The server stops
// only when nothing it holds lives in memory alone — a held notice, one a
// late word may still take back, a letter announced and not yet looked at
// by a turn's end, a letter of this run still waiting — and then returns
// without refusing anything: the next image serves the same mailbox from
// disk, as a server's begin always does. Otherwise it answers why and serves
// on.

// busy answers what this server holds that the disk does not, "" when
// nothing.
func (s *Server) busy() string {
	switch {
	case len(s.held) > 0:
		return "the harness holds a notice whose receipt only this image would take"
	case len(s.recent) > 0:
		return "a notice reported delivered may still be taken back by a late word"
	case len(s.announced) > 0:
		return "a letter announced is not yet looked at by a turn's end"
	}
	messages, err := mail.List(s.Dir, s.Name)
	if err != nil {
		return "the mailbox could not be read: " + err.Error()
	}
	for _, message := range messages {
		if s.Epoch == "" || message.ToEpoch == s.Epoch {
			return "a letter waits for its delivery"
		}
	}
	return ""
}

// handoff answers a request to stop: "" stops the server, anything else is
// why it serves on.
func (s *Server) handoff(reply chan<- string) bool {
	why := s.busy()
	reply <- why
	return why == ""
}
