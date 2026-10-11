package workflow

import "testing"

// The controls of the live case on the gate column. Each breaks one decision
// by the live set, or one transition of the channel keeper, and the
// observations resting on it go with it.

// A letter to a run without a live Wake waits for a delivery nothing makes.
var mutantSendUnwoken = mutation{
	name:  "send-unwoken",
	file:  "internal/cli/send.go",
	edits: []edit{{"if !session.HasLive(string(adapter.WakeCapability)) {", "if false && !session.HasLive(string(adapter.WakeCapability)) {"}},
}

// A reservation goes to the Wake whether it is live or not, and the
// adapter's refusal fails the letter the sender was told it could read.
var mutantReserveUnguarded = mutation{
	name:  "reserve-unguarded",
	file:  "internal/wrap/wrap.go",
	edits: []edit{{"server.Reserve = reserveIfLive(reserver, live)", "server.Reserve = reserver.Reserve"}},
}

// A letter owed a report asks the conversation of a run whose Wake is gone.
var mutantThreadUnguarded = mutation{
	name:  "thread-unguarded",
	file:  "internal/wrap/wrap.go",
	edits: []edit{{"Thread: threadIfLive(thread, live),", "Thread: thread,"}},
}

// A task goes to a run whatever its boundary's live mark says.
var mutantTaskUnbounded = mutation{
	name:  "task-unbounded",
	file:  "internal/cli/turn_reporting.go",
	edits: []edit{{"if session.HasLive(string(adapter.TurnBoundaryCapability)) {", "if true || session.HasLive(string(adapter.TurnBoundaryCapability)) {"}},
}

// A worker's launch starts without waiting for its boundary.
var mutantLaunchUnawaited = mutation{
	name:  "launch-unawaited",
	file:  "internal/wrap/live.go",
	edits: []edit{{"|| live.await(ctx, adapter.TurnBoundaryCapability, adapter.ReadinessBound) {", "|| true {"}},
}

// A lost connection tells the endpoint and not the host: the record keeps
// the set it no longer has.
var mutantLostKept = mutation{
	name:  "lost-kept",
	file:  "internal/adapter/fixture/lifecycle.go",
	edits: []edit{{"\tb.changed()", "\tb.toolsLive()"}},
}

// A tool the exchange did not make live leaves the record waiting for it.
var mutantNoToolUnrecorded = mutation{
	name:  "no-tool-unrecorded",
	file:  "internal/wrap/tools.go",
	edits: []edit{{"\tt.keeper.begin(false, reason)\n", "\n"}},
}

// The harness's denial reaches nothing.
var mutantDenialDropped = mutation{
	name:  "denial-dropped",
	file:  "internal/bridge/endpoint/input.go",
	edits: []edit{{"{ e.tell(channel.Event{Kind: channel.Denied}) }", "{ _ = channel.Denied }"}},
}

// The shell's observations are taken and never folded.
var mutantShellUnread = mutation{
	name:  "shell-unread",
	file:  "internal/wrap/channel.go",
	edits: []edit{{"notes := receipt.TakeShell(k.dir, k.name, k.epoch)", "notes := receipt.TakeShell(k.dir, k.name, k.epoch)[:0]"}},
}

// The run's end does not freeze the record.
var mutantExitUnfrozen = mutation{
	name:  "exit-unfrozen",
	file:  "internal/wrap/channel.go",
	edits: []edit{{"\tk.tell(channel.Event{Kind: channel.Exited})", "\t_ = channel.Exited"}},
}

// The briefing is built as if no tool were live.
var mutantBriefToolless = mutation{
	name:  "brief-toolless",
	file:  "internal/adapter/fixture/fixture.go",
	edits: []edit{{"context.Tool = tool", "context.Tool = false"}},
}

func TestAnUnwokenLetterWaitingForADeliveryFails(t *testing.T) {
	runLiveControl(t, mutantSendUnwoken, obsLiveUnwoken)
}

func TestAReservationWithoutALiveWakeFails(t *testing.T) {
	runLiveControl(t, mutantReserveUnguarded, obsLiveUnwoken)
}

func TestAConversationAskedWithoutALiveWakeFails(t *testing.T) {
	runLiveControl(t, mutantThreadUnguarded, obsLiveUnwoken)
}

func TestATaskWithoutALiveBoundaryAcceptedFails(t *testing.T) {
	runLiveControl(t, mutantTaskUnbounded, obsLiveDropped)
}

func TestALaunchThatDoesNotWaitForItsBoundaryFails(t *testing.T) {
	runLiveControl(t, mutantLaunchUnawaited, obsLiveLaunch)
}

func TestALostConnectionKeptLiveFails(t *testing.T) {
	runLiveControl(t, mutantLostKept, obsLiveDropped)
}

func TestANoToolLeftUnrecordedFails(t *testing.T) {
	runLiveControl(t, mutantNoToolUnrecorded, obsChannelNone)
}

func TestADenialDroppedFails(t *testing.T) {
	runLiveControl(t, mutantDenialDropped, obsChannelDenied)
}

func TestTheShellLeftUnreadFails(t *testing.T) {
	runLiveControl(t, mutantShellUnread, obsChannelShell)
}

func TestARecordLeftUnfrozenFails(t *testing.T) {
	runLiveControl(t, mutantExitUnfrozen, obsChannelFrozen)
}

func TestABriefingWithoutTheLiveToolFails(t *testing.T) {
	runLiveControl(t, mutantBriefToolless, obsChannelLive)
}

func runLiveControl(t *testing.T, mutant mutation, breaks ...string) {
	t.Helper()
	runFindingsControlOn(t, fixtureColumn.harness, "fixture-live", playLive, mutant, breaks...)
}
