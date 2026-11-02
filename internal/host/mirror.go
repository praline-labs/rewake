package host

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// startMirror serves the session's conversation to rewake's own face when the
// launch asked for it (docs/face.md): the model is the wrapper's, in memory,
// served on the run's own socket until the run ends, with the count of the
// faces it serves. A harness without the offer is refused, never run without
// what was asked. An image that adopts a run serves a model of its own, which
// says what it does not hold.
func startMirror(request Request, name, epoch string) (*mirror.Model, func() int, func(), error) {
	if !request.Mirror {
		return nil, nil, func() {}, nil
	}
	if _, ok := request.Harness.(adapter.MirrorHarness); !ok {
		return nil, nil, nil, fmt.Errorf("a %s session cannot be drawn by rewake's own face", request.Harness.Title())
	}
	model := mirror.New()
	if h := request.Handover; h != nil {
		model.Apply(mirror.Event{Kind: mirror.EventNotice, ID: fmt.Sprintf("rewake-handover-%d", h.Count), Text: "rewake was updated while the session ran; what came before is in the native face."})
	}
	server, err := mirror.Serve(registry.MirrorFor(request.Dir, name, epoch), model)
	if err != nil {
		return nil, nil, nil, fmt.Errorf("could not serve the session to rewake's face: %w", err)
	}
	return model, server.Faces, func() {
		model.End()
		_ = server.Close()
	}, nil
}
