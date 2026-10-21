package worktype

import (
	"strings"
	"testing"
)

// The built-in types parse — BuiltIns panics on one that does not — and are
// the first set main was given.
func TestTheBuiltInTypes(t *testing.T) {
	builtins := BuiltIns()
	if got := strings.Join(Names(builtins), ","); got != "analysis,code,process,review" {
		t.Fatalf("built-in types: %s", got)
	}
	for _, value := range builtins {
		if value.Source != BuiltIn || value.Path != "" || value.Text == "" || strings.Contains(value.Text, "+++") {
			t.Fatalf("%+v", value)
		}
	}
}

// A type's file is refused, never half read, when it is not what it should
// be: a worker told nothing or a broken text is worse than a refusal at
// space open.
func TestATypeFileIsReadOrRefused(t *testing.T) {
	good, err := Parse("study", []byte("+++\r\ndescription = \" Look things up \"\r\n+++\r\n\r\nRead first.\r\nThen answer.\r\n"))
	if err != nil || good.Description != "Look things up" || good.Text != "Read first.\nThen answer." {
		t.Fatalf("%+v %v", good, err)
	}
	for _, c := range []struct{ name, raw, refused string }{
		{".hidden", "+++\ndescription = \"x\"\n+++\ntext", "cannot name a type"},
		{"study", "description = \"x\"\ntext", "front matter"},
		{"study", "+++\ndescription = \"x\"\ntext", "front matter"},
		{"study", "+++\ndescription = \"x\"\nrole = \"write\"\n+++\ntext", "field a type does not"},
		{"study", "+++\ndescription = [\n+++\ntext", "not TOML"},
		{"study", "+++\n\n+++\ntext", "needs description"},
		{"study", "+++\ndescription = \"x\"\n+++\n \n", "no text"},
		{"study", "+++\ndescription = \"x\"\n+++\n" + strings.Repeat("a", MaxSize), "KiB"},
	} {
		if _, err := Parse(c.name, []byte(c.raw)); err == nil || !strings.Contains(err.Error(), c.refused) {
			t.Errorf("%s %q: %v, want %q", c.name, c.raw, err, c.refused)
		}
	}
}
