package launch

// The worker type is gone (the owner's decision, November 3, 2026) and what
// it left in the files is obsolete: an alias's `type`, a [types] section, a
// types/ directory. Refusing them refused every launch in a project that
// still carried one, so main could not start to be told what to fix. They are
// skipped with a migration notice instead — the file, the line where TOML
// gives one, and what to write — and the rest of the catalog runs
// (docs/work-modes.md#the-old-worker-types).
//
// Only the old form is skipped: `type` holding a string and a [types] section
// holding off = ["<name>", ...] and nothing else. Any other value or field
// there is malformed and refuses the file, as an unknown field does, and so do
// malformed TOML and a malformed current setting. The definition an obsolete
// setting leaves out is checked against the current schema before it is
// omitted: a type may leave a valid template out, never screen a malformed one.
//
// A template whose `type` is skipped follows whole-template precedence: the
// layer that defines it whole decides, so a valid project definition replaces
// an obsolete user one, while an obsolete project definition hides the user's
// valid one. Only a definition in force is omitted and gets its notice.

import (
	"fmt"
	"sort"

	"github.com/pelletier/go-toml/v2"
)

// obsoleteName says what a key unknown to an alias file is: a template's
// obsolete type, or the [types] section.
func obsoleteName(key []string) (alias string, types bool) {
	if len(key) == 3 && key[0] == "alias" && key[2] == "type" {
		return key[1], false
	}
	return "", len(key) > 0 && key[0] == "types"
}

// degrade validates and takes every obsolete setting out of one file: it
// answers the fields that remain unknown, and the templates this file leaves
// out, with the notice each would have if its definition is the one in force.
func (s *Set) degrade(values map[string]any, fields []toml.DecodeError, description, path string) (unknown []toml.DecodeError, obsolete map[string]string) {
	obsolete = map[string]string{}
	typed := map[string]bool{}
	section := false
	for _, field := range fields {
		alias, types := obsoleteName(field.Key())
		line, _ := field.Position()
		switch {
		case alias != "":
			if typed[alias] {
				continue
			}
			typed[alias] = true
			if _, ok := rawString(values, "alias", alias, "type"); !ok {
				s.unreadable(fmt.Sprintf("%s at %s:%d: the obsolete type in [alias.%s] must be a string, as an old type was; remove it and put start_mode = \"<mode>\" under [alias.%s.pi]", description, path, line, alias, alias))
				obsolete[alias] = malformedAliasNotice(description, path, alias)
				continue
			}
			obsolete[alias] = obsoleteAliasNotice(description, path, line, alias)
		case types:
			if section {
				continue
			}
			section = true
			if !obsoleteTypesValid(values) {
				s.unreadable(fmt.Sprintf("%s at %s:%d: the obsolete [types] section was off = [\"<name>\", ...] and nothing else; this cannot be read as a worker type. Remove it, or move the text to modes/<name>.md", description, path, line))
				continue
			}
			s.Migrations = append(s.Migrations, obsoleteTypesNotice(description, path, line))
		default:
			unknown = append(unknown, field)
		}
	}
	return unknown, obsolete
}

// resolveObsolete applies whole-template precedence to the definitions an
// obsolete setting left out, and records the notice of each definition in
// force (alias_obsolete.go).
func (s *Set) resolveObsolete() {
	names := make([]string, 0, len(s.obsolete))
	for name := range s.obsolete {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		def := s.obsolete[name]
		if !def.project && s.fromProject[name] {
			// The project defines the template whole; the user's obsolete type
			// is not in force, and its notice would name a template nobody
			// starts.
			continue
		}
		delete(s.aliases, name)
		s.omitted[name] = def
		s.Migrations = append(s.Migrations, def.notice)
	}
	s.obsolete = nil
}

// rawString walks a leniently decoded TOML document to a string.
func rawString(values map[string]any, key ...string) (string, bool) {
	var at any = values
	for _, part := range key {
		table, ok := at.(map[string]any)
		if !ok {
			return "", false
		}
		at, ok = table[part]
		if !ok {
			return "", false
		}
	}
	text, ok := at.(string)
	return text, ok
}

// obsoleteTypesValid says whether the obsolete [types] section is the shape it
// had — off = ["<name>", ...], and no other field — so it can be skipped. Any
// other value or field is malformed and refuses the file.
func obsoleteTypesValid(values map[string]any) bool {
	value, ok := values["types"]
	if !ok {
		return false
	}
	table, ok := value.(map[string]any)
	if !ok {
		return false
	}
	for key, raw := range table {
		if key != "off" {
			return false
		}
		names, ok := raw.([]any)
		if !ok {
			return false
		}
		for _, name := range names {
			if _, ok := name.(string); !ok {
				return false
			}
		}
	}
	return true
}

// obsoleteAliasNotice says what to write for a template that still carries the
// old type, and that it is left out until then.
func obsoleteAliasNotice(description, path string, line int, name string) string {
	return fmt.Sprintf("%s at %s:%d: [alias.%s] still has the obsolete type field, so that template is left out until it is fixed: a worker's kind of work is now its runtime work mode, so put start_mode = \"<mode>\" under [alias.%s.pi] (code became build, analysis advisor; review and process keep their names)", description, path, line, name, name)
}

// malformedAliasNotice stands in for the notice of a template whose obsolete
// type is not the old form; the file is refused, so nothing starts on it.
func malformedAliasNotice(description, path, name string) string {
	return fmt.Sprintf("%s at %s: [alias.%s] has an obsolete type that is not a string, which refuses the file", description, path, name)
}

// obsoleteTypesNotice says where the text of an old [types] section goes.
func obsoleteTypesNotice(description, path string, line int) string {
	return fmt.Sprintf("%s at %s:%d: the [types] section is no longer read and is skipped: a worker's kind of work is its runtime work mode, a file under modes/<name>.md (built-in: advisor, build, review, process); move each type's text there, keeping its front matter, and code is now build and analysis is advisor, so a custom code.md or analysis.md needs a name of its own", description, path, line)
}
