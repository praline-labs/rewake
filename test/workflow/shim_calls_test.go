package workflow

import (
	"os"
	"strings"
)

const (
	// shimCallsFile makes every process of the fixture write down how it was
	// started: the arguments it was given and whether shimWrappedMarker was
	// in its environment.
	shimCallsFile = "RW_SHIM_CALLS_FILE"
	// shimWrappedMarker is what a stand-in wrapper exports before it execs
	// the harness; the fixture seeing it proves it runs in the wrapper's
	// environment.
	shimWrappedMarker = "RW_SHIM_WRAPPED"
)

// shimCall is one start of the fixture.
type shimCall struct {
	wrapped bool
	args    []string
}

// recordShimCall appends this start to the file the case named. One record
// per call and one field per argument, with separators no argument carries:
// the briefing holds newlines, so a line per call would split it.
func recordShimCall(argv []string) {
	target := os.Getenv(shimCallsFile)
	if target == "" {
		return
	}
	fields := append([]string{os.Getenv(shimWrappedMarker)}, harnessArgs(argv)...)
	file, err := os.OpenFile(target, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		return
	}
	defer func() { _ = file.Close() }()
	_, _ = file.WriteString(strings.Join(fields, "\x1f") + "\x1e")
}

// shimCalls reads the records back.
func shimCalls(path string) []shimCall {
	raw, err := os.ReadFile(path)
	if err != nil || len(raw) == 0 {
		return nil
	}
	var calls []shimCall
	for _, record := range strings.Split(strings.TrimSuffix(string(raw), "\x1e"), "\x1e") {
		fields := strings.Split(record, "\x1f")
		calls = append(calls, shimCall{wrapped: fields[0] == "1", args: fields[1:]})
	}
	return calls
}
