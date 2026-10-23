package adapter

// WorkerReadiness is a harness whose workers need something only the person
// can give — a login of their own — which the person gives in the worker's
// own pane. space open asks it and opens the worker either way: its answer
// tells main what the person is to do there.
type WorkerReadiness interface {
	// WorkerNotice says what a worker still lacks and what the person does
	// in its pane, or "" when nothing is missing or the harness cannot tell.
	// command is the wrapper the template names, which then decides the
	// configuration the harness starts with.
	WorkerNotice(command string) string
}
