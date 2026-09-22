package workflow

// What a session on this column writes down, and how it reads its mail.
//
// The records are the same files the other column writes, so a scenario reads
// one shape whichever fixture produced it. Two fields differ and the
// difference is the point: a delivery here names how many messages are
// waiting and an id for the announcement, never the messages themselves. A
// scenario that needs the member ids is asking this column for something the
// adapter does not send.

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// recordDelivery writes one notification down as a delivery: which one it was,
// how many messages it announced, the notice as it arrived, and the id the
// announcement carried. The members column is empty here — that is what this
// column cannot say.
func (s *claudeSession) recordDelivery(turn string, notice claudeNotice) {
	if target := os.Getenv(shimGroupsFile); target != "" {
		appendLine(target, fmt.Sprintf("%s\t%d\t%s\t%s\t%s", turn, notice.Count, "", strconv.Quote(notice.Summary), notice.TaskID))
	}
	// The delivered record keeps the announcement's id. A report says which
	// messages it settles; on this column the only thing the delivery named is
	// this id, and a scenario correlating the two has to work from it.
	if target := os.Getenv(shimDeliveredFile); target != "" {
		named := notice.TaskID
		if os.Getenv(shimWrongReportID) != "" {
			// The delivery was real; what the scenario is told about it is
			// not — the same control the other column runs.
			named = "rewake-00000000"
		}
		appendLine(target, named)
	}
}

// readEachAnnounced reads the mail the way the grouped-inbox contract
// describes: an overview first, then one member at a time, leaving the last
// member of a group unread until the next delivery.
//
// The order comes from the overview rather than from the notice, because the
// notice on this column does not name its members. That is the same set the
// other column computes from the notice plus what it deferred — an overview
// lists exactly the mail that is still available — so the two columns read in
// the same order for the same reasons.
func readEachAnnounced(announced int) (string, error) {
	session := &shimSession{}
	if err := insideACase(); err != nil {
		return "", err
	}
	var summary []string
	ids, err := session.peekOverview()
	if err != nil {
		session.recordRead("peek\t0\t\trefused\t" + firstLine(err.Error()))
		return "read-each: peek refused", nil
	}
	session.recordRead(fmt.Sprintf("peek\t%d\t%s\tok\t", len(ids), strings.Join(ids, ",")))
	summary = append(summary, fmt.Sprintf("peek %d", len(ids)))

	// A group leaves its last member for the next delivery; a single message
	// is read whole. Without that, nothing would ever be announced and unread
	// at the same time, and the no-replay observation would have nothing to
	// contradict.
	if announced > 1 && len(ids) > 1 {
		session.recordRead("deferred\t" + ids[len(ids)-1] + "\t\t")
		ids = ids[:len(ids)-1]
	}
	for _, id := range ids {
		count, err := session.readOne(id)
		if err != nil {
			session.recordRead(fmt.Sprintf("message\t%s\trefused\t%s", id, firstLine(err.Error())))
			summary = append(summary, id+" refused")
			continue
		}
		session.recordRead(fmt.Sprintf("message\t%s\tok\t%d", id, count))
		summary = append(summary, id+" ok")
	}
	return "read-each: " + strings.Join(summary, ", "), nil
}
