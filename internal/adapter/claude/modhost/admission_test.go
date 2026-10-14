package modhost

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/infra/proc"
)

// replacingBody names another harness on its first read: the harness is
// replaced after the request's first peer check, while its body is decoded.
type replacingBody struct {
	io.Reader
	replace func()
}

func (b *replacingBody) Read(p []byte) (int, error) {
	if b.replace != nil {
		b.replace()
		b.replace = nil
	}
	return b.Reader.Read(p)
}

// A hello whose harness was replaced while its body was read is cut off with
// no effect: no load bound and nothing made live, since the peer is checked
// again before any effect of the hello, not only before its answer. Kept from
// the review of S4-5 part B.
func TestAHelloFromAReplacedHarnessHasNoEffect(t *testing.T) {
	start, err := proc.StartTime(os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	admissions := 0
	server := New(Config{Wait: time.Second, Admitted: func(HelloRequest) { admissions++ }})
	server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start})
	body := &replacingBody{
		Reader: bytes.NewReader(validHello("replaced-before-binding")),
		replace: func() {
			server.HarnessStarted(adapter.RunProcess{PID: os.Getpid(), Start: start + 1})
		},
	}
	request := httptest.NewRequest(http.MethodPost, PathHello, body)
	request = request.WithContext(context.WithValue(request.Context(), peerKey{}, peer{pid: os.Getpid()}))
	response := httptest.NewRecorder()
	var aborted any
	func() {
		defer func() { aborted = recover() }()
		server.serve(response, request)
	}()
	if aborted != http.ErrAbortHandler {
		t.Fatalf("the replaced harness's hello was not cut off: %v; answered %d %s; body read %v", aborted, response.Code, response.Body.String(), body.replace == nil)
	}
	if admissions != 0 {
		t.Fatalf("a hello from a replaced harness was admitted %d time(s); on a main run that makes spaces live", admissions)
	}
	if _, ok := server.Load(); ok {
		t.Fatal("a hello from a replaced harness was bound as the run's load")
	}
}
