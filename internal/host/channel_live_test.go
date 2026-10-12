package host

import (
	"testing"

	"github.com/praline-labs/rewake/internal/adapter"
	"github.com/praline-labs/rewake/internal/core/channel"
	"github.com/praline-labs/rewake/internal/host/endpoint"
)

// A tool the first exchange did not make live keeps its offered record:
// unproven, not absent. A later exchange that makes it live brings its hello,
// its calls and a denial of it into the record — the denial blocks the
// shell's advice (C7), and only a ticket issued after it lifts the block.
func TestAToolLiveAfterAnUnprovenExchangeKeepsItsLifecycle(t *testing.T) {
	dir, session := liveRun(t)
	tool := &mailTool{offered: true, keeper: newChannelKeeper(dir, session.Name, session.Epoch())}
	tool.keeper.begin(true, "")
	live := newLiveSet(dir, session.Name, session.Epoch())
	live.exchanged = tool.exchanged

	without := []adapter.Capability{adapter.WakeCapability, adapter.TurnBoundaryCapability}
	live.report(without)
	if record := tool.keeper.snapshot(); record == nil || record.Tool != channel.ToolStarting || record.Reason != "" {
		t.Fatalf("an unproven tool after the first exchange: %+v", record)
	}

	live.report(append(without, adapter.ToolTransportCapability))
	tool.keeper.tell(channel.Event{Kind: channel.Hello, Generation: 1, At: endpoint.Stamp()})
	if record := tool.keeper.snapshot(); record.Tool != channel.ToolConnected {
		t.Fatalf("the tool made live later said hello: %+v", record)
	}

	denied := endpoint.Stamp()
	tool.keeper.tell(channel.Event{Kind: channel.Denied, At: denied})
	tool.keeper.mu.Lock()
	publication, planned := tool.keeper.notices.Plan(&tool.keeper.record, channel.Recipient{Role: channel.ToWorker, Name: session.Name, Epoch: session.Epoch()}, endpoint.Stamp())
	tool.keeper.mu.Unlock()
	if record := tool.keeper.snapshot(); !record.Blocked() || record.Category() != channel.CategoryDenied || planned && publication.Advice {
		t.Fatalf("the denial of a tool made live later: %+v, advice planned %t", record, planned && publication.Advice)
	}

	issued := endpoint.Stamp()
	tool.keeper.tell(channel.Event{Kind: channel.Validated, Issued: issued.Boot, At: issued})
	if record := tool.keeper.snapshot(); record.Blocked() || record.Tool != channel.ToolWorking || record.Category() != channel.CategoryTool {
		t.Fatalf("a ticket issued after the denial: %+v", record)
	}
}
