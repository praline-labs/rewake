package state

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// WithOutside lets a write reach a folder the room does not hold, so only the
// briefs' package refers to it (docs/rules/state.md#what-is-a-writer): a
// reference from anywhere else of internal/ — a call, an alias, a function
// value — would be a write nobody reviewed.
func TestOnlyTheBriefsAdmitAFolderOutsideTheRoom(t *testing.T) {
	root := filepath.Join("..", "..")
	var calls []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		slash := filepath.ToSlash(path)
		if strings.Contains(slash, "/infra/state/") || strings.Contains(slash, "/core/taskbrief/") {
			return nil
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(file, func(node ast.Node) bool {
			if selector, ok := node.(*ast.SelectorExpr); ok && (selector.Sel.Name == "WithOutside" || selector.Sel.Name == "EnsureOutsideDir") {
				calls = append(calls, slash+": "+selector.Sel.Name)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(calls) > 0 {
		t.Fatalf("a folder outside the room admitted from outside the briefs:\n%s", strings.Join(calls, "\n"))
	}
}

// Without the admission a folder of the project is no room's: its write is
// refused, and the admission ends with the function it wraps.
func TestAFolderOutsideTheRoomIsWritableOnlyInsideItsAdmission(t *testing.T) {
	folder := filepath.Join(t.TempDir(), "project")
	err := WriteAtomic(filepath.Join(folder, "x"), []byte("x"))
	if !errors.Is(err, ErrNoLease) {
		t.Fatalf("a write outside every room: %v", err)
	}
	if err := WithOutside(folder, folder, func() error { return nil }); !errors.Is(err, ErrNoLease) {
		t.Errorf("an admission without the room's lease: %v", err)
	}
}
