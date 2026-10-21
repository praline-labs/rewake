package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/praline-labs/rewake/internal/core/worktype"
)

// The types main may raise a worker for (docs/worker-types.md): rewake's own,
// the person's in ~/.config/rewake/types/ and the project's in
// .rewake/types/, the nearest of one name winning. A built-in type an alias
// file turns off is gone, but a file of that name, the person's own text,
// still stands.

// readTypes reads every <name>.md of a types directory. A missing directory
// is no types; a file that does not read refuses the catalog, as an alias
// file does, rather than leave a type silently absent.
func (s *Set) readTypes(dir, source string) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return
	}
	if err != nil {
		s.unreadable(fmt.Sprintf("not reading the %s types in %s: %v", source, dir, err))
		return
	}
	for _, entry := range entries {
		name, ok := strings.CutSuffix(entry.Name(), ".md")
		if !ok || strings.HasPrefix(entry.Name(), ".") {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil || !info.Mode().IsRegular() {
			// A named pipe would block the read for ever, as for the alias file.
			s.unreadable(fmt.Sprintf("not reading the %s type %s: it is not an ordinary file", source, path))
			continue
		}
		if info.Size() > worktype.MaxSize {
			s.unreadable(fmt.Sprintf("the %s type %s is %d bytes, over the %d KiB a type may hold", source, path, info.Size(), worktype.MaxSize>>10))
			continue
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			s.unreadable(fmt.Sprintf("not reading the %s type %s: %v", source, path, err))
			continue
		}
		value, err := worktype.Parse(name, raw)
		if err != nil {
			s.unreadable(fmt.Sprintf("%v (%s)", err, path))
			continue
		}
		value.Source, value.Path = source, path
		s.types[name] = value
	}
}

// turnOff records the built-in types an alias file turns off. A name that is
// no built-in type is refused, as an unknown field is: a typo would leave on
// what the person meant to turn off.
func (s *Set) turnOff(names []string, description, path string) error {
	builtins := worktype.BuiltIns()
	for _, name := range names {
		if _, ok := builtins[name]; !ok {
			return fmt.Errorf("%s at %s turns off %q in [types] off, which is not a built-in type; built-in: %s",
				description, path, name, strings.Join(worktype.Names(builtins), ", "))
		}
	}
	for _, name := range names {
		if where, ok := s.off[name]; ok {
			s.off[name] = where + " and " + description
		} else {
			s.off[name] = description
		}
	}
	return nil
}

// refuseMoved refuses the project alias file of before October 21, 2026,
// naming where its aliases go now: read silently, or left unread, either
// would start workers other than the project says.
func (s *Set) refuseMoved(path string) {
	if _, err := os.Lstat(path); err == nil {
		s.unreadable(fmt.Sprintf("%s is no longer read: a project's aliases moved to %s, beside its types in %s; move the file there",
			path, filepath.Join(filepath.Dir(path), projectDir, userFile), filepath.Join(filepath.Dir(path), projectDir, typesDir)))
	}
}

// Types are the types main may raise a worker for: the files read, and the
// built-in types not turned off and not replaced by a file.
func (s *Set) Types() []worktype.Type {
	all := map[string]worktype.Type{}
	for name, value := range worktype.BuiltIns() {
		if _, off := s.off[name]; !off {
			all[name] = value
		}
	}
	for name, value := range s.types {
		all[name] = value
	}
	out := make([]worktype.Type, 0, len(all))
	for _, name := range worktype.Names(all) {
		out = append(out, all[name])
	}
	return out
}

// Type resolves one type by name, the nearest first, refusing one that is
// not there or that an alias file turned off.
func (s *Set) Type(name string) (worktype.Type, error) {
	if value, ok := s.types[name]; ok {
		return value, nil
	}
	if where, off := s.off[name]; off {
		return worktype.Type{}, fmt.Errorf("the built-in type %q is turned off in %s ([types] off); available: %s", name, where, s.typeNames())
	}
	if value, ok := worktype.BuiltIns()[name]; ok {
		return value, nil
	}
	return worktype.Type{}, fmt.Errorf("no type %q; available: %s", name, s.typeNames())
}

func (s *Set) typeNames() string {
	var names []string
	for _, value := range s.Types() {
		names = append(names, value.Name)
	}
	if len(names) == 0 {
		return "none"
	}
	return strings.Join(names, ", ")
}
