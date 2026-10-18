package workflow

import "time"

// waitFor polls until the condition holds or the window runs out.
func waitFor(c *Case, window time.Duration, condition func() bool) bool {
	deadline := time.Now().Add(window)
	for time.Now().Before(deadline) && !c.Expired() {
		if condition() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return false
}
