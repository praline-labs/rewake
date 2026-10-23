package adapter

// WorkerReadiness is a harness whose workers need something only the person
// can give before one starts unattended — a login of their own. space open
// asks it before anything is written: a worker started without it would stop
// in its pane, and the open would show only its timeout.
type WorkerReadiness interface {
	// WorkerRefusal says why a worker cannot start now, with what the person
	// runs to fix it, or nil when it can or the harness cannot tell. command
	// is the wrapper the template names, which then decides the configuration
	// the harness starts with.
	WorkerRefusal(command string) error
}
