package switching

import "testing"

// A drop that ends with a session busy names that session alone: the others
// stopped because it turned busy, and listing them as not parked buried the
// cause in a live switch (November 3, 2026). Only a drop where nobody said
// busy names who stayed silent.
func TestADropRefusalNamesTheBusySessionsOnly(t *testing.T) {
	busy := &leader{members: map[int]*member{
		1: {who: "main's host", answer: Ready},
		2: {who: "session main-claude", answer: Busy, why: "main's turn is open"},
		3: {who: "session review-pi", answer: Parked},
	}}
	if got, want := busy.unparked(), "session main-claude busy: main's turn is open"; got != want {
		t.Errorf("with a busy session: %q, want %q", got, want)
	}
	silent := &leader{members: map[int]*member{
		1: {who: "main's host", answer: Ready},
		2: {who: "session main-claude", answer: Parked},
		3: {who: "session review-pi", answer: Ready},
	}}
	if got, want := silent.unparked(), "main's host, session review-pi did not say parked"; got != want {
		t.Errorf("with nobody busy: %q, want %q", got, want)
	}
}
