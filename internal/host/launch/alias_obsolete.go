package launch

// The worker type is gone (the owner's decision, November 3, 2026) and what
// it left in the files is obsolete: an alias's `type`, a [types] section, a
// types/ directory. Refusing them refused every launch in a project that
// still carried one, so main could not start to be told what to fix. They are
// skipped with a migration notice instead — the file, the line where TOML
// gives one, and what to write — and the rest of the catalog runs
// (docs/work-modes.md#the-old-worker-types).
//
// A template whose `type` is skipped is left out until it is fixed: read
// under a default it would start as something its person did not write. What
// rewake cannot recognize — malformed TOML, an unknown field, a malformed
// value — still refuses the file.

import (
	"fmt"

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

// degrade takes every obsolete setting out of the decoded file, records one
// migration notice each, and answers the fields that remain unknown.
func (s *Set) degrade(file *aliasFile, fields []toml.DecodeError, description, path string) []toml.DecodeError {
	var unknown []toml.DecodeError
	typed := map[string]bool{}
	section := false
	for _, field := range fields {
		alias, types := obsoleteName(field.Key())
		switch {
		case alias != "":
			if typed[alias] {
				continue
			}
			typed[alias] = true
			line, _ := field.Position()
			notice := obsoleteAliasNotice(description, path, line, alias)
			delete(file.Alias, alias)
			s.omitted[alias] = notice
			s.Migrations = append(s.Migrations, notice)
		case types:
			if section {
				continue
			}
			section = true
			line, _ := field.Position()
			s.Migrations = append(s.Migrations, obsoleteTypesNotice(description, path, line))
		default:
			unknown = append(unknown, field)
		}
	}
	return unknown
}

// obsoleteAliasNotice says what to write for a template that still carries the
// old type, and that it is left out until then.
func obsoleteAliasNotice(description, path string, line int, name string) string {
	return fmt.Sprintf("%s at %s:%d: [alias.%s] still has the obsolete type field, so that template is left out until it is fixed: a worker's kind of work is now its runtime work mode, so put start_mode = \"<mode>\" under [alias.%s.pi] (code became build, analysis advisor; review and process keep their names)", description, path, line, name, name)
}

// obsoleteTypesNotice says where the text of an old [types] section goes.
func obsoleteTypesNotice(description, path string, line int) string {
	return fmt.Sprintf("%s at %s:%d: the [types] section is no longer read and is skipped: a worker's kind of work is its runtime work mode, a file under modes/<name>.md (built-in: advisor, build, review, process); move each type's text there, keeping its front matter, and code is now build and analysis is advisor, so a custom code.md or analysis.md needs a name of its own", description, path, line)
}
