package mirror

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"time"

	"github.com/praline-labs/rewake/internal/infra/state"
)

// Server serves a model to the faces on the session's mirror socket, any
// process of the user, as the space socket is (docs/face.md).
type Server struct {
	model    *Model
	listener *state.Listener
	ctx      context.Context
	cancel   context.CancelFunc
	mu       sync.Mutex
	conns    map[net.Conn]struct{}
	wg       sync.WaitGroup
}

// writeWait bounds one write to a face: a face that reads nothing for that
// long is cut off, as one past its queue is.
const writeWait = 10 * time.Second

// inputWait bounds how long an input may take to reach the harness.
const inputWait = 10 * time.Second

// Serve listens at path, which a dead run of the same name and epoch cannot
// hold, and serves the model until Close.
func Serve(path string, model *Model) (*Server, error) {
	if info, err := os.Lstat(path); err == nil && info.Mode()&os.ModeSocket != 0 {
		_ = state.Discard(path)
	}
	listener, err := state.Listen(path)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Server{model: model, listener: listener, ctx: ctx, cancel: cancel, conns: map[net.Conn]struct{}{}}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

// Close stops serving, ends every face's stream and removes the socket.
func (s *Server) Close() error {
	s.cancel()
	err := s.listener.Close()
	s.mu.Lock()
	for conn := range s.conns {
		_ = conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	return err
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.ctx.Err() != nil {
			s.mu.Unlock()
			_ = conn.Close()
			return
		}
		s.conns[conn] = struct{}{}
		s.mu.Unlock()
		s.wg.Add(1)
		go s.serve(conn)
	}
}

// serve writes the face its snapshot and the changes, and reads its inputs;
// either side ending ends both.
func (s *Server) serve(conn net.Conn) {
	defer s.wg.Done()
	c := s.model.subscribe()
	defer func() {
		s.model.unsubscribe(c)
		_ = conn.Close()
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
	}()
	go s.read(conn, c)
	for {
		lines, served := c.take()
		for _, line := range lines {
			_ = conn.SetWriteDeadline(time.Now().Add(writeWait))
			if _, err := conn.Write(line); err != nil {
				return
			}
		}
		if !served {
			return
		}
		select {
		case <-c.wake:
		case <-s.ctx.Done():
			return
		}
	}
}

// read takes the face's inputs, each answered on the same stream once the
// harness took it or refused.
func (s *Server) read(conn net.Conn, c *client) {
	defer c.end()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 0, 4096), MaxLineBytes)
	for scanner.Scan() {
		var input Input
		decoder := json.NewDecoder(bytes.NewReader(scanner.Bytes()))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&input); err != nil || input.Kind != InputKindPrompt || input.ID == "" {
			c.push(encode(answer(input.ID, errors.New("not an input of the face"))))
			continue
		}
		go func() {
			ctx, cancel := context.WithTimeout(s.ctx, inputWait)
			defer cancel()
			c.push(encode(answer(input.ID, s.model.Input(ctx, input))))
		}()
	}
}

func answer(id string, err error) Line {
	line := Line{Op: LineOpAnswer, ID: &id, Ok: ptr(err == nil)}
	if err != nil {
		line.Detail = ptr(err.Error())
	}
	return line
}
