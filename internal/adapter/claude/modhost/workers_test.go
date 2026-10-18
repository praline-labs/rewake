package modhost

import (
	"context"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/praline-labs/rewake/internal/adapter"
)

// spaces is a host's view of one space whose worker writes "hi".
type spaces struct {
	mu     sync.Mutex
	opened int
	keys   []string
	sizes  [][2]int
	closed int
}

func (f *spaces) Workers() ([]adapter.SpaceWorker, error) {
	return []adapter.SpaceWorker{{Space: "s1", Name: "api", Session: "api-claude", Harness: "claude", State: "open", Activity: "idle", Model: "m", Percent: 34}}, nil
}

func (f *spaces) View(string) (adapter.ScreenView, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened++
	return &screen{spaces: f}, nil
}

type screen struct{ spaces *spaces }

func (s *screen) Screen(ctx context.Context, after uint64) (adapter.Screen, error) {
	if after == 1 {
		<-ctx.Done()
	}
	return adapter.Screen{
		Seq: 1, Cols: 3, Rows: 1,
		Lines:  [][]adapter.ScreenRun{{{Text: "hi", Cols: 2, Fg: adapter.ScreenColor{Kind: adapter.IndexedColor, Index: 1}}}},
		Cursor: adapter.ScreenCursor{Row: 0, Col: 2, Visible: true},
	}, nil
}

func (s *screen) Keys(p []byte) error {
	s.spaces.mu.Lock()
	defer s.spaces.mu.Unlock()
	s.spaces.keys = append(s.spaces.keys, string(p))
	return nil
}

func (s *screen) Resize(cols, rows int) error {
	s.spaces.mu.Lock()
	defer s.spaces.mu.Unlock()
	s.spaces.sizes = append(s.spaces.sizes, [2]int{cols, rows})
	return nil
}

func (s *screen) Close() error {
	s.spaces.mu.Lock()
	defer s.spaces.mu.Unlock()
	s.spaces.closed++
	return nil
}

func ask(t *testing.T, socket, path string, request any) (int, string) {
	t.Helper()
	body, _ := json.Marshal(request)
	status, text, err := post(socket, path, body)
	if err != nil {
		t.Fatal(err)
	}
	return status, text
}

// The pane's requests reach the host's spaces: the list with the workers'
// state, a screen as a Raster's cells, a held poll answered the same screen
// once its hold is over, keys and a size; the one view they share is closed
// with the run.
func TestThePanesRequestsReachTheSpaces(t *testing.T) {
	host := &spaces{}
	server, socket := servingWith(t, func(s *Server) {
		s.ShowSpaces(host)
		s.pane.hold = 100 * time.Millisecond
	})
	if status, text := ask(t, socket, PathWorkersList, WorkersRequest{LoadID: "first"}); status != http.StatusConflict {
		t.Fatalf("a load not admitted was answered: %d %s", status, text)
	}
	if status, _, err := post(socket, PathHello, validHello("first")); err != nil || status != http.StatusOK {
		t.Fatalf("the hello: %d %v", status, err)
	}
	status, text := ask(t, socket, PathWorkersList, WorkersRequest{LoadID: "first"})
	var list WorkersAnswer
	if status != http.StatusOK || json.Unmarshal([]byte(text), &list) != nil || len(list.Workers) != 1 ||
		list.Workers[0].Session != "api-claude" || list.Workers[0].Percent == nil || *list.Workers[0].Percent != 34 || list.Workers[0].Effort != nil {
		t.Fatalf("the list: %d %s", status, text)
	}

	status, text = ask(t, socket, PathWorkersScreen, ScreenRequest{LoadID: "first", Space: "s1"})
	var shown ScreenAnswer
	if status != http.StatusOK || json.Unmarshal([]byte(text), &shown) != nil || shown.Seq != 1 || shown.Columns != 3 || shown.Rows != 1 {
		t.Fatalf("the screen: %d %s", status, text)
	}
	raw, err := base64.StdEncoding.DecodeString(shown.Cells)
	if err != nil || len(raw) != 3*12 {
		t.Fatalf("the cells: %d bytes, %v", len(raw), err)
	}
	cell := func(i, part int) uint32 { return binary.LittleEndian.Uint32(raw[i*12+part*4:]) }
	if cell(0, 0) != 'h' || cell(0, 1) != 0xcd0000 || cell(0, 2) != defaultColor || cell(2, 0) != ' ' || cell(2, 1) != darkGrey || cell(2, 2) != lightGrey {
		t.Fatalf("the cells: %x", raw)
	}
	began := time.Now()
	if status, text = ask(t, socket, PathWorkersScreen, ScreenRequest{LoadID: "first", Space: "s1", After: 1}); status != http.StatusOK || time.Since(began) < 100*time.Millisecond {
		t.Fatalf("a poll of an unchanged screen was not held: %d %s after %s", status, text, time.Since(began))
	}
	if status, text = ask(t, socket, PathWorkersScreen, ScreenRequest{LoadID: "first", Space: "s2"}); status != http.StatusNotFound {
		t.Fatalf("a space not listed was shown: %d %s", status, text)
	}

	if status, text = ask(t, socket, PathWorkersKeys, KeysRequest{LoadID: "first", Space: "s1", Keys: "ls\r"}); status != http.StatusOK {
		t.Fatalf("the keys: %d %s", status, text)
	}
	if status, text = ask(t, socket, PathWorkersResize, ResizeRequest{LoadID: "first", Space: "s1", Columns: 80, Rows: 20}); status != http.StatusOK {
		t.Fatalf("the size: %d %s", status, text)
	}
	if status, _ = ask(t, socket, PathWorkersResize, ResizeRequest{LoadID: "first", Space: "s1", Columns: 0, Rows: 20}); status != http.StatusBadRequest {
		t.Fatalf("no size was taken: %d", status)
	}
	server.Close()
	host.mu.Lock()
	defer host.mu.Unlock()
	if host.opened != 1 || len(host.keys) != 1 || host.keys[0] != "ls\r" || len(host.sizes) != 1 || host.sizes[0] != [2]int{80, 20} || host.closed != 1 {
		t.Fatalf("what reached the spaces: %+v", host)
	}
}

// A run that opens no spaces answers no pane.
func TestARunOfNoSpacesAnswersNoPane(t *testing.T) {
	_, socket := serving(t)
	if status, _, err := post(socket, PathHello, validHello("first")); err != nil || status != http.StatusOK {
		t.Fatalf("the hello: %d %v", status, err)
	}
	if status, text := ask(t, socket, PathWorkersList, WorkersRequest{LoadID: "first"}); status != http.StatusNotFound {
		t.Fatalf("the list: %d %s", status, text)
	}
}

// A run's glyphs become cells a Raster takes: a wide glyph, which the
// emulator gives a run of its own, a mark and a blank; a combining mark none;
// a private-use glyph the mark; in a run that is not the emulator's, whose
// columns do not match its glyphs, every character outside ASCII the mark —
// a wide symbol of the BMP such as U+2705 included, which Claude Code would
// refuse the whole screen over; and the run keeps its width.
func TestGlyphsKeepTheirRunsWidth(t *testing.T) {
	for _, c := range []struct {
		text string
		cols int
		want string
	}{
		{"ab", 2, "ab"},
		{"界", 2, "? "},
		{"✅", 2, "? "},
		{"界x", 3, "?x "},
		{"✅x", 3, "?x "},
		{"é", 1, "e"},
		{"", 1, "?"},
		{"abc", 2, "ab"},
		{"a", 3, "a  "},
		{"❯\u00a0", 2, "❯ "},
	} {
		if got := string(glyphs(c.text, c.cols)); got != c.want {
			t.Errorf("glyphs(%q, %d) = %q, want %q", c.text, c.cols, got, c.want)
		}
	}
}

// Bold and dim, which a Raster cannot draw, are drawn as a brighter and a
// darker foreground.
func TestBoldAndDimBecomeColor(t *testing.T) {
	red := adapter.ScreenColor{Kind: adapter.IndexedColor, Index: 1}
	orange := adapter.ScreenColor{Kind: adapter.RGBColor, R: 0xd7, G: 0x77, B: 0x57}
	for _, c := range []struct {
		color adapter.ScreenColor
		attrs adapter.ScreenAttrs
		want  uint32
	}{
		{adapter.ScreenColor{}, 0, defaultColor},
		{adapter.ScreenColor{}, adapter.Dim, dimGrey},
		{adapter.ScreenColor{}, adapter.Bold, defaultColor},
		{red, 0, 0xcd0000},
		{red, adapter.Bold, 0xff0000},
		{orange, 0, 0xd77757},
		{orange, adapter.Dim, 0x8f4f3a},
		{orange, adapter.Bold, 0xe4a48f},
	} {
		if got := foreground(c.color, c.attrs); got != c.want {
			t.Errorf("foreground(%+v, %d) = %06x, want %06x", c.color, c.attrs, got, c.want)
		}
	}
}

// A view nobody names for its idle time is closed even when no other space
// is asked for: a closed pane asks for none.
func TestALoneIdleViewIsClosed(t *testing.T) {
	server, _ := serving(t)
	host := &spaces{}
	server.ShowSpaces(host)
	server.mu.Lock()
	server.pane.idle = 20 * time.Millisecond
	server.mu.Unlock()
	if _, err := server.screenOf("s1"); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		host.mu.Lock()
		closed := host.closed
		host.mu.Unlock()
		if closed == 1 {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("the lone view stayed open past its idle time")
}
