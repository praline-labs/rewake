package pi

import (
	"fmt"
	"maps"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// The extension's wire is pinned word by word (brief 94): what it sends is a
// reading of pi, an outcome of a native call or pi's own event, and what the
// wrapper makes of them is Go's. A new op or field fails here until it is
// listed; a reading the wrapper would project goes raw inside native rather
// than as a field of its own. pi's events (raw) are pi's words, carried as
// they come, and the translator's alone.

// frameSays is each type of the frame and the fields it carries.
var frameSays = map[string]string{
	"frame":     "action arguments argv code error events focus from id isError levels mirror native notice ok op outcome pending protocol ran refused request sequence state steered text thread timeoutMs tool tools turn",
	"sample":    "compacting idle native sequence",
	"toolEntry": "description inputSchema label name",
	"Ran":       "error exitCode killed stderr stdout stdoutCut",
}

// frameOps are the ops the extension sends or answers.
var frameOps = "answer closing compacted compaction-result control events hello interrupt prompt state thinking tool-plan tool-result tools trace turn-end turn-start wake"

// words are a type's fields as JSON names them, and those of the types they
// hold, but pi's events.
func words(t reflect.Type, says map[string]string) {
	for t.Kind() == reflect.Pointer || t.Kind() == reflect.Slice {
		t = t.Elem()
	}
	if t.Kind() != reflect.Struct || t == reflect.TypeFor[raw]() {
		return
	}
	if _, seen := says[t.Name()]; seen {
		return
	}
	var fields []string
	says[t.Name()] = ""
	for i := range t.NumField() {
		f := t.Field(i)
		fields = append(fields, strings.Split(f.Tag.Get("json"), ",")[0])
		words(f.Type, says)
	}
	slices.Sort(fields)
	says[t.Name()] = strings.Join(fields, " ")
}

var opNamed = regexp.MustCompile(`(?:\bop:\s*|\.op\s*[!=]==\s*)"([a-z-]+)"`)

// opsOf are the ops the extension's files name, but a check of the op's type.
func opsOf(files []string) string {
	ops := map[string]bool{}
	for _, text := range files {
		for line := range strings.SplitSeq(text, "\n") {
			if strings.Contains(line, "typeof") || strings.HasPrefix(strings.TrimSpace(line), "//") {
				continue
			}
			for _, m := range opNamed.FindAllStringSubmatch(line, -1) {
				ops[m[1]] = true
			}
		}
	}
	return strings.Join(slices.Sorted(maps.Keys(ops)), " ")
}

func unlistedFrame(t reflect.Type, ops string) []string {
	says := map[string]string{}
	words(t, says)
	var out []string
	for _, name := range slices.Sorted(maps.Keys(says)) {
		if frameSays[name] != says[name] {
			out = append(out, fmt.Sprintf("the frame's %s says {%s}, listed {%s}: a reading the wrapper projects goes raw inside native", name, says[name], frameSays[name]))
		}
	}
	if ops != frameOps {
		out = append(out, fmt.Sprintf("the extension's ops are {%s}, listed {%s}", ops, frameOps))
	}
	return out
}

func TestTheExtensionSendsOnlyTheListedWords(t *testing.T) {
	var files []string
	for _, name := range modules {
		text, err := extensions.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		files = append(files, string(text))
	}
	if got := unlistedFrame(reflect.TypeFor[frame](), opsOf(files)); len(got) != 0 {
		t.Fatal(strings.Join(got, "\n"))
	}
	// A field the Статус would read, and an op of its own, are caught.
	type grown struct {
		frame
		Percent int `json:"percent"`
	}
	if got := unlistedFrame(reflect.TypeFor[grown](), frameOps); len(got) == 0 {
		t.Error("a frame grown by a field passed")
	}
	more := append(slices.Clone(files), `    if (frame.op === "level") respond({ ok: true, level: pi.getThinkingLevel() });`)
	if got := unlistedFrame(reflect.TypeFor[frame](), opsOf(more)); len(got) != 1 {
		t.Errorf("an op of its own: %q", got)
	}
}
