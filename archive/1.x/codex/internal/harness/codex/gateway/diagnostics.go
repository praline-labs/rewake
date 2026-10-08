package gateway

import (
	"errors"
	"io"
	"net"
)

// CloseInfo contains only local counters and fixed reason codes, never RPC values.
type CloseInfo struct {
	Connection, Generation     uint64
	Direction, Reason, Error   string
	Bytes, Requests, Responses int
	SizeStage                  string
	MessageBytes, LimitBytes   uint64
}

type sizeError struct {
	Stage       string
	Size, Limit uint64
}

func (*sizeError) Error() string { return "websocket message too large" }

func closeError(err error) string {
	if err == nil {
		return ""
	}
	if errors.Is(err, io.EOF) {
		return "peer-eof"
	}
	if errors.Is(err, net.ErrClosed) {
		return "socket-closed"
	}
	var network net.Error
	if errors.As(err, &network) && network.Timeout() {
		return "timeout"
	}
	// Unknown errors may embed paths or payload values. Only these local constants
	// are safe to copy verbatim into a persistent diagnostic.
	switch err.Error() {
	case "invalid RPC object", "duplicate routing field", "request id too long",
		"unsupported request id", "websocket message too large", "invalid server websocket frame",
		"invalid websocket control frame", "nested websocket message", "unexpected websocket continuation",
		"invalid websocket text", "duplicate outstanding TUI request id", "pending TUI request limit",
		"manual-scope capacity reached; control not forwarded":
		return err.Error()
	}
	return "transport-or-control-error"
}
