package sessiontools

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
)

func decode(t *testing.T, text string) map[string]any {
	t.Helper()
	var input map[string]any
	if err := json.Unmarshal([]byte(text), &input); err != nil {
		t.Fatal(err)
	}
	return input
}

func tool(t *testing.T, name string) Tool {
	t.Helper()
	found, ok := Named(name)
	if !ok {
		t.Fatalf("no tool %s", name)
	}
	return found
}

func TestEachSchemaIsClosedAndRequiresOnlyWhatItDescribes(t *testing.T) {
	seen := map[string]bool{}
	for _, tool := range all {
		if seen[tool.Name] {
			t.Fatalf("%s twice", tool.Name)
		}
		seen[tool.Name] = true
		var schema struct {
			Properties           map[string]any `json:"properties"`
			Required             []string       `json:"required"`
			AdditionalProperties *bool          `json:"additionalProperties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatal(err)
		}
		if schema.AdditionalProperties == nil || *schema.AdditionalProperties {
			t.Fatalf("%s's schema is open", tool.Name)
		}
		for _, name := range schema.Required {
			if _, ok := schema.Properties[name]; !ok {
				t.Fatalf("%s requires %s it does not describe", tool.Name, name)
			}
		}
		if tool.Description == "" || tool.Label == "" || tool.what == "" || tool.timeout <= 0 {
			t.Fatalf("%s is not whole", tool.Name)
		}
	}
}

func TestARoleIsOfferedItsOwnTools(t *testing.T) {
	names := func(tools []Tool) string {
		var out []string
		for _, tool := range tools {
			out = append(out, tool.Name)
		}
		return strings.Join(out, " ")
	}
	for _, c := range []struct {
		role  string
		enter bool
		want  string
	}{
		{"main", false, "worktree_status worktree_list worktree_land"},
		{"worker", false, "worktree_status brief_read brief_tick brief_comment"},
		{"worker", true, "worktree_status brief_read brief_tick brief_comment worktree_enter"},
		{"", false, "worktree_status brief_read brief_tick brief_comment"},
	} {
		if got := names(Offered(c.role, c.enter)); got != c.want {
			t.Fatalf("%s enter=%v: %s, want %s", c.role, c.enter, got, c.want)
		}
	}
}

func TestTheToolsMakeTheirCommandsArguments(t *testing.T) {
	for _, c := range []struct {
		tool, input string
		want        []string
	}{
		{"worktree_status", `{}`, []string{"worktree", "status", "--json"}},
		{"worktree_list", `{}`, []string{"worktree", "ls", "--this-room", "--json"}},
		{"worktree_land", `{"name":"fix-login","into":"release"}`, []string{"worktree", "land", "fix-login", "--into=release", "--json"}},
		{"worktree_land", `{"name":"rewake/fix-login"}`, []string{"worktree", "land", "rewake/fix-login", "--json"}},
		{"worktree_enter", `{}`, []string{"worktree", "enter", "--json"}},
		{"worktree_enter", `{"name":"fix"}`, []string{"worktree", "enter", "fix", "--json"}},
		{"brief_read", `{"id":3}`, []string{"brief", "show", "3"}},
		{"brief_tick", `{"id":3,"item":1,"edition":2}`, []string{"brief", "tick", "3", "1", "--edition=2"}},
		{"brief_tick", `{"id":3,"item":1,"edition":2,"undo":false}`, []string{"brief", "tick", "3", "1", "--edition=2"}},
		{"brief_tick", `{"id":3,"item":1,"edition":2,"undo":true}`, []string{"brief", "tick", "3", "1", "--edition=2", "--undo"}},
		// The text goes after --, so one that begins with a dash is no flag.
		{"brief_comment", `{"id":3,"text":"--force was not needed"}`, []string{"brief", "comment", "3", "--", "--force was not needed"}},
	} {
		args, refused := tool(t, c.tool).args(decode(t, c.input))
		if refused != "" || !slices.Equal(args, c.want) {
			t.Fatalf("%s %s: %q %q, want %q", c.tool, c.input, args, refused, c.want)
		}
	}
}

// An empty into once landed into the default branch (2afdce1).
func TestAnInputThatWouldBeReadAsAFlagOrLeftOutIsRefused(t *testing.T) {
	for _, c := range []struct{ tool, input, want string }{
		{"worktree_land", `{}`, "worktree_land needs name, the worktree's name from worktree_list"},
		{"worktree_land", `{"name":""}`, "worktree_land needs name, the worktree's name from worktree_list"},
		{"worktree_land", `{"name":"--force"}`, "worktree_land needs name, the worktree's name from worktree_list"},
		{"worktree_land", `{"name":3}`, "worktree_land needs name, the worktree's name from worktree_list"},
		{"worktree_land", `{"name":"x","into":"-f"}`, "into must be a branch name; leave it out for the default"},
		{"worktree_land", `{"name":"x","into":""}`, "into must be a branch name; leave it out for the default"},
		{"worktree_land", `{"name":"x","into":4}`, "into must be a branch name; leave it out for the default"},
		{"worktree_enter", `{"name":"-b"}`, "name must be a worktree name; leave it out for a generated one"},
		{"worktree_enter", `{"name":""}`, "name must be a worktree name; leave it out for a generated one"},
		{"brief_read", `{}`, "brief_read needs id, the brief's number from 1"},
		{"brief_read", `{"id":0}`, "brief_read needs id, the brief's number from 1"},
		{"brief_read", `{"id":"3"}`, "brief_read needs id, the brief's number from 1"},
		{"brief_read", `{"id":-1}`, "brief_read needs id, the brief's number from 1"},
		{"brief_tick", `{"id":3,"item":1.5,"edition":2}`, "brief_tick needs item, the item's number from 1 as brief_read shows it"},
		{"brief_tick", `{"id":3,"item":0,"edition":2}`, "brief_tick needs item, the item's number from 1 as brief_read shows it"},
		{"brief_tick", `{"id":3,"item":1}`, "brief_tick needs edition, the edition brief_read named"},
		{"brief_tick", `{"id":3,"item":1,"edition":2,"undo":"yes"}`, "undo is true or false"},
		{"brief_comment", `{"id":3}`, "brief_comment needs text, the comment"},
		{"brief_comment", `{"id":3,"text":"  "}`, "brief_comment needs text, the comment"},
		{"brief_comment", `{"id":3,"text":5}`, "brief_comment needs text, the comment"},
	} {
		args, refused := tool(t, c.tool).args(decode(t, c.input))
		if refused != c.want || args != nil {
			t.Fatalf("%s %s: %q %q, want refused %q", c.tool, c.input, args, refused, c.want)
		}
	}
}

func TestEachToolIsBoundedLandPastItsOwnMinutesWaitForAHook(t *testing.T) {
	for name, want := range map[string]int64{
		"worktree_status": 60_000, "worktree_list": 60_000, "worktree_land": 120_000,
		"worktree_enter": 120_000, "brief_read": 60_000, "brief_tick": 60_000, "brief_comment": 60_000,
	} {
		if got := tool(t, name).timeout.Milliseconds(); got != want {
			t.Fatalf("%s is bounded at %d ms, want %d", name, got, want)
		}
	}
}

// The table goes to an agent as data: its JSON names what a harness registers
// and nothing of the command behind it.
func TestTheTableEncodesWhatAnAgentRegisters(t *testing.T) {
	data, err := json.Marshal(Offered("main", false)[2:])
	if err != nil {
		t.Fatal(err)
	}
	var got []map[string]json.RawMessage
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	keys := slices.Sorted(func(yield func(string) bool) {
		for key := range got[0] {
			if !yield(key) {
				return
			}
		}
	})
	if !slices.Equal(keys, []string{"description", "inputSchema", "label", "name"}) {
		t.Fatalf("a table entry holds %v", keys)
	}
	// The model reads the properties in the order they are described.
	if schema := string(got[0]["inputSchema"]); strings.Index(schema, `"name"`) > strings.Index(schema, `"into"`) {
		t.Fatalf("worktree_land's schema reordered: %s", schema)
	}
	if Digest(Offered("main", false)) == Digest(Offered("worker", false)) {
		t.Fatal("two tables share a digest")
	}
}
