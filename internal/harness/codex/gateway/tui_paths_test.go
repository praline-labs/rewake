package gateway

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// tuiPath is one terminal's traffic through the gateway as a live probe of
// September 26, 2026 recorded it, a real terminal and app-server of each
// version in a container without a network: testdata/tui-paths holds its
// projection — the lifecycle requests with their parameters, the replies'
// routing fields — with each record's line in the recording. The model list
// and the notifications recorded without parameters are left out; neither
// selects anything. checks are the probe's own checkpoints: the recording's
// length when the terminal had started, run /new and run /resume.
type tuiPath struct {
	name   string
	checks []tuiCheck
}

type tuiCheck struct {
	label  string
	after  int
	thread string
}

const seededThread = "01a0dea0-9d0f-7850-a072-27682e647d7e"

// unlisted fills a loaded list's reply that the recording kept without its ids:
// the probe's first recordings did not keep them. Its ids are the two
// conversations the terminal had loaded then, as the terminal's next reads and
// the same step recorded with its ids on 0.157.1 show; without them the
// terminal's reads after its /resume would be refused as unexplained.
var unlisted = map[string]map[int][]string{
	"0.155.1-interactive": {88: {seededThread, "01a0dea0-a907-7a71-a136-0e1d9c0d3aae"}},
}

var tuiPaths = []tuiPath{
	{"0.155.1-interactive", []tuiCheck{{"startup", 36, seededThread}, {"new", 62, "01a0dea0-a907-7a71-a136-0e1d9c0d3aae"}, {"resume", 98, seededThread}}},
	{"0.155.1-launch-resume", []tuiCheck{{"resume", 47, seededThread}}},
	// /resume of a conversation that ran no turn fails natively ("no rollout
	// found"), so the terminal is left with no selection proved.
	{"0.157.1-interactive", []tuiCheck{{"startup", 36, "01a0dea0-2099-74c2-ab58-5d93934e4cef"}, {"new", 49, "01a0dea0-32d7-7512-8a6a-ec5abdc6c8f4"}, {"resume", 53, ""}}},
	{"0.157.1-seeded-interactive", []tuiCheck{{"startup", 47, seededThread}, {"new", 60, "01a0dea1-f01f-77c1-b9c6-a7977f5f63e5"}, {"resume", 85, seededThread}}},
	{"0.157.1-seeded-launch-resume", []tuiCheck{{"resume", 47, seededThread}}},
}

// Each path the probe drove selects what the terminal selected, on both
// versions: the 0.157.1 ones are what the gateway of that day refused.
func TestTheTerminalsPathsSelectOnBothVersions(t *testing.T) {
	for _, path := range tuiPaths {
		t.Run(path.name, func(t *testing.T) {
			g, ui, peers, _ := setup(t)
			native := <-peers
			defer func() { _ = native.conn.Close() }()
			checks := path.checks
			for _, record := range tuiRecords(t, path.name) {
				for len(checks) > 0 && record.line > checks[0].after {
					tuiCheckpoint(t, g, checks[0])
					checks = checks[1:]
				}
				if record.fromTUI {
					write(t, ui, record.raw)
					readWithin(t, native)
				} else {
					write(t, native, record.raw)
					readWithin(t, ui)
				}
			}
			for _, check := range checks {
				tuiCheckpoint(t, g, check)
			}
		})
	}
}

func tuiCheckpoint(t *testing.T, g *Gateway, check tuiCheck) {
	t.Helper()
	b := g.Binding()
	if check.thread == "" && b.Ready || check.thread != "" && (!b.Ready || b.Thread != check.thread) {
		t.Errorf("after %s: %+v, want thread %q", check.label, b, check.thread)
	}
}

type tuiRecord struct {
	line    int
	fromTUI bool
	raw     []byte
}

// tuiRecords rebuilds the messages of a recording. A reply's fields were kept
// apart from the reply's shape, so it is put back: a thread under result, the
// ids of a list as the list — plain ids for the loaded list, as the server
// sends them, objects for the others.
func tuiRecords(t *testing.T, name string) []tuiRecord {
	t.Helper()
	file, err := os.Open(filepath.Join("testdata", "tui-paths", name+".jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	var out []tuiRecord
	methods := map[string]string{}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(nil, 1<<20)
	for scanner.Scan() {
		var r struct {
			Direction  string          `json:"direction"`
			ID         json.RawMessage `json:"id"`
			Method     string          `json:"method"`
			Params     json.RawMessage `json:"params"`
			Thread     json.RawMessage `json:"thread"`
			DataIDs    []string        `json:"dataIds"`
			NextCursor json.RawMessage `json:"nextCursor"`
			Error      json.RawMessage `json:"error"`
			Line       int             `json:"line"`
		}
		if err := json.Unmarshal(scanner.Bytes(), &r); err != nil {
			t.Fatal(err)
		}
		message := map[string]any{}
		if len(r.ID) > 0 && string(r.ID) != "null" {
			message["id"] = r.ID
		}
		switch {
		case r.Direction == "tui-to-gateway" || r.Method != "":
			message["method"] = r.Method
			if len(r.Params) > 0 {
				message["params"] = r.Params
			}
			if r.Direction == "tui-to-gateway" {
				methods[string(r.ID)] = r.Method
			}
		case len(r.Error) > 0:
			message["error"] = r.Error
		case len(r.Thread) > 0:
			message["result"] = map[string]any{"thread": r.Thread}
		case r.DataIDs == nil && unlisted[name][r.Line] != nil:
			message["result"] = map[string]any{"data": unlisted[name][r.Line], "nextCursor": nil}
		case r.DataIDs != nil:
			var data any = r.DataIDs
			if methods[string(r.ID)] != "thread/loaded/list" {
				objects := []map[string]string{}
				for _, id := range r.DataIDs {
					objects = append(objects, map[string]string{"id": id})
				}
				data = objects
			}
			message["result"] = map[string]any{"data": data, "nextCursor": r.NextCursor}
		default:
			message["result"] = map[string]any{}
		}
		raw, err := json.Marshal(message)
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, tuiRecord{line: r.Line, fromTUI: r.Direction == "tui-to-gateway", raw: raw})
	}
	if err := scanner.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}
