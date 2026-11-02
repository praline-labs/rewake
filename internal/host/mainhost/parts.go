package mainhost

import (
	"fmt"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
)

// reloadWithin: a shell asked to reload a part answers within it; an older
// shell, which takes no reload, never does.
const reloadWithin = 2 * time.Second

// digest takes the shell's digest and wakes the loop, which publishes why it
// cannot be handed over only after an event: the digest taken is none, and an
// idle host went on saying its shell's parts were being read.
func (h *Host) digest(p *shellProc) {
	p.takeDigest(h.cfg.Shell)
	h.post(ev{kind: kDigested})
}

// takeDigest takes the digest of the shell started from path, with the
// manifest's function: its sources' when it runs from a checkout, else its
// file's, completed by the installed manifest's digest when that manifest
// describes the file. The match gives a digest only; the contracts are what
// the shell said in its hello (docs/install.md#the-running-set).
func (p *shellProc) takeDigest(path string) {
	defer close(p.digested)
	digest, file, err := parts.ShellDigest(path)
	if err != nil {
		return
	}
	if digest == "" {
		if m, err := parts.Read(parts.InstalledDir()); err == nil {
			if installed, ok := m.Find(parts.ShellPart); ok && installed.File == file {
				digest = installed.Digest
			}
		}
	}
	p.mu.Lock()
	p.digest, p.file = digest, file
	p.mu.Unlock()
}

// took keeps the contracts a shell's hello said; a hello without them
// leaves requires nil, which the plan never takes for compatible.
func (p *shellProc) took(hello Hello) {
	if hello.Contracts == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.requires, p.face = hello.Contracts.Requires, hello.Contracts.Face
	if p.requires == nil {
		p.requires = map[string]string{}
	}
}

// parts is the shell as it runs, once its digest is taken, and the face
// compiled into it when it said one.
func (p *shellProc) parts() []parts.Part {
	select {
	case <-p.digested:
	case <-time.After(doorWait):
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	out := []parts.Part{{Name: parts.ShellPart, Class: parts.Shell, Digest: p.digest, File: p.file, Requires: p.requires}}
	if p.face != nil {
		out = append(out, parts.Part{Name: parts.Face, Class: parts.Shell, Within: parts.ShellPart, Speaks: p.face.Speaks, Requires: p.face.Requires})
	}
	return out
}

// running answers the door's parts: the shell drawing now, if one does.
func (h *Host) running(reply chan Message) {
	p := h.shell
	if p == nil || !p.ready {
		answer(reply, Message{T: kindParts, Running: []parts.Part{}})
		return
	}
	go func() { reply <- Message{T: kindParts, Running: p.parts()} }()
}

// reload asks the drawing shell to reload a part in place, as its own watch
// does; the face is the one part that reloads so.
func (h *Host) reload(part string, reply chan Message) {
	if h.phase != running || h.shell == nil || !h.shell.ready {
		why := h.whyNot()
		if h.phase == alone {
			why = "no shell draws: main runs alone; rewake restart shell brings one back"
		}
		answer(reply, Message{T: kindFailed, Why: why})
		return
	}
	if reply != nil {
		h.reloads = append(h.reloads, reply)
	}
	h.shell.send(Message{T: kindReload, Part: part})
	h.after(reloadWithin, ev{kind: kReloadLate, gen: h.shell.gen})
}

// reloaded answers the reloads waiting.
func (h *Host) reloaded(msg Message) {
	for _, reply := range h.reloads {
		answer(reply, msg)
	}
	h.reloads = nil
}

// reloadLate fails the reloads still waiting.
func (h *Host) reloadLate() {
	h.reloaded(Message{T: kindFailed, Why: fmt.Sprintf("the shell did not answer the reload within %s: an older shell takes none, and rewake restart shell starts one that does", reloadWithin)})
}
