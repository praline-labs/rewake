package delivery

import (
	"context"
	"crypto/sha256"
	"fmt"

	"github.com/praline-labs/rewake/internal/core/mail"
)

// announce remembers when a letter's notice was handed to the harness, for the
// turn end that looks at what its turn left unread.
func (s *Server) announce(id string, at int64) {
	if s.announced == nil {
		s.announced = map[string]int64{}
	}
	if at != 0 {
		s.announced[id] = at
	}
}

// remind sends one notice of the letters asking for work that the finished
// turn ending at ended left unread (mail.Box.Unattended), through the same
// Wake as any notice. It writes no status: the letters were announced already,
// and a reminder that does not go is not tried again. Every announcement up to
// ended is forgotten after the look: a later turn's window opens after this
// end, so no letter is reminded twice, a retry of the same end included. The
// latest end looked at is kept for a release heard after it (receive).
func (s *Server) remind(ctx context.Context, ended int64) {
	s.looked = max(s.looked, ended)
	var members []mail.Message
	err := s.lockOrAlone(s.lockCtx(), func() error {
		var err error
		members, err = s.box().Unattended(ended, func(id string) (int64, bool) {
			at, ok := s.announced[id]
			return at, ok
		})
		return err
	})
	for id, at := range s.announced {
		if at <= ended {
			delete(s.announced, id)
		}
	}
	if err != nil || len(members) == 0 || s.Deliver == nil || !s.owned() {
		return
	}
	if !s.validAnnouncement(s.lockCtx(), members) {
		return
	}
	s.Deliver(ctx, reminder(members))
}

// reminder is the notice of members as a reminder, under an id of its own: a
// late word about it names no notice this server tracks, and changes nothing.
// Its members are named even when there is one, since the id is not a
// letter's.
func reminder(members []mail.Message) mail.Message {
	message := announcement(members)
	message.Batch = members
	hash := sha256.New()
	for _, member := range members {
		_, _ = fmt.Fprintf(hash, "%s\x00", member.ID)
	}
	message.ID = fmt.Sprintf("reminder-%x", hash.Sum(nil))
	message.Reminder = true
	return message
}
