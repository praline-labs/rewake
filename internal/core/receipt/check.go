package receipt

import (
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
)

// The mailbox's list of record kinds (inbox/records.go) checks the files kept
// here by these two, since this package knows what they say; the mailbox
// reads them itself, through its one seam. Each answers an error when the
// content does not read as what its place says it is.

// CheckRecord checks the content of one receipt at path as Load would read it.
func CheckRecord(path string, raw []byte) error {
	var record Record
	if err := json.Unmarshal(raw, &record); err != nil {
		return fmt.Errorf("the receipt %s is not readable: %w", path, err)
	}
	token := strings.TrimSuffix(filepath.Base(path), ".json")
	if record.Version != version || record.Token != token || record.Epoch != filepath.Base(filepath.Dir(path)) {
		return fmt.Errorf("the receipt %s does not say what its place says", path)
	}
	return nil
}

// CheckKey checks the content of one key of a scoped call, which names its
// receipt.
func CheckKey(path string, raw []byte) error {
	if !ValidToken(strings.TrimSpace(string(raw))) {
		return fmt.Errorf("the call key %s names no receipt", path)
	}
	return nil
}
