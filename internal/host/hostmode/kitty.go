package hostmode

import (
	"bytes"
	"strconv"
)

// The kitty keyboard protocol's key events, as the person's terminal sends
// them while the shown program asked for the protocol: CSI code[:alternates]
// [; modifiers[:event] [; text]] u, the modifiers one more than their bits.
const (
	modCtrl      = 4
	modLocks     = 64 | 128 // Caps Lock and Num Lock, reported when on
	eventPress   = 1
	eventRelease = 3
)

// kittyKey is one key event in the CSI u form.
type kittyKey struct {
	code, mods, event int
}

// parseKitty reads a key event in the CSI u form; the modifiers are their
// bits, the lock keys taken off.
func parseKitty(token []byte) (kittyKey, bool) {
	if len(token) < 4 || !bytes.HasPrefix(token, []byte("\x1b[")) || token[len(token)-1] != 'u' {
		return kittyKey{}, false
	}
	fields := bytes.Split(token[2:len(token)-1], []byte(";"))
	code, ok := number(bytes.SplitN(fields[0], []byte(":"), 2)[0], -1)
	if !ok {
		return kittyKey{}, false
	}
	k := kittyKey{code: code, mods: 1, event: eventPress}
	if len(fields) > 1 {
		parts := bytes.SplitN(fields[1], []byte(":"), 2)
		if k.mods, ok = number(parts[0], 1); !ok {
			return kittyKey{}, false
		}
		if len(parts) > 1 {
			if k.event, ok = number(parts[1], eventPress); !ok {
				return kittyKey{}, false
			}
		}
	}
	k.mods = (k.mods - 1) &^ modLocks
	return k, true
}

// number reads a decimal parameter, def when it is empty; -1 as def makes
// it required.
func number(p []byte, def int) (int, bool) {
	if len(p) == 0 {
		return def, def >= 0
	}
	n, err := strconv.Atoi(string(p))
	return n, err == nil && n >= 0
}

// keyRelease says the token is a key's release, which the protocol's event
// types report in the CSI u form and in the CSI forms of the other keys
// (CSI 1;5:3A). A release says nothing to the host.
func keyRelease(token []byte) bool {
	if len(token) < 4 || !bytes.HasPrefix(token, []byte("\x1b[")) || token[2] < '0' || token[2] > '9' {
		return false
	}
	fields := bytes.Split(token[2:len(token)-1], []byte(";"))
	if len(fields) < 2 {
		return false
	}
	parts := bytes.SplitN(fields[1], []byte(":"), 2)
	return len(parts) == 2 && string(parts[1]) == strconv.Itoa(eventRelease)
}
