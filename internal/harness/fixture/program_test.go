//go:build rewakefixture

package fixture

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"
)

// The package's tests run the adapter against this test binary, started again
// as a scripted program: the exchange needs a real process, because the peer
// check reads the connecting process's pid and start time. The workflow
// suite's program is the full one; this one speaks only what the tests here
// ask of it, each switch an environment variable.
const (
	programEnv   = "FIXTURE_TEST_PROGRAM"
	switchNo     = "FIXTURE_TEST_NO"         // capabilities left out of the hello, comma-separated
	switchFail   = "FIXTURE_TEST_FAIL_PROBE" // a capability served whose probe fails
	switchDrop   = "FIXTURE_TEST_DROP"       // close the socket once the probes are answered
	switchAgain  = "FIXTURE_TEST_RECONNECT"  // after the drop, connect again from the same process
	switchHelper = "FIXTURE_TEST_HELPER"     // say hello from a child, in the program's name
	switchSilent = "FIXTURE_TEST_SILENT"     // connect and never say hello
	switchOrphan = "FIXTURE_TEST_ORPHAN"     // once probed, leave the socket to a child and exit
	childEnv     = "FIXTURE_TEST_CHILD_OF"
	programThrd  = "fixture-test-thread"
)

func TestMain(m *testing.M) {
	if os.Getenv(programEnv) != "" {
		os.Exit(runProgram(os.Args[1:]))
	}
	os.Exit(m.Run())
}

// runProgram is the scripted program: connect to the adapter's socket, say
// hello, answer the probes, and stay until the adapter ends it.
func runProgram(args []string) int {
	socket := ""
	for i, arg := range args {
		if arg == "--connect" && i+1 < len(args) {
			socket = args[i+1]
		}
	}
	if socket == "" {
		fmt.Fprintln(os.Stderr, "no --connect")
		return 2
	}
	if os.Getenv(switchHelper) != "" && os.Getenv(childEnv) == "" {
		child := exec.Command(os.Args[0], args...)
		child.Env = append(os.Environ(), childEnv+"="+strconv.Itoa(os.Getpid()))
		child.Stderr = os.Stderr
		if err := child.Start(); err != nil {
			return 1
		}
		time.Sleep(time.Minute)
		return 0
	}
	pid := os.Getpid()
	if parent, err := strconv.Atoi(os.Getenv(childEnv)); err == nil {
		pid = parent
	}
	connections := 1
	if os.Getenv(switchAgain) != "" {
		connections = 2
	}
	for n := range connections {
		if n > 0 {
			time.Sleep(100 * time.Millisecond)
		}
		if err := speak(socket, pid, os.Getenv(switchDrop) != "" && n == 0); err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 1
		}
	}
	time.Sleep(time.Minute)
	return 0
}

// speak runs one connection; drop closes it once every probe is answered.
func speak(socket string, pid int, drop bool) error {
	conn, err := net.Dial("unix", socket)
	if err != nil {
		return err
	}
	if os.Getenv(switchSilent) != "" {
		time.Sleep(time.Minute)
		return nil
	}
	write := func(frame Frame) error {
		raw, _ := json.Marshal(frame)
		_, err := conn.Write(append(raw, '\n'))
		return err
	}
	withheld := strings.Split(os.Getenv(switchNo), ",")
	serves := slices.DeleteFunc(slices.Clone(Served), func(c string) bool { return slices.Contains(withheld, c) })
	if err := write(Frame{Op: opHello, Version: "1.0.0", PID: pid, Thread: programThrd, Serves: serves}); err != nil {
		return err
	}
	reader := bufio.NewReader(conn)
	answered := 0
	for {
		line, err := readLine(reader)
		if err != nil {
			if drop {
				return nil
			}
			return err
		}
		var frame Frame
		if json.Unmarshal(line, &frame) != nil {
			continue
		}
		reply := Frame{Op: opAnswer, ID: frame.ID, OK: true}
		switch frame.Op {
		case opProbe:
			reply.State = map[string]string{TurnBoundary: "idle", Telemetry: "none", Control: "ready"}[frame.Capability]
			if frame.Capability == os.Getenv(switchFail) {
				reply.OK, reply.Error = false, "withheld"
			}
			answered++
		case opReserve:
			reply.Thread = programThrd
		case opRelease:
			continue
		}
		if err := write(reply); err != nil {
			return err
		}
		if os.Getenv(switchOrphan) != "" && answered == len(serves) {
			// The child holds the connection open after the program is gone.
			file, err := conn.(*net.UnixConn).File()
			if err != nil {
				return err
			}
			child := exec.Command("sleep", "30")
			child.ExtraFiles = []*os.File{file}
			if err := child.Start(); err != nil {
				return err
			}
			time.Sleep(100 * time.Millisecond)
			os.Exit(0)
		}
		if drop && answered == len(serves) {
			// Let the last answer reach the adapter before the socket goes.
			time.Sleep(200 * time.Millisecond)
			return conn.Close()
		}
	}
}
