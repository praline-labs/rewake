package gateway

import (
	"context"
	"io"
	"time"
)

// Header and payload share one write gate/deadline. A failed chunk closes the
// stream so another writer cannot append after a partial frame.
func (s *socketClient) writeBytes(ctx context.Context, data []byte, deadline time.Time) error {
	for len(data) > 0 {
		if err := ctx.Err(); err != nil {
			_ = s.conn.Close()
			return err
		}
		n, err := s.conn.Write(data)
		if err != nil || n == 0 {
			_ = s.conn.Close()
			if cause := ctx.Err(); cause != nil {
				return cause
			}
			if !deadline.IsZero() && !time.Now().Before(deadline) {
				return context.DeadlineExceeded
			}
			if err != nil {
				return err
			}
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}
