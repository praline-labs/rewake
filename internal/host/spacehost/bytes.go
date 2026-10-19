package spacehost

import (
	"net"
	"time"
)

// bytes serves one client of the raw stream until it leaves or the worker's
// output ends: the accepting byte, the grid's preamble, then every byte the
// grid's emulator takes after it, unframed (docs/v2/shell.md). The client
// writes the frames a view does. A client that stops reading, or falls too
// far behind for the grid to keep its bytes, is dropped and attaches again.
func (r *relay) bytes(conn net.Conn) {
	defer func() { _ = conn.Close() }()
	stream := r.grid.Attach()
	defer stream.Detach()
	go func() {
		r.frames(conn)
		// The client left: its stream ends, which ends the loop below.
		stream.Detach()
	}()
	_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
	if _, err := conn.Write(append([]byte{accepted}, stream.Preamble...)); err != nil {
		return
	}
	for p := range stream.Live {
		_ = conn.SetWriteDeadline(time.Now().Add(clientWrite))
		if _, err := conn.Write(p); err != nil {
			return
		}
	}
}
