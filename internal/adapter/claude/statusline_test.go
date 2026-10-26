package claude

import (
	"bytes"
	"context"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The command forwards the input's prompt_cache and effort to the run's host and prints
// the person's own status line, run with the same input; a host that is not
// there and a line that is not set each leave the other alone.
func TestStatusLineForwardsTheCacheAndPrintsThePersonsLine(t *testing.T) {
	socket := filepath.Join(t.TempDir(), "s")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatal(err)
	}
	got := make(chan string, 1)
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		got <- r.URL.Path + " " + string(body)
		_, _ = w.Write([]byte("{}"))
	})}
	go func() { _ = server.Serve(listener) }()
	t.Cleanup(func() { _ = server.Close() })

	folder := t.TempDir()
	settings := `{"statusLine":{"type":"command","command":"cat >/dev/null; echo person-line"}}`
	if err := os.WriteFile(filepath.Join(folder, "settings.json"), []byte(settings), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv(modSocketEnv, socket)
	t.Setenv("CLAUDE_CONFIG_DIR", folder)
	input := []byte(`{"model":{"id":"m1"},"prompt_cache":{"warm":true,"ttl":"1h","expires_at":4102444800,"requests":3}}`)
	var out bytes.Buffer
	claudeHarness{}.StatusLine(context.Background(), input, &out)
	if line := <-got; line != `/v1/status/line {"ttl":"1h","expiresAt":4102444800,"warm":true}` {
		t.Fatalf("the post: %s", line)
	}
	if strings.TrimSpace(out.String()) != "person-line" {
		t.Fatalf("the person's line: %q", out.String())
	}

	// Before any request the input has an effort and no cache: the effort
	// alone goes.
	t.Setenv("CLAUDE_CONFIG_DIR", t.TempDir())
	out.Reset()
	claudeHarness{}.StatusLine(context.Background(), []byte(`{"model":{"id":"m1"},"effort":{"level":"max"}}`), &out)
	if line := <-got; line != `/v1/status/line {"effort":"max"}` {
		t.Fatalf("the effort's post: %s", line)
	}

	// The model's own name goes alone too.
	claudeHarness{}.StatusLine(context.Background(), []byte(`{"model":{"id":"m1","display_name":"Haiku 5.5"}}`), &out)
	if line := <-got; line != `/v1/status/line {"modelName":"Haiku 5.5"}` {
		t.Fatalf("the model name's post: %s", line)
	}

	// Neither cache nor effort (a model without effort, before any request)
	// posts nothing; no socket and no line print nothing and fail nothing.
	claudeHarness{}.StatusLine(context.Background(), []byte(`{"model":{"id":"m1"}}`), &out)
	t.Setenv(modSocketEnv, "")
	claudeHarness{}.StatusLine(context.Background(), input, &out)
	if out.Len() != 0 || len(got) != 0 {
		t.Fatalf("nothing was to be done: %q, %d posts", out.String(), len(got))
	}
}
