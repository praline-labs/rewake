package inbox

import (
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/praline-labs/rewake/internal/state"
)

// settleRecorded settles the letters a recorded outcome still owes that no
// other pass reads: a refused letter whose waiting copy is gone and whose
// readable copy is left in unread/. A refusal that comes after the notice was
// reported delivered leaves one behind whenever its settling stops short —
// the lock could not be taken, an open stop names the letter, its archive
// could not be written, or the run that recorded it ended first. A letter with
// a waiting copy is the waiting passes' (pendingMessages).
//
// The status and the copies are the whole record of that debt, so it is read
// from them on every pass rather than remembered: a restart loses none of it,
// and a settling that stopped short is found again until it is done. Only a
// failed status with no report made available owes the archive; every other
// outcome leaves the letter readable on purpose — held, delivered, an
// available report, a withdrawal's tombstone, a letter being read in parts —
// and a status that cannot be read is no outcome at all. Settling only, never
// telling: whoever had to hear of the refusal was told when it happened.
func (s *Server) settleRecorded() {
	entries, err := os.ReadDir(state.UnreadPath(s.Dir, s.Name))
	if err != nil {
		return
	}
	for _, entry := range entries {
		id, letter := strings.CutSuffix(entry.Name(), ".json")
		if !letter || strings.HasPrefix(id, ".") || !entry.Type().IsRegular() {
			continue
		}
		if !s.owned() {
			return
		}
		if last, tried := s.attempts[id]; tried && time.Since(last) < retryInterval && !s.stopping {
			continue
		}
		if !gone(filepath.Join(state.InboxPath(s.Dir, s.Name), id+".json")) {
			continue
		}
		s.settleUnreadOnly(id)
	}
}

// settleUnreadOnly settles one letter left only in unread/. An outcome this
// run holds and its status does not say yet is published, status first, the
// way it was meant to be; otherwise the status decides, and is read again
// under the lock that the settling takes. A letter it settles is forgotten;
// one it leaves is tried again once the retry interval has passed.
func (s *Server) settleUnreadOnly(id string) {
	status, known, err := ReadStatus(s.Dir, s.Name, id)
	if err != nil {
		s.attempts[id] = time.Now()
		return
	}
	if known && status.final() {
		return
	}
	if result, remembered := s.outcomes[id]; remembered && result.State != Held && result.State != Pending && (!known || status.State != result.State) {
		if s.publish(id, result) {
			delete(s.attempts, id)
		}
		return
	}
	if !known || !owesArchive(status) || claimed(s.Dir, s.Name, id) {
		return
	}
	s.attempts[id] = time.Now()
	settled := false
	_ = s.lock(func() error {
		status, known, err := ReadStatus(s.Dir, s.Name, id)
		if err != nil {
			return err
		}
		if !known || !owesArchive(status) {
			settled = true
			return nil
		}
		settled = settle(s.Dir, s.Name, id, Failed)
		return nil
	})
	if settled {
		delete(s.attempts, id)
	}
}

// owesArchive says whether a status settles its letter into done/: a refusal,
// neither withdrawn — its tombstone stays to be read — nor a report left
// readable for its recipient.
func owesArchive(status Status) bool {
	return status.State == Failed && !status.Withdrawn && !status.ReportAvailable
}
