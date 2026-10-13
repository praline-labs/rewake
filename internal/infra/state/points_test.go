package state

import (
	"errors"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

// A write told its points (Points) is told them in their order, each before
// it is reached: a point that fails before the file is placed leaves the name
// free, a failed directory flush leaves the file in place and answers the
// error, and a temporary file the discard keeps stays as a stopped process
// leaves it.
func TestAWriteIsToldEachPointBeforeItIsReached(t *testing.T) {
	order := []string{PointTemp, PointTempSync, PointPlace, PointDirSync, PointDiscard}
	writes := map[string]func(string, []byte, Points) error{
		"publication": PublishExclusiveWith,
		"replacement": WriteAtomicWith,
	}
	for name, write := range writes {
		t.Run(name+" told every point", func(t *testing.T) {
			path := filepath.Join(leasedDir(t), "record")
			var told []string
			if err := write(path, []byte("x"), func(point, at string) error {
				if at != path {
					t.Errorf("point %s told of %s", point, at)
				}
				told = append(told, point)
				return nil
			}); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(told, order) {
				t.Fatalf("told %v, want %v", told, order)
			}
			if leftover(t, filepath.Dir(path)) != 0 {
				t.Error("a temporary file was left")
			}
		})
		for _, failed := range order[:4] {
			t.Run(name+" failed at "+failed+", the temporary file kept", func(t *testing.T) {
				path := filepath.Join(leasedDir(t), "record")
				stopped := errors.New("stopped")
				err := write(path, []byte("x"), func(point, _ string) error {
					if point == failed || point == PointDiscard {
						return stopped
					}
					return nil
				})
				if !errors.Is(err, stopped) {
					t.Fatalf("the write answered %v", err)
				}
				_, statErr := os.Stat(path)
				if placed := statErr == nil; placed != (failed == PointDirSync) {
					t.Errorf("the record placed %v after a failure at %s", placed, failed)
				}
				// Placed by a rename, the temporary file is the record.
				want := 1
				if failed == PointTemp || failed == PointDirSync && name == "replacement" {
					want = 0
				}
				if kept := leftover(t, filepath.Dir(path)); kept != want {
					t.Errorf("%d temporary files kept, want %d", kept, want)
				}
			})
		}
	}
}

func leftover(t *testing.T, dir string) int {
	t.Helper()
	temps, err := filepath.Glob(filepath.Join(dir, ".tmp-*"))
	if err != nil {
		t.Fatal(err)
	}
	return len(temps)
}
