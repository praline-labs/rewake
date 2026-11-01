package update

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const changelog = `# Changelog

What each release changed, by part.

## 2.13.0
### shell
- The banner of a newer release.
### rewake
- rewake update.
  Installs exactly the version it names.

## 2.12.0
### face
- Faster.

## 2.11.0
### rewake
- Older.
`

// The entries between what runs and the target are those after the running
// release, up to the target, newest first.
func TestChangelogBetweenTwoReleases(t *testing.T) {
	releases, err := ParseChangelog([]byte(changelog))
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, r := range Between(releases, "2.11.0", "2.13.0") {
		got = append(got, r.Version)
	}
	if want := []string{"2.13.0", "2.12.0"}; !reflect.DeepEqual(got, want) {
		t.Errorf("between 2.11.0 and 2.13.0: %v, want %v", got, want)
	}
	if got := Between(releases, "2.12.0", "2.12.0"); len(got) != 0 {
		t.Errorf("between a release and itself: %v", got)
	}
	newest, _ := Find(releases, "2.13.0")
	want := []Change{{"shell", []string{"The banner of a newer release."}}, {"rewake", []string{"rewake update. Installs exactly the version it names."}}}
	if !reflect.DeepEqual(newest.Parts, want) {
		t.Errorf("2.13.0 reads %+v, want %+v", newest.Parts, want)
	}
}

// A changelog the command would show half of fails as a whole.
func TestChangelogOutOfShapeFails(t *testing.T) {
	for text, want := range map[string]string{
		"## 2.12.0\n- no part\n":                    "a change before its part",
		"## 2.11.0\n### shell\n- a\n## 2.12.0\n":    "newest first",
		"## next\n":                                 "not a release's version",
		"## 2.12.0\n### shell\nloose\n":             "not a release, a part or a change",
		"## 2.12.0\n### shell\n  continued alone\n": "no change before it",
	} {
		if _, err := ParseChangelog([]byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: %v, want %q", text, err, want)
		}
	}
}

// The repository's changelog reads, so a release is not the first to find
// an entry out of shape.
func TestTheRepositorysChangelogReads(t *testing.T) {
	data, err := os.ReadFile(filepath.Join("..", "..", "..", ChangelogFile))
	if err != nil {
		t.Fatal(err)
	}
	releases, err := ParseChangelog(data)
	if err != nil || len(releases) == 0 || len(releases[0].Parts) == 0 {
		t.Fatalf("%s: %d releases, %v", ChangelogFile, len(releases), err)
	}
}
