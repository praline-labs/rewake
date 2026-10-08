package harness

// ToolInput is what a harness reports of the mail tool's calls, in no
// harness's terms: the five inputs the wrapper's endpoint takes
// (docs/v2/stage3-steps-adapters.md, S7).
// An adapter reads its harness's own records — a stream of events, a hook's
// input, a frame of its program — and calls these; none of them waits, so the
// adapter's reader goes on at once.
type ToolInput interface {
	// TurnStarted and TurnEnded follow the turns of the run's conversation:
	// a call is bound to a turn the harness was seen to start and not yet
	// seen to end.
	TurnStarted(conversation, turn string)
	TurnEnded(conversation, turn string)
	// CallSeen is the harness's own record that the model made a call: what
	// a request for the call's binding is matched against.
	CallSeen(call ObservedCall)
	// CallResult is the result the harness handed the model for a call,
	// which acknowledges a read when it proves the whole answer arrived.
	CallResult(callID string, result ToolResult)
	// StartupFailed says the harness could not start the tool for a
	// conversation, "" when it named none.
	StartupFailed(conversation string)
}

// ObservedCall is one call as the harness recorded it.
type ObservedCall struct {
	// ID is the harness's own id of the call; Conversation and Turn its ids
	// of the conversation and turn the call was made in.
	ID, Conversation, Turn string
	// Words are the command's words the call carried, nil when its
	// arguments could not be read as words.
	Words []string
	// Nested says a nested agent made the call, not the conversation's
	// model.
	Nested bool
}

// ToolResult is a call's result as the harness handed it to the model.
type ToolResult struct {
	// Succeeded says the harness took the result as a success; Direct that
	// the model called the tool itself, not from inside a script.
	Succeeded, Direct bool
	// Texts are the result's text items in order. Shortened says the harness
	// cut, persisted, previewed or replaced the result with something that
	// is not text, so that no text of it is the whole answer.
	Texts     []string
	Shortened bool
}
