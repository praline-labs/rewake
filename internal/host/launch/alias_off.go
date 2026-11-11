package launch

import "fmt"

// offRequest is one name a project file turned off, with the file's words.
type offRequest struct{ name, description, path string }

// noteAliasesOff takes the project file's [aliases] off. The user's file may
// not hold it: a list there would hide templates everywhere, which is deleting
// them, and so it is refused rather than read. A project's own alias of a
// listed name is a contradiction in one file and refused, not hidden.
func (s *Set) noteAliasesOff(names []string, own map[string]Alias, description, path string, user bool) error {
	if len(names) == 0 {
		return nil
	}
	if user {
		return fmt.Errorf("%s at %s has [aliases] off; it belongs in a project's .rewake/aliases.toml, and to drop a template everywhere delete it from this file", description, path)
	}
	for _, name := range names {
		if _, ok := own[name]; ok {
			return fmt.Errorf("%s at %s defines [alias.%s] and turns it off in [aliases] off; keep one", description, path, name)
		}
		s.pendingOff = append(s.pendingOff, offRequest{name, description, path})
	}
	return nil
}

// applyAliasesOff hides the listed templates once the user's and the project's
// files are merged. `off` is subtraction from the catalog: a name no file
// holds any more is already off, so a list that still names a template removed
// from the user's or the project's file skips it with a migration notice
// instead of refusing every launch in that project.
func (s *Set) applyAliasesOff() {
	for _, request := range s.pendingOff {
		if _, ok := s.aliases[request.name]; !ok {
			s.Migrations = append(s.Migrations, fmt.Sprintf("%s at %s turns off %q in [aliases] off, which no template of the catalog holds any more; it is already off, so nothing is hidden and the launch goes on. Remove the name from the list.", request.description, request.path, request.name))
			continue
		}
		s.aliasOff[request.name] = request.description
		delete(s.aliases, request.name)
	}
	s.pendingOff = nil
}
