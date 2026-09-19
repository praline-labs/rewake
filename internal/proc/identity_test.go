package proc

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestObserveIdentityEvidence(t *testing.T) {
	for _, tc := range []struct {
		name  string
		pid   int
		start uint64
		state string
		want  IdentityEvidence
	}{
		{"same identity", 7, 4242, "S", IdentityAlive},
		{"legacy start", 7, 0, "R", IdentityAlive},
		{"missing", 8, 4242, "S", IdentityEnded},
		{"reused", 7, 9999, "S", IdentityEnded},
		{"zombie", 7, 4242, "Z", IdentityEnded},
		{"unhandled state", 7, 4242, "X", IdentityUnknown},
		{"invalid pid", 0, 4242, "S", IdentityUnknown},
		{"invalid state", 7, 4242, "?", IdentityUnknown},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reader := fixture(t, map[int][2]int{7: {1, 4242}})
			path := filepath.Join(reader.Root, "7", "stat")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(path, []byte(strings.Replace(string(raw), ") S ", ") "+tc.state+" ", 1)), 0o600); err != nil {
				t.Fatal(err)
			}
			if got := reader.ObserveIdentity(tc.pid, tc.start); got != tc.want {
				t.Fatalf("evidence = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestObserveIdentityUnreadableOrMalformed(t *testing.T) {
	for _, mode := range []string{"permission", "truncated", "bad start", "missing command", "directory"} {
		t.Run(mode, func(t *testing.T) {
			reader := fixture(t, map[int][2]int{7: {1, 4242}})
			path := filepath.Join(reader.Root, "7", "stat")
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			switch mode {
			case "permission":
				if err := os.Chmod(path, 0); err != nil {
					t.Fatal(err)
				}
				if _, err := reader.StartTime(7); !os.IsPermission(err) {
					t.Fatalf("expected EACCES, got %v", err)
				}
			case "directory":
				if err := os.Remove(path); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(path, 0o700); err != nil {
					t.Fatal(err)
				}
			default:
				text := "7 (command) S 1"
				if mode == "bad start" {
					text = strings.Replace(string(raw), "4242", "invalid", 1)
				}
				if mode == "missing command" {
					text = "invalid"
				}
				if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if got := reader.ObserveIdentity(7, 4242); got != IdentityUnknown {
				t.Fatalf("evidence = %v, want unknown", got)
			}
		})
	}
}
