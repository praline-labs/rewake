// Package modhost is the wrapper's end of the mod's protocol: an HTTP server on
// the run's mod socket that checks each request by its peer and turns it into
// the adapter API's calls (docs/v2/stage4-space.md#2-the-protocol). Its types
// are generated from the protocol's schema by tools/protogen, so the mod and
// this package cannot drift apart. It serves hello, the turns, mod/ended and
// the heartbeat; the keeper and the screen of the space left the protocol on
// October 18, 2026.
package modhost
