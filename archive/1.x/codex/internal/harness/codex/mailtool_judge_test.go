package codex

import (
	"strings"
	"testing"

	"github.com/praline-labs/rewake/internal/harness"
)

// Reading the name check's answers (docs/mail-bridge-launch-codex.md): every
// sequence of layers config/read may answer, every list of -c values a caller
// may pass, judged by oracles written apart from the reading.

const sentinel = "S3CR3T-sentinel-value"

// layerKinds are every kind config/read names, and one it may name later.
var layerKinds = []string{
	layerSessionFlags, layerUser, layerProject, layerDefaults,
	"system", "mdm", "enterpriseManaged", "legacyManagedConfigTomlFromFile", "legacyManagedConfigTomlFromMdm",
	"aKindNotYetNamed",
}

// whereOracle is what a diagnostic names for a layer, as the rules' table
// states it.
func whereOracle(kind string, position int) harness.Where {
	file, folder := "/cfg/"+kind+".toml", "/work/.codex"
	switch kind {
	case layerSessionFlags:
		if position > 0 {
			return harness.Where{Flag: "-c", Position: position, Key: true}
		}
		return harness.Where{Scope: harness.ScopeSession}
	case layerUser:
		return harness.Where{Scope: harness.ScopeUser, Path: file}
	case layerProject:
		return harness.Where{Scope: harness.ScopeProject, Path: folder}
	case layerDefaults:
		return harness.Where{Scope: harness.ScopeDefaults, Path: file}
	case "aKindNotYetNamed":
		return harness.Where{Scope: harness.ScopeUnnamed}
	}
	return harness.Where{Scope: harness.ScopeManaged, Path: file}
}

func layerOf(kind string, named bool) configLayer {
	servers := map[string]any{"other": map[string]any{"command": sentinel}}
	if named {
		servers["rewake"] = map[string]any{"command": sentinel, "env": map[string]any{"K": sentinel}}
	}
	return configLayer{
		Name:   layerSource{Kind: kind, File: "/cfg/" + kind + ".toml", DotCodexFolder: "/work/.codex"},
		Config: map[string]any{"mcp_servers": servers},
	}
}

func TestEveryLayerSequenceNamesItsSource(t *testing.T) {
	type option struct {
		kind  string
		named bool
	}
	var options []option
	for _, kind := range layerKinds {
		options = append(options, option{kind, false}, option{kind, true})
	}
	sequences := [][]option{nil}
	level := [][]option{nil}
	for depth := 1; depth <= 3; depth++ {
		var next [][]option
		for _, prefix := range level {
			for _, each := range options {
				next = append(next, append(append([]option{}, prefix...), each))
			}
		}
		sequences = append(sequences, next...)
		level = next
	}
	argsWith := map[int][]string{
		0: {"-c", `model="x"`},
		2: {"-c", `model="x"`, "--config", `mcp_servers.rewake.command="` + sentinel + `"`},
	}
	cases, taken := 0, 0
	for _, sequence := range sequences {
		for _, effective := range []bool{false, true} {
			for position, args := range argsWith {
				var reply configReply
				reply.Layers = []configLayer{}
				var want *harness.Where
				for _, each := range sequence {
					reply.Layers = append(reply.Layers, layerOf(each.kind, each.named))
					if each.named && want == nil {
						where := whereOracle(each.kind, position)
						want = &where
					}
				}
				reply.Config = layerOf("x", effective).Config
				if want == nil && effective {
					want = &harness.Where{Scope: harness.ScopeDefaults}
				}
				got, named := takenWhere(reply, args)
				cases++
				switch {
				case want == nil && named:
					t.Fatalf("%v effective %v: a free name read as taken at %+v", sequence, effective, got)
				case want != nil && (!named || got != *want):
					t.Fatalf("%v effective %v args %q: got %+v %v, want %+v", sequence, effective, args, got, named, *want)
				case named && strings.Contains(got.String(), sentinel):
					t.Fatalf("the diagnostic carries a value: %s", got)
				}
				if named {
					taken++
				}
			}
		}
	}
	t.Logf("%d replies, %d with the name taken", cases, taken)
}

// configSpellings are the forms of -c a caller may write.
var configSpellings = []func(string) []string{
	func(v string) []string { return []string{"-c", v} },
	func(v string) []string { return []string{"-c" + v} },
	func(v string) []string { return []string{"-c=" + v} },
	func(v string) []string { return []string{"--config", v} },
	func(v string) []string { return []string{"--config=" + v} },
}

// configSettings are settings that name our table, and near misses.
var configSettings = []struct {
	value string
	named bool
}{
	{`mcp_servers.rewake.command="x"`, true},
	{`mcp_servers."rewake".command="x"`, true},
	{`mcp_servers.'rewake'.env.K="` + sentinel + `"`, true},
	{`mcp_servers . rewake . args=[]`, true},
	{`mcp_servers.rewake={command="x"}`, true},
	{`mcp_servers={rewake={command="x"}}`, true},
	{`mcp_servers.rewaker.command="x"`, false},
	{`mcp_servers.other.command="rewake"`, false},
	{`model="rewake"`, false},
	{`mcp_servers.re.wake=1`, false},
	{`mcp_servers={other={command="x"}}`, false},
}

func TestEveryConfigListNamesTheArgumentThatSetsTheTable(t *testing.T) {
	type item struct {
		words []string
		named bool
	}
	var items []item
	for _, spell := range configSpellings {
		for _, setting := range configSettings {
			items = append(items, item{spell(setting.value), setting.named})
		}
	}
	cases := 0
	var walk func(args []string, count, want int, depth int)
	walk = func(args []string, count, want, depth int) {
		for _, tail := range [][]string{nil, {"--", "-c", configSettings[0].value}} {
			line := append(append([]string{}, args...), tail...)
			if got := rewakeArgument(line); got != want {
				t.Fatalf("%q: got %d, want %d", line, got, want)
			}
			cases++
		}
		if depth == 3 {
			return
		}
		for _, each := range items {
			next := want
			if next == 0 && each.named {
				next = count + 1
			}
			walk(append(append([]string{}, args...), each.words...), count+1, next, depth+1)
		}
	}
	walk([]string{"-m", "x"}, 0, 0, 0)
	t.Logf("%d -c lists", cases)
}

func TestManagedRequirementsNamingServersKeepTheToolOut(t *testing.T) {
	for _, tc := range []struct {
		requirements map[string]any
		want         bool
	}{
		{map[string]any{"requirements": nil}, false},
		{map[string]any{"requirements": map[string]any{"allowedSandboxModes": []any{"read-only"}}}, false},
		{map[string]any{"requirements": map[string]any{"mcpServers": map[string]any{}}}, true},
		{map[string]any{"requirements": map[string]any{"mcp_servers": nil}}, false},
		{map[string]any{"requirements": map[string]any{"nested": []any{map[string]any{"allowedMcp": "x"}}}}, true},
		{map[string]any{"requirements": map[string]any{"MCP": false}}, true},
	} {
		if got := requiresMCP(tc.requirements); got != tc.want {
			t.Fatalf("%v: got %v", tc.requirements, got)
		}
	}
}
