package gateway

import "sync/atomic"

// Startup issues concurrent metadata reads and receives notification bursts.
// Keep the readers independent of writes/admission while bounding retained bytes,
// including spare slice capacity and the message currently being processed.
// One maximum-size reply can coexist with the former small-message byte budget;
// an item limit alone would allow hundreds of large frames behind a stalled peer.
const (
	transportQueueItems = 256
	transportQueueBytes = maxMessage + (16 << 20)
)

func retainBytes(counter *atomic.Int64, size int) bool {
	if counter.Add(int64(size)) <= transportQueueBytes {
		return true
	}
	counter.Add(-int64(size))
	return false
}

func (c *connection) queueRequest(raw []byte, run func()) bool {
	if !retainBytes(&c.requestBytes, cap(raw)) {
		return false
	}
	select {
	case c.work <- func() {
		defer c.requestBytes.Add(-int64(cap(raw)))
		run()
	}:
		return true
	default:
		c.requestBytes.Add(-int64(cap(raw)))
		return false
	}
}

func (c *connection) queueResponse(raw []byte) bool {
	if !retainBytes(&c.responseBytes, cap(raw)) {
		return false
	}
	select {
	case c.toUI <- raw:
		return true
	default:
		c.responseBytes.Add(-int64(cap(raw)))
		return false
	}
}
