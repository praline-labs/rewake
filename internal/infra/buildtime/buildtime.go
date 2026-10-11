// Package buildtime reads durations a build may set with -ldflags -X.
//
// A handful of the product's waits are set shorter for the workflow suite's
// binary: the suite waits on each of them in dozens of cases, and at the real
// length every such wait costs seconds the logic under test does not need.
// The build sets them rather than a test branch in the code, so the binary
// under test runs the same code as a release and differs only in the numbers.
// A release build sets none of them.
package buildtime

import (
	"fmt"
	"time"
)

// Duration is the duration a build set in value, or standard when it set
// none. A value that does not parse stops the binary at its start: a build
// asked for a length, and quietly serving another would make whatever it tests
// untrue. name is the variable's name, for that message.
func Duration(name, value string, standard time.Duration) time.Duration {
	if value == "" {
		return standard
	}
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		panic(fmt.Sprintf("rewake was built with %s=%q, not a positive duration", name, value))
	}
	return parsed
}
