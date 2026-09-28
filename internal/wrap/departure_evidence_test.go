package wrap

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/proc"
	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/role"
	"github.com/praline-labs/rewake/internal/state"
)

func departureFixture(t *testing.T) (string, registry.Session, registry.Session, *sessionNotices, string) {
	t.Helper()
	dir := stateDir(t)
	main := availabilityPeer(t, dir, "main-fixture", role.Main.ID, 1, true)
	peer := availabilityPeer(t, dir, "worker-fixture", role.Write.ID, 2, true)
	peer.PIDNamespace = "departure-namespace"
	peer.HarnessPID = 222
	peer.HarnessStart = 3
	if err := registry.Update(dir, peer); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "self/ns"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(peer.PIDNamespace, filepath.Join(root, "self/ns/pid")); err != nil {
		t.Fatal(err)
	}
	for _, p := range []struct {
		pid   int
		start uint64
	}{{peer.ServicePID, peer.ServiceStart}, {peer.HarnessPID, peer.HarnessStart}} {
		if err := os.MkdirAll(filepath.Join(root, fmt.Sprint(p.pid)), 0o700); err != nil {
			t.Fatal(err)
		}
		writeDepartureStat(t, root, p.pid, p.start, "S")
	}
	original := proc.Default
	proc.Default = proc.Reader{Root: root}
	t.Cleanup(func() { proc.Default = original })
	noticeState(t, dir, peer, "working", 1, nil)
	observer := newSessionNotices()
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	if len(observer.known) != 1 {
		t.Fatal("live worker not tracked")
	}
	return dir, main, peer, observer, root
}

func writeDepartureStat(t *testing.T, root string, pid int, start uint64, status string) {
	t.Helper()
	line := fmt.Sprintf("%d (worker) %s%s %d\n", pid, status, strings.Repeat(" 0", 18), start)
	if err := os.WriteFile(filepath.Join(root, fmt.Sprint(pid), "stat"), []byte(line), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestDepartureUnknownRetainsWorkerUntilConfirmedDeath(t *testing.T) {
	if runProcessFixture(t) {
		return
	}
	for _, target := range []string{"service", "harness"} {
		for _, mode := range []string{"permission", "malformed", "foreign namespace", "missing namespace"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				dir, main, peer, observer, root := departureFixture(t)
				pid, start := peer.ServicePID, peer.ServiceStart
				if target == "harness" {
					pid, start = peer.HarnessPID, peer.HarnessStart
				}
				path := filepath.Join(root, fmt.Sprint(pid), "stat")
				ns := filepath.Join(root, "self/ns/pid")
				switch mode {
				case "permission":
					if err := os.Chmod(path, 0); err != nil {
						t.Fatal(err)
					}
					if _, err := proc.StartTime(pid); !os.IsPermission(err) {
						t.Fatalf("expected EACCES, got %v", err)
					}
				case "malformed":
					if err := os.WriteFile(path, []byte("bad stat"), 0o600); err != nil {
						t.Fatal(err)
					}
				default:
					if err := os.Remove(ns); err != nil {
						t.Fatal(err)
					}
					if mode == "foreign namespace" {
						if err := os.Symlink("other-namespace", ns); err != nil {
							t.Fatal(err)
						}
					}
				}
				for range 2 {
					if err := observer.scan(context.Background(), dir, main); err != nil {
						t.Fatal(err)
					}
				}
				_, _, departed := noticeKinds(t, dir, main.Name)
				if len(departed) != 0 || len(observer.known) != 1 {
					t.Fatal("unknown identity lost worker or announced departure")
				}
				if mode == "permission" {
					if err := os.Chmod(path, 0o600); err != nil {
						t.Fatal(err)
					}
				}
				if mode == "foreign namespace" || mode == "missing namespace" {
					if mode == "foreign namespace" {
						if err := os.Remove(ns); err != nil {
							t.Fatal(err)
						}
					}
					if err := os.Symlink(peer.PIDNamespace, ns); err != nil {
						t.Fatal(err)
					}
				}
				writeDepartureStat(t, root, pid, start, "S")
				if err := observer.scan(context.Background(), dir, main); err != nil {
					t.Fatal(err)
				}
				available, _, departed := noticeKinds(t, dir, main.Name)
				if len(available) != 1 || len(departed) != 0 || len(observer.known) != 1 {
					t.Fatal("readability recovery duplicated availability or lost worker")
				}
				writeDepartureStat(t, root, pid, start, "Z")
				for range 2 {
					if err := observer.scan(context.Background(), dir, main); err != nil {
						t.Fatal(err)
					}
				}
				_, _, departed = noticeKinds(t, dir, main.Name)
				if len(departed) != 1 || len(observer.known) != 0 {
					t.Fatal("confirmed death did not publish once")
				}
				if current, err := registry.Load(dir, peer.Name); err != nil || current.Epoch() != peer.Epoch() {
					t.Fatal("observation mutated registry")
				}
			})
		}
	}
}

func TestDepartureConfirmedProcessEvidence(t *testing.T) {
	if runProcessFixture(t) {
		return
	}
	for _, target := range []string{"service", "harness"} {
		for _, mode := range []string{"missing", "reused", "zombie"} {
			t.Run(target+"/"+mode, func(t *testing.T) {
				dir, main, peer, observer, root := departureFixture(t)
				pid, start := peer.ServicePID, peer.ServiceStart
				if target == "harness" {
					pid, start = peer.HarnessPID, peer.HarnessStart
				}
				switch mode {
				case "missing":
					if err := os.Remove(filepath.Join(root, fmt.Sprint(pid), "stat")); err != nil {
						t.Fatal(err)
					}
				case "reused":
					writeDepartureStat(t, root, pid, start+1, "S")
				case "zombie":
					writeDepartureStat(t, root, pid, start, "Z")
				}
				for range 2 {
					if err := observer.scan(context.Background(), dir, main); err != nil {
						t.Fatal(err)
					}
				}
				_, _, departed := noticeKinds(t, dir, main.Name)
				if len(departed) != 1 || departed[0].Departure.Reason != "process ended" {
					t.Fatal("confirmed death missing or duplicated")
				}
			})
		}
	}
}

func TestDepartureRevalidationAndUncertainRetry(t *testing.T) {
	if runProcessFixture(t) {
		return
	}
	dir, main, peer, observer, root := departureFixture(t)
	writeDepartureStat(t, root, peer.HarnessPID, peer.HarnessStart, "Z")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := state.WithMailboxLock(context.Background(), dir, main.Name, func() error { return observer.scan(ctx, dir, main) }); err != nil {
		t.Fatal(err)
	}
	var worker *knownWorker
	for _, known := range observer.known {
		worker = known
	}
	if worker == nil || worker.departure == nil {
		t.Fatal("failed publication lost pending notice")
	}
	pending := worker.departure
	path := filepath.Join(root, fmt.Sprint(peer.HarnessPID), "stat")
	if err := os.Chmod(path, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := proc.StartTime(peer.HarnessPID); !os.IsPermission(err) {
		t.Fatalf("expected EACCES, got %v", err)
	}
	if err := putDepartureNotice(context.Background(), dir, main, peer, *pending); !errors.Is(err, registry.ErrNotFound) {
		t.Fatalf("uncertain publication revalidation = %v", err)
	}
	if err := observer.scan(context.Background(), dir, main); err != nil {
		t.Fatal(err)
	}
	_, _, departed := noticeKinds(t, dir, main.Name)
	if len(departed) != 0 || len(observer.known) != 1 || worker.departure != pending {
		t.Fatal("uncertain retry published or lost pending identity")
	}
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := observer.scan(context.Background(), dir, main); err != nil {
			t.Fatal(err)
		}
	}
	_, _, departed = noticeKinds(t, dir, main.Name)
	if len(departed) != 1 || departed[0].ID != pending.ID {
		t.Fatal("retry lost or duplicated pending identity")
	}
}

func TestDepartureRevalidationRejectsAliveAndForeignNamespace(t *testing.T) {
	if runProcessFixture(t) {
		return
	}
	for _, mode := range []string{"alive", "foreign namespace"} {
		t.Run(mode, func(t *testing.T) {
			dir, main, peer, observer, root := departureFixture(t)
			writeDepartureStat(t, root, peer.HarnessPID, peer.HarnessStart, "Z")
			var worker *knownWorker
			for _, known := range observer.known {
				worker = known
			}
			message := departureMessage(main, worker, departureReason(dir, peer))
			worker.departure = &message
			if mode == "alive" {
				writeDepartureStat(t, root, peer.HarnessPID, peer.HarnessStart, "S")
			} else {
				peer.PIDNamespace = "other-namespace"
				if err := registry.Update(dir, peer); err != nil {
					t.Fatal(err)
				}
			}
			if err := putDepartureNotice(context.Background(), dir, main, peer, message); !errors.Is(err, registry.ErrNotFound) {
				t.Fatalf("obsolete publication = %v", err)
			}
			if err := observer.scan(context.Background(), dir, main); err != nil {
				t.Fatal(err)
			}
			if len(observer.known) != 1 || (worker.departure == nil) != (mode == "alive") {
				t.Fatal("alive evidence must clear pending departure; unknown must retain it")
			}
			_, _, departed := noticeKinds(t, dir, main.Name)
			if len(departed) != 0 {
				t.Fatal("revalidation queued obsolete notice")
			}
		})
	}
}
