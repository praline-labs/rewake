package adapter

// WorkerReadiness is a harness whose workers need something only the person
// can give — a login of their own — which the person gives in the worker's
// own pane. space open asks it and opens the worker either way: its answer
// tells main what the person is to do there.
type WorkerReadiness interface {
	// WorkerNotice says what a worker still lacks and what the person does
	// in its pane, or "" when nothing is missing or the harness cannot tell.
	// command is the wrapper the template names. A harness may skip the
	// probe for one: the wrapper's role-sensitive configuration is unknown
	// before the worker's launch, so a guessed role could misstate the login.
	WorkerNotice(command string) string
}
