package inbox

import "time"

// CompactionNotice is an observed completion fact; current activity is fetched
// separately when main reads the note, without imposing idle or working.
type CompactionNotice struct {
	Count      uint64    `json:"completedCount"`
	ObservedAt time.Time `json:"observedAt"`
}

// DepartureNotice retains the old run's identity and a bounded factual reason.
// It does not classify an unknown exit as a crash.
type DepartureNotice struct {
	Identity Availability `json:"identity"`
	Reason   string       `json:"reason"`
}
