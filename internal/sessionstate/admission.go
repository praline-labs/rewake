package sessionstate

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"syscall"

	"github.com/praline-labs/rewake/internal/state"
)

// A run that asked to resume a conversation keeps its mail closed until that
// conversation is resumed or the person accepts the one selected
// (docs/delivery-conversation.md). The snapshot cannot say so: it is published
// on a tick and reads as "no hold" when missing. So the wrapper writes this
// record itself, before the terminal starts, and rewrites it before the hold
// ends; rewake inbox refuses while it says the mail is held, and when it
// cannot be read. It changes once, from held to admitted, so a reader never
// sees an admission the wrapper takes back.
type admission struct {
	Held   bool   `json:"held"`
	Detail string `json:"detail,omitempty"`
}

const maxAdmissionBytes = 4 << 10

func admissionPath(dir, name, epoch string) string {
	sum := sha256.Sum256([]byte(name + "\x00" + epoch))
	return filepath.Join(dir, "admission", fmt.Sprintf("%x.json", sum[:]))
}

// HoldMail closes the run's mail until AdmitMail, saying why in detail.
func HoldMail(dir, name, epoch, detail string) error {
	return saveAdmission(dir, name, epoch, admission{Held: true, Detail: detail})
}

// AdmitMail opens the run's mail for good.
func AdmitMail(dir, name, epoch string) error {
	return saveAdmission(dir, name, epoch, admission{})
}

func saveAdmission(dir, name, epoch string, record admission) error {
	if !state.ValidName(name) || epoch == "" {
		return fmt.Errorf("invalid admission identity")
	}
	record.Detail = cut(record.Detail, 2<<10)
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	path := admissionPath(dir, name, epoch)
	if err := state.EnsureSubdir(filepath.Dir(path)); err != nil {
		return err
	}
	return state.WriteAtomic(path, raw)
}

// MailHeld says whether the run's mail is closed, and why. A run that never
// asked to resume has no record, and its mail is open; a record that cannot
// be read is an error, never an admission.
func MailHeld(dir, name, epoch string) (held bool, detail string, err error) {
	if !state.ValidName(name) || epoch == "" {
		return false, "", fmt.Errorf("invalid admission identity")
	}
	path := admissionPath(dir, name, epoch)
	if _, err := os.Lstat(filepath.Dir(path)); errors.Is(err, os.ErrNotExist) {
		return false, "", nil
	}
	if err := state.Verify(filepath.Dir(path)); err != nil {
		return false, "", err
	}
	fd, err := syscall.Open(path, syscall.O_RDONLY|syscall.O_NOFOLLOW|syscall.O_NONBLOCK|syscall.O_CLOEXEC, 0)
	if errors.Is(err, syscall.ENOENT) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	file := os.NewFile(uintptr(fd), path)
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	if err != nil {
		return false, "", err
	}
	if !info.Mode().IsRegular() || info.Size() > maxAdmissionBytes {
		return false, "", fmt.Errorf("%s is not an admission record", path)
	}
	raw, err := io.ReadAll(io.LimitReader(file, maxAdmissionBytes+1))
	if err != nil {
		return false, "", err
	}
	var record admission
	if err := json.Unmarshal(raw, &record); err != nil {
		return false, "", fmt.Errorf("%s: %w", path, err)
	}
	return record.Held, record.Detail, nil
}
