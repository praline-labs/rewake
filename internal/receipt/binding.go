package receipt

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/praline-labs/rewake/internal/state"
)

// A call's binding (docs/archive-1.x/mail-bridge-server.md#running-the-child) names the
// record a tool call holds. Every path that holds a record writes it right
// after taking the record's lock and before its first step, so its absence,
// read as "no such file", proves the call made no effect, and its presence
// names the operation whose outcome is then unknown. The server reads it after
// a child died without an answer, and the wrapper's observer reads it to find
// the read a completion settles.

// callsDir holds the bindings of one run's journal.
const callsDir = "calls"

// ErrUnbound says the call holds no record: its binding is proven absent.
var ErrUnbound = errors.New("the call holds no operation")

var callKeyShape = regexp.MustCompile(`^[0-9a-f]{64}$`)

func bindingPath(dir, name, epoch, call string) (string, error) {
	if !callKeyShape.MatchString(call) {
		return "", fmt.Errorf("%q names no call", call)
	}
	journal, err := Path(dir, name, epoch)
	if err != nil {
		return "", err
	}
	return filepath.Join(journal, callsDir, call), nil
}

// Bind records that the call names holds the record token. A native call has
// one ticket and so one binding: a second binding to the same record changes
// nothing, and one to another record is refused.
func Bind(dir, name, epoch, call, token string) error {
	path, err := bindingPath(dir, name, epoch, call)
	if err != nil {
		return err
	}
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	err = state.PublishExclusive(path, []byte(token))
	if !errors.Is(err, state.ErrNameTaken) {
		return err
	}
	bound, err := Bound(dir, name, epoch, call)
	if err != nil {
		return err
	}
	if bound != token {
		return fmt.Errorf("the call is bound to the receipt %s already", bound)
	}
	return nil
}

// Bound answers the record a call holds. ErrUnbound is the proof that it holds
// none; any other error leaves that unknown.
func Bound(dir, name, epoch, call string) (string, error) {
	path, err := bindingPath(dir, name, epoch, call)
	if err != nil {
		return "", err
	}
	raw, err := state.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", ErrUnbound
	}
	if err != nil {
		return "", err
	}
	token := strings.TrimSpace(string(raw))
	if !ValidToken(token) {
		return "", fmt.Errorf("the binding %s names no receipt", path)
	}
	return token, nil
}

// Joined answers the record the key of a scoped call names, if any: the
// operation the same words in the same turn join. An error leaves it unknown.
func Joined(dir, name string, key Key) (string, bool, error) {
	if !key.Scoped() {
		return "", false, nil
	}
	journal, err := Path(dir, name, key.Epoch)
	if err != nil {
		return "", false, err
	}
	raw, err := state.ReadFile(filepath.Join(journal, key.index()))
	if errors.Is(err, os.ErrNotExist) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	return strings.TrimSpace(string(raw)), true, nil
}

// sweepBindings removes the bindings whose record is gone: every one, when
// kept is nil.
func sweepBindings(journal string, kept map[string]bool) {
	directory := filepath.Join(journal, callsDir)
	entries, err := os.ReadDir(directory)
	if err != nil {
		return
	}
	for _, entry := range entries {
		path := filepath.Join(directory, entry.Name())
		if kept != nil {
			raw, err := state.ReadFile(path)
			if err != nil || kept[strings.TrimSpace(string(raw))] {
				continue
			}
		}
		_ = state.Remove(path)
	}
}

// CheckBinding checks the content of one binding, which names its receipt.
func CheckBinding(path string, raw []byte) error {
	if !callKeyShape.MatchString(filepath.Base(path)) || !ValidToken(strings.TrimSpace(string(raw))) {
		return fmt.Errorf("the call binding %s names no receipt", path)
	}
	return nil
}
