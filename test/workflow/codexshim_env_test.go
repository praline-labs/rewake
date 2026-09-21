package workflow

// How a case tells the shim what to be: the marks that say it is running
// inside a case at all, the files it writes its evidence to, and the controls
// that make it answer like a server that is wrong in one specific way.
//
// Everything passes through the environment because the shim is a separate
// process, re-executed as `codex` by the wrapper — a case cannot hand it a
// value any other way.

const (
	shimEnv = "RW_SHIM"
	// shimThread is the conversation the shim starts. A numeric request id and
	// threadSource=user are what make the request recognizable to the
	// gateway; the thread id itself only has to be stable.
	shimThread = "0199ab12-0000-7000-8000-abcdef000001"

	// Controls. Each one makes the shim answer like a server that is wrong in
	// one specific way, so a scenario can check that readiness does not follow.
	shimNoDirectInput = "RW_SHIM_NO_DIRECT_INPUT"
	shimWrongThread   = "RW_SHIM_WRONG_THREAD"
	// shimNoCorrelatedReply announces a conversation as an event and never
	// answers the request that asked for it.
	shimNoCorrelatedReply = "RW_SHIM_NO_CORRELATED_REPLY"
	// mailboxToolName is the operation a delivery arrives as. Spelled out here
	// rather than imported from internal: a fixture sharing the product's
	// constant would agree with it by construction.
	mailboxToolName = "rewake_mailbox_notice"
	// shimWrongReportID makes the session record a different message id than
	// the one delivered, so that the report's link points elsewhere;
	// shimReadFails makes the mailbox read fail.
	shimWrongReportID = "RW_SHIM_WRONG_REPORT_ID"
	shimReadFails     = "RW_SHIM_READ_FAILS"
	// shimLateFailure fails the turn after its text was sent and before the
	// wrapper has published a report for it.
	shimLateFailure = "RW_SHIM_LATE_FAILURE"
	// shimSecondTerminal sends a second terminal event for a turn that has
	// already ended. A server does not do this; the wrapper is supposed to
	// ignore it, and this is how that is observed rather than assumed.
	shimSecondTerminal = "RW_SHIM_SECOND_TERMINAL"
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
	// shimResume makes the client continue a named conversation instead of
	// starting a new one. Only a continuation carries a conversation id the
	// client chose, so that is where "the server answered about a different
	// conversation" can be asked at all.
	shimResume = "RW_SHIM_RESUME"

	// sessionNameEnv and sessionEpochEnv are how the wrapper tells a harness
	// which session it is running. Spelled out here rather than imported, like
	// every other name this fixture checks a delivery against.
	// stateDirEnv is the state directory a case gives its sessions. A shim that
	// cannot see one is not inside a case.
	stateDirEnv     = "REWAKE_DIR"
	sessionNameEnv  = "REWAKE_SESSION"
	sessionEpochEnv = "REWAKE_EPOCH"

	// shimStateFile is where the client half writes what `rewake list` says.
	// The session runs the command, not the test: that is the only way the
	// provenance of an observation can be the session's.
	shimStateFile = "RW_SHIM_STATE_FILE"
	// shimAcceptedFile is where the client writes the conversation id the
	// server actually gave it.
	shimAcceptedFile = "RW_SHIM_ACCEPTED_FILE"
)
