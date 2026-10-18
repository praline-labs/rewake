package workflow

import "fmt"

// recorder stands in for *testing.T so a deliberately incomplete case can be
// finished and its verdict read without failing this test.
type recorder struct {
	cleanups []func()
	errors   []string
	fatals   []string
}

func (r *recorder) Helper()      {}
func (r *recorder) Failed() bool { return len(r.errors) > 0 }
func (r *recorder) Errorf(format string, args ...any) {
	r.errors = append(r.errors, fmt.Sprintf(format, args...))
}

func (r *recorder) Fatal(args ...any) { r.fatals = append(r.fatals, fmt.Sprint(args...)) }

func (r *recorder) Fatalf(format string, args ...any) {
	r.fatals = append(r.fatals, fmt.Sprintf(format, args...))
}
func (r *recorder) Cleanup(fn func()) { r.cleanups = append(r.cleanups, fn) }

// finish runs the registered cleanups last-in-first-out, the way testing does,
// so the ordering the case relies on is the ordering under test.
func (r *recorder) finish() {
	for i := len(r.cleanups) - 1; i >= 0; i-- {
		r.cleanups[i]()
	}
}
