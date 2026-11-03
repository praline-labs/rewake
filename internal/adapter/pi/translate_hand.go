package pi

import (
	"encoding/json"
	"errors"
	"io"
	"os"

	"github.com/praline-labs/rewake/internal/infra/memfile"
)

// hand puts what the translator holds in a memory file for the next image.
func (t *translator) hand() (*os.File, error) {
	t.mu.Lock()
	raw, err := json.Marshal(t)
	t.mu.Unlock()
	if err != nil {
		return nil, err
	}
	return memfile.New("rewake-pi-mirror", raw)
}

// adoptTranslator reads what an earlier image's translator held, and closes
// its file.
func adoptTranslator(file *os.File) (*translator, error) {
	defer func() { _ = file.Close() }()
	raw, err := io.ReadAll(io.LimitReader(file, maxTranslator+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxTranslator {
		return nil, errors.New("what was handed over is past its bound")
	}
	t := newTranslator()
	if err := json.Unmarshal(raw, t); err != nil {
		return nil, err
	}
	return t, nil
}

// maxTranslator bounds what a translator hands over: its open texts are
// the stream's, a long event's parts and the calls' streamed input among
// them, each within an event's bound.
const maxTranslator = 64 << 20
