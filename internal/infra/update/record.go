package update

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/infra/parts"
	"github.com/praline-labs/rewake/internal/infra/state"
)

// Every is how often the registry is asked again: main's host asks at its
// start when the record is older, then every Every while it runs.
const Every = 6 * time.Hour

// Record is the last check, kept under the state root by
// state.WriteUpdateRecord. Latest, Manifest and Changelog are the last good
// answer, kept through a failed check, which sets Error.
type Record struct {
	Checked time.Time `json:"checked"`
	// Latest is the release npm's latest names.
	Latest string `json:"latest,omitempty"`
	// Manifest is Latest's manifest for this platform, Changelog its
	// changelog.
	Manifest  *parts.Manifest `json:"manifest,omitempty"`
	Changelog []Release       `json:"changelog,omitempty"`
	// Error is why the last check failed; empty when it did not.
	Error string `json:"error,omitempty"`
}

// ReadRecord is root's record; one missing or unreadable is none, which the
// next check writes again.
func ReadRecord(root string) Record {
	data, err := os.ReadFile(filepath.Join(root, state.UpdateRecordName))
	if err != nil {
		return Record{}
	}
	var r Record
	if json.Unmarshal(data, &r) != nil {
		return Record{}
	}
	return r
}

// WriteRecord replaces root's record.
func WriteRecord(root string, r Record) error {
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		return err
	}
	return state.WriteUpdateRecord(root, append(data, '\n'))
}

// Due says the record is older than Every, or was never written.
func (r Record) Due(now time.Time) bool {
	return r.Checked.IsZero() || now.Sub(r.Checked) >= Every || now.Before(r.Checked)
}
