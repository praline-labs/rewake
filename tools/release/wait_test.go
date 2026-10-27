package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

// waiting gives a gate a clock that moves only when --wait pauses, and
// counts the pauses.
func waiting(g *gate) *int {
	now, pauses := time.Date(2026, 10, 27, 9, 0, 0, 0, time.UTC), 0
	g.clock = func() time.Time { return now }
	g.pause = func(_ context.Context, d time.Duration) error {
		pauses++
		now = now.Add(d)
		return nil
	}
	return &pauses
}

// With --wait a build the registry is slow to show is asked for again until
// it is there, and the release then counts as done.
func TestWaitAsksUntilTheReleaseShows(t *testing.T) {
	looks := 0
	f := &fake{answer: func(line string) result {
		if viewed(line) == "1.0.0-linux-arm64" {
			looks++
			if looks <= 3 {
				return e404
			}
		}
		return inRegistry(line)
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true, wait: defaultWait}, f)
	pauses := waiting(g)
	if !g.verify(context.Background()) {
		t.Fatalf("a release that showed within the wait did not count\n%s", out)
	}
	if *pauses != 3 {
		t.Errorf("paused %d times, want 3", *pauses)
	}
	text := out.String()
	for _, want := range []string{"waiting for the registry (40m0s left): " + packageName + "@1.0.0-linux-arm64", "rewake@1.0.0-linux-arm64 visible"} {
		if !strings.Contains(text, want) {
			t.Errorf("output lacks %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "not visible yet") {
		t.Errorf("the report is of an earlier look:\n%s", text)
	}
}

// A wait that ends with a build still unseen, or a tag still elsewhere, is
// no finished release, as without --wait.
func TestWaitEndsWithoutTheRelease(t *testing.T) {
	f := &fake{answer: func(line string) result {
		if strings.Contains(line, " dist-tags") {
			return result{stdout: `{"latest":"0.9.0","linux-x64":"1.0.0-linux-x64","linux-arm64":"1.0.0-linux-arm64"}`}
		}
		return inRegistry(line)
	}}
	g, out := newGate(t, call{version: "1.0.0", publish: true, wait: 2 * time.Minute}, f)
	pauses := waiting(g)
	if g.verify(context.Background()) {
		t.Fatalf("a tag still behind counted as done\n%s", out)
	}
	if *pauses != 4 {
		t.Errorf("paused %d times over a 2m wait, want 4 of 30s", *pauses)
	}
	for _, want := range []string{"waited 2m0s", "latest is 0.9.0, not 1.0.0"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestParseWait(t *testing.T) {
	for _, tc := range []struct {
		args []string
		want time.Duration
		says string
	}{
		{args: []string{"1.0.0", "--publish", "--wait"}, want: defaultWait},
		{args: []string{"1.0.0", "--publish", "--wait=15m"}, want: 15 * time.Minute},
		{args: []string{"1.0.0", "--wait"}, says: "--wait is for --publish"},
		{args: []string{"1.0.0", "--publish", "--wait=soon"}, says: "--wait takes a duration"},
		{args: []string{"1.0.0", "--publish", "--wait=0s"}, says: "--wait takes a duration"},
	} {
		got, err := parseArgs(tc.args)
		switch {
		case tc.says != "" && (err == nil || !strings.Contains(err.Error(), tc.says)):
			t.Errorf("%v: got %v, want %q", tc.args, err, tc.says)
		case tc.says == "" && (err != nil || got.wait != tc.want):
			t.Errorf("%v: got %v %v, want %v", tc.args, got.wait, err, tc.want)
		}
	}
}
