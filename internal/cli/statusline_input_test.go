package cli

import (
	"os"
	"testing"
	"time"
)

// The harness's JSON is read within a time as well as a size: a pipe that is
// held open after its bytes returns what came.
func TestStatusLineInputIsBoundedInTime(t *testing.T) {
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = reader.Close(); _ = writer.Close() })
	if _, err := writer.WriteString(`{"a":1}`); err != nil {
		t.Fatal(err)
	}
	started := time.Now()
	got := readStatusLineInput(reader, 200*time.Millisecond)
	if string(got) != `{"a":1}` || time.Since(started) > 3*time.Second {
		t.Fatalf("read %q in %v", got, time.Since(started))
	}
}
