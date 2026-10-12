package upgraderig

import (
	"fmt"
	"sort"
)

// expectation is what one launch of a world ends in, uninterrupted, read
// from the protocol's two parts and its table of states rather than from the
// code: whether it goes on, what it says, and which trees it leaves.
type expectation struct {
	end   string
	says  []string
	trees []string
}

// atAdmission are the conditions admission finds, with what the refusal says.
var atAdmission = map[string]string{
	"live wrapper":      "a rewake 1.x session runs: web in room old",
	"launch lock held":  "old/.launch.lock",
	"name lock held":    "sessions/.web.lock",
	"mailbox lock held": "inbox/web/.lock",
	"unreadable record": "web.json cannot be checked",
	"other namespace":   "pid namespace pid:[1]",
	// The wrapper's /proc data, from the tree the world's launches read
	// instead of /proc.
	"unreadable process data": "/stat cannot be read",
	"malformed process data":  "/stat cannot be read",
}

// atValidation are the conditions validation finds in the tree they arrived in.
var atValidation = map[string]string{
	"late live room":         "a rewake 1.x session runs: late in room new",
	"late unreadable record": "late.json cannot be checked",
	"late lock":              "sessions/.late.lock",
}

// treeCounts is what a tree of a world holds, as the summary counts it.
type treeCounts struct{ rooms, unread int }

func expect(s scenario) expectation {
	var entry []string
	if s.hasRooms() {
		entry = append(entry, "rooms")
	}
	if s.hasCandidate() {
		entry = append(entry, "candidate")
	}
	if s.hasValidated() {
		entry = append(entry, "validated")
	}
	if says, ok := atAdmission[s.old]; ok {
		return expectation{end: refused, says: []string{says}, trees: sorted(entry)}
	}

	var left []string
	var removed []treeCounts
	deferred, failed := false, ""
	failOnce := s.old == "delete fails"
	// What the launch says it removed and deferred, on every end after
	// admission: beside a refusal too.
	summary := func(says ...string) []string {
		if len(removed) > 0 {
			total := treeCounts{}
			for _, c := range removed {
				total.rooms, total.unread = total.rooms+c.rooms, total.unread+c.unread
			}
			says = append(says, fmt.Sprintf("removed the state of rewake 1.x (%s, %s, 0 undelivered reports)",
				plural(total.rooms, "room"), plural(total.unread, "unread letter")))
		}
		if deferred {
			says = append(says, "1 validated tree of rewake 1.x could not be deleted yet")
		}
		return says
	}
	// Step 4: a validated tree is deleted after admission, its counts the
	// ones recorded before, once the base is flushed: a validated name
	// proves no flush.
	if s.hasValidated() && s.old == "sync fails" {
		return expectation{end: refused, says: []string{"so no validated tree of rewake 1.x was deleted"}, trees: sorted(entry)}
	}
	if s.hasValidated() {
		if failOnce {
			failOnce, deferred = false, true
			left = append(left, "validated")
		} else {
			removed = append(removed, treeCounts{3, 3})
		}
	}
	// Step 5.
	type tree struct {
		counts   treeCounts
		evidence string
	}
	var validate []tree
	if s.hasRooms() {
		switch s.old {
		case "rename fails":
			return expectation{end: refused, says: summary("aside, so nothing in it was removed"), trees: sorted(append(left, entryOf(s, "rooms")...))}
		case "sync fails":
			return expectation{end: refused, says: summary("aside, so nothing in it was removed"), trees: sorted(append(left, entryOf(s, "candidate")...))}
		}
		own := tree{counts: treeCounts{1, 1}, evidence: atValidation[s.old]}
		switch s.old {
		case "late bare room":
			own.counts.rooms++
		case "late replaced lock":
			own.evidence = "is not the file this launch locked"
		}
		validate = append(validate, own)
	}
	if s.hasCandidate() {
		c := tree{counts: treeCounts{1, 1}}
		if !s.hasRooms() {
			c.evidence = atValidation[s.old]
			if s.old == "late bare room" {
				c.counts.rooms++
			}
		}
		validate = append(validate, c)
	}
	// Step 6, with a rooms/ recreated meanwhile.
	roomsThere := s.old == "recreated bare" || s.old == "recreated live"
	outside := ""
	if s.old == "recreated live" {
		outside = "a rewake 1.x session runs: web in room x"
	}
	if roomsThere {
		left = append(left, "rooms")
	}
	says := outside
	for _, t := range validate {
		evidence := t.evidence
		if outside != "" {
			evidence = outside
		}
		if evidence != "" {
			if says == "" {
				says = evidence
			}
			if roomsThere {
				left = append(left, "candidate")
			} else {
				left, roomsThere = append(left, "rooms"), true
			}
			continue
		}
		switch s.old {
		case "rename fails":
			failed = "validated, so nothing in it was removed"
			left = append(left, "candidate")
			continue
		case "sync fails":
			failed = "validated, so nothing in it was removed"
			left = append(left, "validated")
			continue
		}
		if failOnce {
			failOnce, deferred = false, true
			left = append(left, "validated")
			continue
		}
		removed = append(removed, t.counts)
	}
	switch {
	case says != "":
		return expectation{end: refused, says: summary(says), trees: sorted(left)}
	case failed != "":
		return expectation{end: refused, says: summary(failed), trees: sorted(left)}
	}
	return expectation{end: went, says: summary(), trees: sorted(left)}
}

// entryOf is the trees a refusal at set-aside leaves: rooms/ as it was, or
// the candidate it became, beside a candidate the world started with.
func entryOf(s scenario, own string) []string {
	trees := []string{own}
	if s.hasCandidate() {
		trees = append(trees, "candidate")
	}
	return trees
}

func plural(n int, one string) string {
	if n == 1 {
		return "1 " + one
	}
	return fmt.Sprintf("%d %ss", n, one)
}

func sorted(trees []string) []string {
	out := append([]string(nil), trees...)
	sort.Strings(out)
	return out
}

// persists says whether a condition is still there for the next launch, once
// it arrived: live state and unreadable records stay; a replaced lock, a bare
// room and an injected failure do not.
func persists(s scenario) bool {
	if _, ok := atAdmission[s.old]; ok {
		return true
	}
	_, ok := atValidation[s.old]
	return ok || s.old == "recreated live"
}
