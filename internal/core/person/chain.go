package person

import (
	"errors"
	"fmt"
	"strconv"
)

// link is one process of the caller's chain as one reading saw it.
type link struct {
	pid, parent int
	start       uint64
	namespaces  string
	// session says the process was started with the session's variable;
	// read only for a process of the caller's own user.
	session bool
}

// maxChain bounds the walk: a chain longer than any machine runs is a loop
// the reads made, not ancestry.
const maxChain = 4096

// chain reads the caller and each of its ancestors up to PID 1. A process is
// never older than its parent, so a parent that started after its child is a
// pid taken again during the walk: the scan starts over. Every process of the
// caller's user shares its pid namespace — an ancestor in another could not be
// seen from here — so one that does not was misread, and the scan starts over
// too.
//
// The initial environment is read for each process of the caller's user; one
// that does not read refuses. A process of another user — PID 1, a login
// daemon, sudo — is read for its real and effective user, positively, and its
// environment, which the kernel keeps from this user, is not required: the
// session's variable and its wrapper reach a chain only through processes of
// the user who started the session, and those are read. Nor are its
// namespaces, whose links the kernel keeps from this user as well: its pid,
// parent and start come from its stat, which every user reads. A session
// started wholly under another user keeps another state directory and is not
// in the selected root (main's decision of October 12, 2026, recorded with
// Q3).
func (s Scan) chain() ([]link, error) {
	var chain []link
	own := ""
	for pid := s.Self; len(chain) < maxChain; {
		l, err := s.read(pid)
		if err != nil {
			return nil, err
		}
		if l.namespaces != "" {
			pidNamespace := pidNamespaceOf(l.namespaces)
			if own == "" {
				own = pidNamespace
			}
			if pidNamespace != own {
				return nil, fmt.Errorf("%w: process %d is in pid namespace %s, not %s", errUnstable, pid, pidNamespace, own)
			}
		}
		if n := len(chain); n > 0 && l.start > chain[n-1].start {
			return nil, fmt.Errorf("%w: process %d started after its child %d", errUnstable, pid, chain[n-1].pid)
		}
		chain = append(chain, l)
		if pid == 1 {
			return chain, nil
		}
		if l.parent <= 0 || l.parent == pid {
			return nil, &UnreadableError{What: "the chain above process " + strconv.Itoa(s.Self), Err: fmt.Errorf("process %d names parent %d and the chain ends short of PID 1", pid, l.parent)}
		}
		pid = l.parent
	}
	return nil, &UnreadableError{What: "the chain above process " + strconv.Itoa(s.Self), Err: errors.New("it does not end")}
}

// read is one process of the chain.
func (s Scan) read(pid int) (link, error) {
	what := "process " + strconv.Itoa(pid)
	parent, start, err := s.Proc.Parent(pid)
	if err != nil {
		return link{}, &UnreadableError{What: "the stat of " + what, Err: err}
	}
	real, effective, err := s.Proc.Owner(pid)
	if err != nil {
		return link{}, &UnreadableError{What: "the user of " + what, Err: err}
	}
	l := link{pid: pid, parent: parent, start: start}
	if real != s.UID && effective != s.UID {
		return l, nil
	}
	if l.namespaces, err = s.Proc.Namespaces(strconv.Itoa(pid)); err != nil {
		return link{}, &UnreadableError{What: "the namespaces of " + what, Err: err}
	}
	if l.session, err = s.Proc.InitialEnv(pid, sessionEnv); err != nil {
		return link{}, &UnreadableError{What: "the initial environment of " + what, Err: err}
	}
	return l, nil
}

// pidNamespaceOf is the pid namespace in proc.Namespaces's answer, its last.
func pidNamespaceOf(namespaces string) string {
	for i := len(namespaces) - 1; i >= 0; i-- {
		if namespaces[i] == ' ' {
			return namespaces[i+1:]
		}
	}
	return namespaces
}
