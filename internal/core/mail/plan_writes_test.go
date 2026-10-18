package mail

// brokenRun is one barrier with a failed write or a read failure in its effects.
type brokenRun struct {
	barrier string
	broke   bool
}

func writeRun(dir, mailbox string, write *seamWrite, fault *seamFault) brokenRun {
	plan, effects := newProbe(dir, osAccess{}, nil), newProbe(dir, osAccess{}, fault)
	effects.broken = write
	testAccess.Store(dir, passAccess{live: effects, plan: plan})
	defer testAccess.Delete(dir)
	answer := outcome(barrier(dir, mailbox, true))
	return brokenRun{barrier: answer, broke: effects.broke}
}
