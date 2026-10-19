package terminal

import (
	"io"
	"testing"
	"time"
)

// The kitty keyboard flags follow the program's pushes, pops and sets, and
// its query is answered from them: a program that hears no answer falls back
// to legacy keys, where Shift+Enter is a plain Enter.
func TestTheKittyFlagsFollowTheProgram(t *testing.T) {
	emulator, err := newVT(Size{Cols: 8, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	vt := emulator.(*vtEmulator)
	defer func() { _ = vt.stopReplies() }()
	replies := make(chan string, 1)
	go func() {
		buf := make([]byte, 64)
		n, _ := vt.replies().Read(buf)
		replies <- string(buf[:n])
	}()
	for _, step := range []struct {
		write string
		want  uint8
	}{
		{"\x1b[>5u", 5},
		{"\x1b[>7u", 7},
		{"\x1b[=8;2u", 15},
		{"\x1b[=2;3u", 13},
		{"\x1b[<u", 5},
		{"\x1b[=1u", 1},
		{"\x1b[<u", 0},
		{"\x1b[<u", 0},
		{"\x1b[>1u\x1b[>3u\x1b[<2u", 0},
	} {
		vt.write([]byte(step.write))
		if got := vt.look().modes.KittyKeys; got != step.want {
			t.Fatalf("after %q the flags are %d, want %d", step.write, got, step.want)
		}
	}
	vt.write([]byte("\x1b[>5u\x1b[?u"))
	select {
	case got := <-replies:
		if got != "\x1b[?5u" {
			t.Fatalf("the query was answered %q", got)
		}
	case <-time.After(time.Second):
		t.Fatal("the query was not answered")
	}
}

func TestModifyOtherKeysFollowsTheProgram(t *testing.T) {
	emulator, err := newVT(Size{Cols: 8, Rows: 1})
	if err != nil {
		t.Fatal(err)
	}
	vt := emulator.(*vtEmulator)
	go func() { _, _ = io.Copy(io.Discard, vt.replies()) }()
	defer func() { _ = vt.stopReplies() }()
	for _, step := range []struct {
		write string
		want  uint8
	}{{"\x1b[>4;2m", 2}, {"\x1b[>1;2m", 2}, {"\x1b[>4m", 0}, {"\x1b[>4;1m", 1}, {"\x1b[>4n", 0}} {
		vt.write([]byte(step.write))
		if got := vt.look().modes.ModifyOtherKeys; got != step.want {
			t.Fatalf("after %q the level is %d, want %d", step.write, got, step.want)
		}
	}
}
