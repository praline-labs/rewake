package mail

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// A decision authorizes its effects only once it is durable (Q4 of
// docs/v2/stage3-decision-build.md): a link that is visible may still be lost
// when the flush of its directory failed, and an effect made on such a
// decision would outlive the authority it stood on. So before the first
// effect of a decision the installation flushes the decision's file, the
// decisions directory and the mailbox's, under the mailbox lock, and records
// that it did in a write-once marker beside the decision. The marker is the
// evidence the readers that write nothing go by — the disposal of the stop,
// the evidence of the installation, the plan — so none of them acts on a
// decision whose flush has not been seen to succeed. The marker's own flush
// is no condition: it is written only after the decision's flushes
// succeeded, and a marker lost in a crash only has them made again.

// durableSuffix names the marker of a decision made durable.
const durableSuffix = ".durable"

// DurableRecord says the decision named was flushed whole before any effect
// it fixed.
type DurableRecord struct {
	Decision string    `json:"decision"`
	At       time.Time `json:"at"`
}

func parseDurableFile(path string, raw []byte) error {
	_, err := parseDurable(path, raw)
	return err
}

func parseDurable(path string, raw []byte) (DurableRecord, error) {
	var durable DurableRecord
	if err := strictJSON(raw, &durable); err != nil {
		return DurableRecord{}, fmt.Errorf("the record %s of a decision made durable is not readable: %w", path, err)
	}
	if !freshID.MatchString(durable.Decision) || durable.At.IsZero() {
		return DurableRecord{}, fmt.Errorf("the record %s names no decision or time", path)
	}
	return durable, nil
}

// readDurable reads the marker at path, held to the decision d: a marker of
// another decision says nothing of this one.
func readDurable(files fileAccess, path string, d Decision) (DurableRecord, error) {
	raw, err := files.ReadFile(path)
	if err != nil {
		return DurableRecord{}, err
	}
	durable, err := parseDurable(path, raw)
	if err != nil {
		return DurableRecord{}, err
	}
	if durable.Decision != d.ID {
		return DurableRecord{}, fmt.Errorf("the record %s says decision %s was made durable, not %s", path, durable.Decision, d.ID)
	}
	return durable, nil
}

// durable says d, read at path, was made durable: its marker reads and names
// it.
func (w world) durable(path string, d Decision) bool {
	_, err := readDurable(w.files, path+durableSuffix, d)
	return err == nil
}

// makeDurable makes the decision d read at path durable before any effect
// it fixed, unless its marker says it was. A marker that does not read, or
// names another decision, is an unknown and is never written over; a flush
// that fails leaves d with no effect in this pass, an unknown about d. The
// plan flushes nothing: a decision with no marker is one the installation
// before it would make durable.
func (w world) makeDurable(name, path string, d Decision) error {
	marker := path + durableSuffix
	_, err := readDurable(w.files, marker, d)
	if err == nil {
		return nil
	}
	if !errors.Is(err, os.ErrNotExist) {
		return unknownRead(marker, err)
	}
	decisions := filepath.Dir(path)
	for _, flush := range []func() error{
		func() error { return w.syncFile(path) },
		func() error { return w.syncDir(decisions) },
		func() error { return w.syncDir(state.InboxPath(w.dir, name)) },
	} {
		if err := flush(); err != nil {
			return unknownRecord(path, fmt.Errorf("the decision %s could not be made durable, so nothing it fixed is installed yet: %w", path, err))
		}
	}
	raw, err := json.Marshal(DurableRecord{Decision: d.ID, At: time.Now()})
	if err != nil {
		return err
	}
	if err := w.publish(marker, raw); err != nil {
		if errors.Is(err, state.ErrNameTaken) {
			_, err = readDurable(w.files, marker, d)
		}
		if err != nil {
			return unknownRead(marker, fmt.Errorf("the decision %s was flushed and its marker %s could not be written: %w", path, marker, err))
		}
	}
	return nil
}
