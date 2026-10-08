package workflow

// How a case tells the shim what to be: the marks that say it is running
// inside a case at all, the files it writes its evidence to, and the controls
// that make it answer like a harness that is wrong in one specific way.
//
// Everything passes through the environment because the shim is a separate
// process, re-executed as the harness by the wrapper — a case cannot hand it a
// value any other way.

const (
	shimEnv = "RW_SHIM"
	// shimHarness says which harness this process is re-executed as. The two
	// fixtures are different programs behind one binary, and the script that
	// stands in for each sets this.
	shimHarness = "RW_SHIM_HARNESS"

	// shimWrongReportID makes the session record a different message id than
	// the one delivered, so that the report's link points elsewhere;
	// shimReadFails makes the mailbox read fail.
	shimWrongReportID = "RW_SHIM_WRONG_REPORT_ID"
	shimReadFails     = "RW_SHIM_READ_FAILS"
	// shimLateFailure fails the turn after its text was sent and before the
	// wrapper has published a report for it.
	shimLateFailure = "RW_SHIM_LATE_FAILURE"
	// shimExitAfterTurn makes the session leave, quietly and successfully, as
	// soon as it has worked a turn. A session that disappears before the case
	// is judged takes its evidence with it.
	shimExitAfterTurn = "RW_SHIM_EXIT_AFTER_TURN"
	// shimSendTo and shimSendText make this session send a task once it is
	// ready, so that the sender in a delivery scenario is a session rather
	// than the test process.
	shimSendTo   = "RW_SHIM_SEND_TO"
	shimSendText = "RW_SHIM_SEND_TEXT"
	// shimMailboxFile is where a session records what its own inbox read
	// returned, so a scenario can see what the session saw.
	shimMailboxFile = "RW_SHIM_MAILBOX_FILE"
	// shimExitFile is the file whose appearance asks this session to stop.
	shimExitFile = "RW_SHIM_EXIT_FILE"
	// shimTurnsFile is where a session records the turns it accepted.
	shimTurnsFile = "RW_SHIM_TURNS_FILE"
	// shimDeliveredFile is where a session records the id of every message a
	// delivery named. A report says which messages it settles, and the only
	// way to check that link is to know what actually arrived here.
	shimDeliveredFile = "RW_SHIM_DELIVERED_FILE"
	// shimInboxJSON makes a session read its mailbox in the machine form. The
	// link between a report and the messages it answers is a field, not a line
	// of the printed form, so the side that has to observe it reads JSON.
	shimInboxJSON = "RW_SHIM_INBOX_JSON"
	// shimReadEach makes a session read a delivery the way the grouped-inbox
	// contract describes: an overview first, then one member at a time. The
	// last member of a group is left unread until the next delivery, so that
	// the case holds mail that was announced and stays unread — the state in
	// which a replay would show.
	shimReadEach = "RW_SHIM_READ_EACH"
	// shimGroupsFile is where a session records every delivery as a group:
	// the turn, the member count, the member ids and the notice text.
	shimGroupsFile = "RW_SHIM_GROUPS_FILE"
	// shimReadsFile is where a session records each inbox call it made under
	// shimReadEach, and what came of it.
	shimReadsFile = "RW_SHIM_READS_FILE"
	// shimSendTexts is a list of texts sent at the same moment, separated by
	// "|"; shimSendLaterText is one more, sent laterSendDelay after them.
	shimSendTexts     = "RW_SHIM_SEND_TEXTS"
	shimSendLaterText = "RW_SHIM_SEND_LATER_TEXT"
	// shimSendNotes is a list of heads-ups, separated by "|", sent with
	// --notify noteSpacing apart once the later letter has had its time.
	shimSendNotes = "RW_SHIM_SEND_NOTES"
	// shimHoldTurn makes the session hold its first turn open: it reads, then
	// waits for a second delivery to be steered into that same turn before it
	// finishes. That is the pending operation a mid-turn scenario needs, and
	// the session's own record of it is what the scenario reads.
	shimHoldTurn = "RW_SHIM_HOLD_TURN"
	// shimFailHeldTurn ends the held turn with a failure instead of an answer.
	shimFailHeldTurn = "RW_SHIM_FAIL_HELD_TURN"
	// shimEndReason is what the fixture program's turn says when it ends
	// failed or interrupted, in place of what it read: the reason a scenario
	// looks for in the report, distinct from any text the mail carried.
	shimEndReason = "RW_SHIM_END_REASON"
	// shimSendWhenWorking and shimSendWhenIdle name the session whose telemetry
	// the sender watches before its second letter leaves: working for a
	// delivery that must land inside a turn, idle again for the control that
	// must land after it.
	shimSendWhenWorking = "RW_SHIM_SEND_WHEN_WORKING"
	shimSendWhenIdle    = "RW_SHIM_SEND_WHEN_IDLE"
	// shimSendSecondText is the letter that goes out once that state is seen.
	shimSendSecondText = "RW_SHIM_SEND_SECOND_TEXT"
	// shimSendsFile is where a sender records what became of each letter it
	// sent: accepted, or refused with the reason its own rewake gave. Without
	// it a scenario cannot tell a letter that has not arrived yet from one
	// whose delivery was refused.
	shimSendsFile = "RW_SHIM_SENDS_FILE"
	// shimReadyFile is where a session says it is listening. A sender waits
	// for this file rather than for a readiness field — and mail sent before
	// the session listens waits in the mailbox and lands in the first
	// delivery, which would make a scenario about the collection window
	// measure the launch instead.
	shimReadyFile = "RW_SHIM_READY_FILE"
	// shimWaitForFile is that file, as the sender is told about it.
	shimWaitForFile = "RW_SHIM_WAIT_FOR_FILE"

	// sessionNameEnv and sessionEpochEnv are how the wrapper tells a harness
	// which session it is running. Spelled out here rather than imported, like
	// every other name this fixture checks a delivery against.
	// stateDirEnv is the state directory a case gives its sessions. A shim that
	// cannot see one is not inside a case.
	stateDirEnv     = "REWAKE_DIR"
	sessionNameEnv  = "REWAKE_SESSION"
	sessionEpochEnv = "REWAKE_EPOCH"

	// shimStateFile is where the session writes what `rewake list` says.
	// The session runs the command, not the test: that is the only way the
	// provenance of an observation can be the session's.
	shimStateFile = "RW_SHIM_STATE_FILE"
)
