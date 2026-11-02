package host

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// startMirror serves the session's conversation to rewake's own face when the
// launch asked for it (docs/face.md): the model is the wrapper's, in memory,
// served on the run's own socket until the run ends. A harness without the
// offer is refused, never run without what was asked. An image that adopts a
// run serves the model the last image handed over, which a face reconnects
// to; one it cannot read is replaced by a model that says what it lost.
func startMirror(request Request, name, epoch string) (*mirror.Model, func(), error) {
	if !request.Mirror {
		return nil, func() {}, nil
	}
	if _, ok := request.Harness.(adapter.MirrorHarness); !ok {
		return nil, nil, fmt.Errorf("a %s session cannot be drawn by rewake's own face", request.Harness.Title())
	}
	model := mirror.New()
	if h := request.Handover; h != nil {
		adopted, err := adoptModel(h)
		if err != nil {
			model.Apply(mirror.Event{Kind: mirror.EventNotice, ID: fmt.Sprintf("rewake-handover-%d", h.Count), Text: "rewake was updated while the session ran, and what came before could not be kept (" + err.Error() + "); the native face has it."})
		} else {
			model = adopted
		}
	}
	server, err := mirror.Serve(registry.MirrorFor(request.Dir, name, epoch), model)
	if err != nil {
		return nil, nil, fmt.Errorf("could not serve the session to rewake's face: %w", err)
	}
	return model, func() {
		model.End()
		_ = server.Close()
	}, nil
}

// handModel puts the model in a memory file for the next image.
func handModel(model *mirror.Model) (*os.File, error) {
	raw, err := model.Hand()
	if err != nil {
		return nil, err
	}
	return memoryFile("rewake-model", raw)
}

// adoptModel reads the model an earlier image handed over, and closes its file.
func adoptModel(h *Handover) (*mirror.Model, error) {
	if h.model == nil {
		return nil, errors.New("no model was handed over")
	}
	defer func() { _ = h.model.Close() }()
	raw, err := io.ReadAll(h.model)
	if err != nil {
		return nil, err
	}
	return mirror.Adopted(raw)
}
