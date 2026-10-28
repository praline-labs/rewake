package host

import (
	"fmt"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/mirror"
	"github.com/praline-labs/rewake/internal/core/registry"
)

// startMirror serves the session's conversation to rewake's own face when the
// launch asked for it (docs/face.md): the model is the wrapper's, in memory,
// served on the run's own socket until the run ends. A harness without the
// offer is refused, never run without what was asked.
func startMirror(request Request, name, epoch string) (*mirror.Model, func(), error) {
	if !request.Mirror {
		return nil, func() {}, nil
	}
	if _, ok := request.Harness.(adapter.MirrorHarness); !ok {
		return nil, nil, fmt.Errorf("a %s session cannot be drawn by rewake's own face", request.Harness.Title())
	}
	model := mirror.New()
	server, err := mirror.Serve(registry.MirrorFor(request.Dir, name, epoch), model)
	if err != nil {
		return nil, nil, fmt.Errorf("could not serve the session to rewake's face: %w", err)
	}
	return model, func() {
		model.End()
		_ = server.Close()
	}, nil
}
