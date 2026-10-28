package adapter

// MirrorHarness is a harness whose adapter can feed rewake's own face
// (docs/face.md): it hands the model in LaunchRequest.Mirror what its
// harness says and carries the face's inputs back. A session is drawn by
// the face only on a harness offering it.
type MirrorHarness interface {
	// Mirrors marks the offer; LaunchRequest.Mirror turns it on for one run.
	Mirrors()
}
