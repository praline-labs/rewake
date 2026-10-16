// Package modhost is the wrapper's end of the mod's protocol: an HTTP server on
// the run's mod socket that checks each request by its peer and turns it into
// the adapter API's calls (docs/v2/stage4-space.md#2-the-protocol). Its types
// are generated from the protocol's schema by tools/protogen, so the mod and
// this package cannot drift apart. Stage 4's step S4-5 serves hello, S4-6 the
// turns, mod/ended and the heartbeat; the other endpoints answer that they are
// not in this build until their steps.
package modhost
