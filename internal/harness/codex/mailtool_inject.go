package codex

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"time"

	"github.com/praline-labs/rewake/internal/harness"
)

// The injection check (docs/mail-bridge-launch-codex.md#the-injection-check-at-start-and-at-every-thread):
// at start and before each thread request is forwarded, the real server is
// asked what a thread in that directory would load, and only our entry, as
// we set it, may be named rewake. Its words are a closed list, like the name
// check's: no value of the person's configuration is ever shown.

// injectionBound bounds one check, at start or at a thread.
const injectionBound = 5 * time.Second

// toolInjection is what a run's checks compare against.
type toolInjection struct {
	upstream string
	// cwd is the server's own directory, a thread start's when it names none.
	cwd    string
	args   []string
	leaves []toolLeaf
	gates  harness.Gates
}

// injectionResult is a check's answer: a refusal, else whether tool reads
// are off and why.
type injectionResult struct {
	refusal  string
	readsOff string
}

// The refusals of the injection check, in the words it may show.
const (
	refusedKeyOffList    = "its config sets a key rewake does not yet let through"
	refusedFeatures      = "its features differ from the launch's -c values"
	refusedRoots         = "it names runtimeWorkspaceRoots"
	refusedCwdUnproven   = "it names a working directory, and whether that makes the server trust a project is not settled (gate G9)"
	refusedNoTrust       = "it names a working directory that holds no trust decision, which the server could take and then load another project layer"
	refusedNoCwd         = "it resumes or forks without naming a working directory, and which one the server takes is not settled (gate G3)"
	refusedNotOurs       = "the mail tool's entry holds a key rewake did not set"
	refusedRequirements  = "managed requirements name MCP servers, and which of them admit rewake's is not settled (gate G4)"
	refusedNotUnderstood = "the request could not be read"
)

// readsOffOmit, readsOffLimit and readsOffNoBound say why reads through the
// tool are off.
const (
	readsOffOmit    = "omit_tools_from is not in effect on the mail tool's server"
	readsOffLimit   = "tool_output_token_limit is below the smallest limit that keeps a read whole for this version, or not a whole number (L5)"
	readsOffNoBound = "tool_output_token_limit is set, and no smallest limit that keeps a read whole is calibrated for this version (L5)"
)

// terminalKeys are the top-level keys of a thread request's config the
// terminal of 0.159.0 sends (tui/src/app_server_session.rs,
// config_request_overrides_from_config): only these pass step 0.
var terminalKeys = map[string]bool{
	"allow_login_shell": true, "default_permissions": true, "features": true,
	"network": true, "permissions": true, "sandbox_workspace_write": true,
	"shell_environment_policy": true, "suppress_unstable_features_warning": true,
	"model_reasoning_effort": true, "model_reasoning_summary": true,
	"model_verbosity": true, "personality": true, "web_search": true,
	"bypass_hook_trust": true,
}

// threadRequest is what step 0 reads of a thread request's params.
type threadRequest struct {
	Config       map[string]json.RawMessage `json:"config"`
	Cwd          *string                    `json:"cwd"`
	RuntimeRoots json.RawMessage            `json:"runtimeWorkspaceRoots"`
}

// threadMethods are the requests that make a thread, and so load servers.
var threadMethods = map[string]bool{"thread/start": true, "thread/resume": true, "thread/fork": true}

// checkThread is step 0 and the check, for one thread request. It answers
// the refusal to send the terminal, or "" to forward the request.
func (t *toolInjection) checkThread(ctx context.Context, method string, params json.RawMessage) (injectionResult, bool) {
	if !threadMethods[method] {
		return injectionResult{}, false
	}
	var request threadRequest
	if len(params) > 0 && json.Unmarshal(params, &request) != nil {
		return injectionResult{refusal: refusedNotUnderstood}, true
	}
	if refusal := stepZero(request); refusal != "" {
		return injectionResult{refusal: refusal}, true
	}
	// A cwd present as a string is named even when empty: the server takes
	// it as a path relative to its own directory, and its trust branch asks
	// only whether one was given (thread_processor.rs at rust-v0.159.0).
	named := request.Cwd != nil
	cwd := t.cwd
	switch {
	case named:
		if t.gates.Open(harness.GateG9) {
			return injectionResult{refusal: refusedCwdUnproven}, true
		}
		cwd = resolveCwd(t.cwd, *request.Cwd)
	case method != "thread/start" && t.gates.Open(harness.GateG3):
		return injectionResult{refusal: refusedNoCwd}, true
	}
	result := t.check(ctx, cwd, func(reply configReply) string {
		if !sameFeatures(request.Config["features"], sessionFlags(reply)) {
			return refusedFeatures
		}
		if named && !trustDecided(reply.Config, cwd) {
			return refusedNoTrust
		}
		return ""
	})
	return result, true
}

// resolveCwd is a request's cwd as the server takes it: an absolute path as
// it is, any other relative to the server's own directory.
func resolveCwd(own, requested string) string {
	if filepath.IsAbs(requested) {
		return filepath.Clean(requested)
	}
	return filepath.Join(own, requested)
}

// stepZero is what the request alone decides: its config keys and the
// workspace roots the terminal never sends.
func stepZero(request threadRequest) string {
	if len(request.RuntimeRoots) > 0 && string(request.RuntimeRoots) != "null" {
		return refusedRoots
	}
	for key := range request.Config {
		if !terminalKeys[key] {
			return refusedKeyOffList
		}
	}
	return ""
}

// check asks the real server for the configuration a thread in cwd loads
// and runs steps 1, 2 and 4; extra, at a thread, adds what step 0 needs of
// the answer. Step 3 is gate G2's call: a run reaches here only when G2 is
// taken as closed.
func (t *toolInjection) check(ctx context.Context, cwd string, extra func(configReply) string) injectionResult {
	ctx, cancel := context.WithTimeout(ctx, injectionBound)
	defer cancel()
	client, err := connectRPC(ctx, t.upstream, nil)
	if err != nil {
		return injectionResult{refusal: checkFailed(err)}
	}
	defer client.close()
	var raw, rawRequirements json.RawMessage
	if err := client.call(ctx, "config/read", map[string]any{"cwd": cwd, "includeLayers": true}, &raw); err != nil {
		return injectionResult{refusal: checkFailed(err)}
	}
	if err := client.call(ctx, "configRequirements/read", map[string]any{}, &rawRequirements); err != nil {
		return injectionResult{refusal: checkFailed(err)}
	}
	reply, ok := parseConfigReply(raw)
	var requirements map[string]any
	if !ok || json.Unmarshal(rawRequirements, &requirements) != nil || requirements == nil {
		return injectionResult{refusal: checkFailed(nil)}
	}
	if extra != nil {
		if refusal := extra(reply); refusal != "" {
			return injectionResult{refusal: refusal}
		}
	}
	return t.judge(reply, requirements)
}

// judge runs steps 1, 2 and 4 on an answer.
func (t *toolInjection) judge(reply configReply, requirements map[string]any) injectionResult {
	for _, layer := range reply.Layers {
		entry, named := rewakeEntry(layer.Config)
		if !named {
			continue
		}
		// Another layer refuses by naming the server at all, whatever its
		// values, an empty table too; the session flags only by what is left
		// once our leaves are out, an empty table counted as a leaf.
		taken := injectionResult{refusal: "another MCP server named rewake is configured for it (" + layerWhere(layer.Name, t.args).String() + ")"}
		if layer.Name.Kind != layerSessionFlags {
			return taken
		}
		leaves := map[string]any{}
		flattenTables("", entry, leaves)
		t.removeOurs(leaves)
		if len(leaves) > 0 {
			return taken
		}
	}
	entry, named := rewakeEntry(reply.Config)
	if !named {
		return injectionResult{refusal: refusedNotOurs}
	}
	effective := map[string]any{}
	flatten("", entry, effective)
	var readsOff string
	for _, leaf := range t.leaves {
		value, present := effective[leaf.leafPath()]
		delete(effective, leaf.leafPath())
		switch {
		case leaf.leafPath() == "omit_tools_from" && (!present || !sameJSON(value, leaf.value)):
			readsOff = readsOffOmit
		case !present || !sameJSON(value, leaf.value):
			return injectionResult{refusal: refusedNotOurs}
		}
	}
	for key, value := range effective {
		if !replyDefault(key, value) {
			return injectionResult{refusal: refusedNotOurs}
		}
	}
	if requiresMCP(requirements) {
		return injectionResult{refusal: refusedRequirements}
	}
	if limit, set := reply.Config["tool_output_token_limit"]; set && limit != nil && readsOff == "" {
		readsOff = t.limitReadsOff(limit)
	}
	return injectionResult{readsOff: readsOff}
}

// limitReadsOff takes a set tool_output_token_limit against the bound L5
// calibrated for the launch's version: a whole number at or above it keeps
// reads on; any other value, or any value at all without a bound, turns
// them off (docs/mail-bridge-launch-codex.md#the-output-limit).
func (t *toolInjection) limitReadsOff(limit any) string {
	bound, calibrated := t.gates.OutputBound()
	if !calibrated {
		return readsOffNoBound
	}
	value, number := limit.(float64)
	if !number || value != math.Trunc(value) || value < float64(bound) {
		return readsOffLimit
	}
	return ""
}

// removeOurs takes our leaves out of the session flags' entry, each only by
// its key and our value: one the caller set to anything else stays.
func (t *toolInjection) removeOurs(leaves map[string]any) {
	for _, leaf := range t.leaves {
		if value, present := leaves[leaf.leafPath()]; present && sameJSON(value, leaf.value) {
			delete(leaves, leaf.leafPath())
		}
	}
}

// replyDefault is a field the reply fills in for any entry (probe 2,
// 0.159.0): enabled, the local environment, or a field left null.
func replyDefault(key string, value any) bool {
	switch key {
	case "enabled":
		return value == true
	case "environment_id":
		return value == "local"
	}
	return value == nil
}

// sessionFlags is the configuration the -c values make, ours among them.
func sessionFlags(reply configReply) map[string]any {
	for _, layer := range reply.Layers {
		if layer.Name.Kind == layerSessionFlags {
			return layer.Config
		}
	}
	return nil
}

// sameFeatures says whether a request's features set only what the
// caller's -c values set, each leaf to the same value.
func sameFeatures(raw json.RawMessage, flags map[string]any) bool {
	if len(raw) == 0 || string(raw) == "null" {
		return true
	}
	var requested any
	if json.Unmarshal(raw, &requested) != nil {
		return false
	}
	if _, table := requested.(map[string]any); !table {
		return false
	}
	asked, given := map[string]any{}, map[string]any{}
	flatten("", requested, asked)
	if features, ok := flags["features"]; ok {
		flatten("", features, given)
	}
	for key, value := range asked {
		if mine, ok := given[key]; !ok || !sameJSON(value, mine) {
			return false
		}
	}
	return true
}

// trustDecided says whether the effective configuration holds a trust
// decision for cwd or its repository root, under the key Codex uses — the
// absolute path, as gate G9 is to confirm for each version.
func trustDecided(config map[string]any, cwd string) bool {
	projects, _ := config["projects"].(map[string]any)
	for _, dir := range []string{filepath.Clean(cwd), repositoryRoot(cwd)} {
		if project, ok := projects[dir].(map[string]any); ok && dir != "" {
			if level, ok := project["trust_level"].(string); ok && level != "" {
				return true
			}
		}
	}
	return false
}

// repositoryRoot is the nearest directory at or above dir holding .git.
func repositoryRoot(dir string) string {
	for dir = filepath.Clean(dir); ; {
		if _, err := os.Lstat(filepath.Join(dir, ".git")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return ""
		}
		dir = parent
	}
}

// checkFailed words a check that could not answer, by its class alone.
func checkFailed(err error) string {
	var answered *rpcError
	outcome := harness.OutcomeUnrecognized
	switch {
	case err == nil, errors.As(err, &answered):
	case errors.Is(err, context.DeadlineExceeded):
		outcome = harness.OutcomeBound
	default:
		outcome = harness.OutcomeUnreadable
	}
	return "the check of its configuration failed: config/read: " + outcome
}
