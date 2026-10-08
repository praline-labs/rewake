package codex

import (
	"encoding/json"
	"strings"

	"github.com/praline-labs/rewake/internal/harness"
)

// Reading config/read's answer with its layers (schema of 0.159.0,
// ConfigReadResponse): where a server named rewake comes from, in the words a
// diagnostic may use, and the leaves of an entry to compare with ours.

// configReply is config/read's answer with includeLayers.
type configReply struct {
	Config map[string]any `json:"config"`
	Layers []configLayer  `json:"layers"`
}

// configLayer is one layer; a layer off (disabledReason set) still counts,
// since trusting its folder turns it on.
type configLayer struct {
	Name   layerSource    `json:"name"`
	Config map[string]any `json:"config"`
}

// layerSource is ConfigLayerSource: its kind and the file or folder that
// names it.
type layerSource struct {
	Kind           string `json:"type"`
	File           string `json:"file"`
	DotCodexFolder string `json:"dotCodexFolder"`
}

// The layer kinds config/read names.
const (
	layerSessionFlags = "sessionFlags"
	layerUser         = "user"
	layerProject      = "project"
	layerDefaults     = "packagedDefaults"
)

// managedLayers are the kinds a diagnostic calls managed.
var managedLayers = map[string]bool{
	"system": true, "mdm": true, "enterpriseManaged": true,
	"legacyManagedConfigTomlFromFile": true, "legacyManagedConfigTomlFromMdm": true,
}

func parseConfigReply(raw []byte) (configReply, bool) {
	var reply configReply
	if json.Unmarshal(raw, &reply) != nil || reply.Config == nil || reply.Layers == nil {
		return configReply{}, false
	}
	return reply, true
}

// rewakeEntry is a configuration's mcp_servers.rewake, if it has one.
func rewakeEntry(config map[string]any) (any, bool) {
	servers, ok := config["mcp_servers"].(map[string]any)
	if !ok {
		return nil, false
	}
	entry, ok := servers["rewake"]
	return entry, ok
}

// flatten lists an entry's leaves by dotted path; an array is one leaf.
func flatten(prefix string, value any, out map[string]any) { walkLeaves(prefix, value, out, false) }

// flattenTables is flatten that also lists an empty table as a leaf of its
// own: a layer's entry names whatever it holds, nothing included.
func flattenTables(prefix string, value any, out map[string]any) {
	walkLeaves(prefix, value, out, true)
}

func walkLeaves(prefix string, value any, out map[string]any, empty bool) {
	table, ok := value.(map[string]any)
	if !ok || empty && len(table) == 0 {
		out[prefix] = value
		return
	}
	for key, inner := range table {
		path := key
		if prefix != "" {
			path = prefix + "." + key
		}
		walkLeaves(path, inner, out, empty)
	}
}

// leafPath is a leaf's path below mcp_servers.rewake, as flatten names it.
func (l toolLeaf) leafPath() string { return strings.Join(l.path, ".") }

// layerWhere names a layer for a diagnostic. The session flags are named by
// the caller's -c argument that sets the entry, when one can be told.
func layerWhere(source layerSource, args []string) harness.Where {
	switch {
	case source.Kind == layerSessionFlags:
		if position := rewakeArgument(args); position > 0 {
			return harness.Where{Flag: configFlag, Position: position, Key: true}
		}
		return harness.Where{Scope: harness.ScopeSession}
	case source.Kind == layerUser:
		return harness.Where{Scope: harness.ScopeUser, Path: source.File}
	case source.Kind == layerProject:
		return harness.Where{Scope: harness.ScopeProject, Path: source.DotCodexFolder}
	case source.Kind == layerDefaults:
		return harness.Where{Scope: harness.ScopeDefaults, Path: source.File}
	case managedLayers[source.Kind]:
		return harness.Where{Scope: harness.ScopeManaged, Path: source.File}
	}
	return harness.Where{Scope: harness.ScopeUnnamed}
}

// rewakeArgument is the position, among the caller's -c values, of the
// first that sets mcp_servers.rewake: the key alone is read, never the
// value. A table under mcp_servers as a whole counts when its text names
// rewake; that over-covers rather than misses.
func rewakeArgument(args []string) int {
	for index, setting := range harness.FlagValues(args, configFlag, "--config") {
		key, value, _ := strings.Cut(setting, "=")
		key = strings.NewReplacer(`"`, "", "'", "", " ", "", "\t", "").Replace(key)
		if key == "mcp_servers.rewake" || strings.HasPrefix(key, "mcp_servers.rewake.") ||
			(key == "mcp_servers" && strings.Contains(value, "rewake")) {
			return index + 1
		}
	}
	return 0
}

// takenWhere finds a server named rewake in a reply that holds none of ours:
// any layer naming it, on or off, else the effective table alone, which only
// the packaged defaults fill.
func takenWhere(reply configReply, args []string) (harness.Where, bool) {
	for _, layer := range reply.Layers {
		if _, named := rewakeEntry(layer.Config); named {
			return layerWhere(layer.Name, args), true
		}
	}
	if _, named := rewakeEntry(reply.Config); named {
		return harness.Where{Scope: harness.ScopeDefaults}, true
	}
	return harness.Where{}, false
}

// requiresMCP says whether managed requirements say anything about MCP
// servers. Their exact fields are gate G4; until it closes, any key naming
// MCP with a value set is taken as one that may not admit ours.
func requiresMCP(value any) bool {
	switch typed := value.(type) {
	case map[string]any:
		for key, inner := range typed {
			if inner != nil && strings.Contains(strings.ToLower(key), "mcp") {
				return true
			}
			if requiresMCP(inner) {
				return true
			}
		}
	case []any:
		for _, inner := range typed {
			if requiresMCP(inner) {
				return true
			}
		}
	}
	return false
}
