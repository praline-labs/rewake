package workflow

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"
)

// TestWrappedLaunch launches each column twice, in two rooms: once as usual,
// once through a person's own wrapper named with --command. The wrapper stands
// in for one that sets up an environment and then execs the harness: it
// exports a marker, writes down that it ran, and execs the fixture. Every
// process of the fixture records how it was started.
//
// rewake must start the wrapper wherever it would have started the harness —
// for the fixture that is the version read and the session half as well as the
// terminal — with exactly the arguments the plain launch gets, and each of
// those processes must run in the wrapper's environment.
//
// The wrapper is named by a relative path, and another directory holds a
// script of the same name: a relative path resolved anywhere but the launch
// directory would start that one.
func TestWrappedLaunch(t *testing.T) {
	runInColumns(t, "wrapped-launch", runWrappedLaunch)
}

const (
	obsWrapperStarted = "the session starts through the named wrapper, and only that one"
	obsWrapperArgs    = "each wrapped process gets exactly what the plain launch gets"
	obsWrapperEnv     = "every harness process runs in the wrapper's environment"
	obsWrappedReady   = "the wrapped session becomes ready"
)

// Rooms with names nothing else in a launch line contains, so they can be
// normalized away when the two launches are compared.
const (
	plainRoom   = "roomplainq7"
	wrappedRoom = "roomwrappedq7"
)

func runWrappedLaunch(t *testing.T, col column) {
	binary := enterScenario(t, "wrapped-launch")
	c := Start(t, Spec{
		Name:         "wrapped-launch",
		Harness:      col.harness,
		Observations: []string{obsWrapperStarted, obsWrapperArgs, obsWrapperEnv, obsWrappedReady},
		Deadline:     60 * time.Second,
	})
	iso := Isolate(t, c, binary)

	name := col.harness + "-worker"
	log := filepath.Join(iso.Home, "wrapper.log")
	wrong := filepath.Join(iso.Home, "wrong-wrapper.log")
	// In the launch directory, which is where the case starts rewake.
	writeScript(t, filepath.Join(iso.Home, name),
		"export "+shimWrappedMarker+"=1\nprintf x >> '"+log+"'\nexec "+col.harness+" \"$@\"\n")
	// The same name in another directory, which must never run.
	other := filepath.Join(iso.Home, "other-repository")
	if err := os.MkdirAll(other, 0o700); err != nil {
		t.Fatal(err)
	}
	writeScript(t, filepath.Join(other, name), "printf x >> '"+wrong+"'\nexit 1\n")
	var harnessArgs []string

	plainCalls, wrappedCalls := filepath.Join(iso.Home, "plain.calls"), filepath.Join(iso.Home, "wrapped.calls")
	plain := startSessionWith(t, c, iso, col.harness, "same", "--main", []string{"--room", plainRoom}, harnessArgs,
		shimCallsFile+"="+plainCalls)
	defer stopSession(t, c, plain)
	wrapped := startSessionWith(t, c, iso, col.harness, "same", "--main", []string{"--room", wrappedRoom, "--command", "./" + name}, harnessArgs,
		shimCallsFile+"="+wrappedCalls)
	defer stopSession(t, c, wrapped)

	for _, session := range []*scenarioSession{plain, wrapped} {
		if !sessionReady(c, session) {
			c.Contradicted(obsWrappedReady, "a session never became ready")
			return
		}
	}
	c.Observed(obsWrappedReady, "both launches ready")

	// The fixture's three processes are started one after another — its
	// version, its session half and its terminal; each records itself on
	// start, so the counts settle once the sessions are ready.
	want := 1
	if col.harness == fixtureColumn.harness {
		want = 3
	}
	waitFor(c, 10*time.Second, func() bool {
		return len(shimCalls(plainCalls)) >= want && len(shimCalls(wrappedCalls)) >= want
	})
	plainRecorded, wrappedRecorded := shimCalls(plainCalls), shimCalls(wrappedCalls)

	ran, _ := os.ReadFile(log)
	strayed, _ := os.ReadFile(wrong)
	switch {
	case len(strayed) > 0:
		c.Contradicted(obsWrapperStarted, "the same-named script in the -C directory ran %d times", len(strayed))
	case len(ran) != want:
		c.Contradicted(obsWrapperStarted, "the wrapper ran %d times, want %d", len(ran), want)
	default:
		c.Observed(obsWrapperStarted, fmt.Sprintf("%d calls, all to the wrapper in the launch directory", len(ran)))
	}

	marked := len(wrappedRecorded) == want
	for _, call := range wrappedRecorded {
		marked = marked && call.wrapped
	}
	for _, call := range plainRecorded {
		marked = marked && !call.wrapped
	}
	if marked {
		c.Observed(obsWrapperEnv, fmt.Sprintf("all %d wrapped processes saw the marker, no plain one did", want))
	} else {
		c.Contradicted(obsWrapperEnv, "want %d wrapped processes with the marker and none in the plain launch; got %d wrapped (%v), %d plain",
			want, len(wrappedRecorded), markers(wrappedRecorded), len(plainRecorded))
	}

	a, b := normalizedCalls(plainRecorded, plainRoom), normalizedCalls(wrappedRecorded, wrappedRoom)
	if len(a) == want && slices.Equal(a, b) {
		c.Observed(obsWrapperArgs, fmt.Sprintf("%d calls, argument for argument", want))
	} else {
		c.Contradicted(obsWrapperArgs, "the launches differ:\nplain   %q\nwrapped %q", a, b)
	}
}

func writeScript(t *testing.T, path, body string) {
	t.Helper()
	if err := os.WriteFile(path, []byte("#!/bin/sh\n"+body), 0o700); err != nil {
		t.Fatal(err)
	}
}

func sessionReady(c *Case, session *scenarioSession) bool {
	return waitFor(c, 20*time.Second, func() bool { _, err := os.Stat(session.ready); return err == nil })
}

func markers(calls []shimCall) []bool {
	var out []bool
	for _, call := range calls {
		out = append(out, call.wrapped)
	}
	return out
}

// epochPattern is a run's epoch where it appears in a path: after the session
// name, before the socket's suffix, with the boot id a run of this build
// carries.
var epochPattern = regexp.MustCompile(`(same-(?:claude|fixture))\.[0-9]+\.[0-9]+(?:\.[0-9a-f-]{36})?`)

// digestPattern is a socket named by the digest of its name and epoch, the
// form a path past 103 bytes takes (internal/state/paths.go): the digest
// differs between two runs as the epoch does.
var digestPattern = regexp.MustCompile(`/sock/(rewake-)?[0-9a-f]{24}\.`)

// normalizedCalls are the calls with what differs between two rooms by
// construction taken out — the room's name and the run's epoch, or the
// digest standing for it — each call one string, in a stable order.
func normalizedCalls(calls []shimCall, room string) []string {
	var out []string
	for _, call := range calls {
		joined := strings.Join(call.args, "\x1f")
		joined = strings.ReplaceAll(joined, room, "<room>")
		joined = epochPattern.ReplaceAllString(joined, "$1.<epoch>")
		joined = digestPattern.ReplaceAllString(joined, "/sock/${1}<digest>.")
		out = append(out, joined)
	}
	slices.Sort(out)
	return out
}
