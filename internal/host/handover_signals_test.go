package host

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"testing"
	"time"
)

// signalChild marks the test binary run as the parked image.
const signalChild = "REWAKE_TEST_PARKED_SIGNAL"

// A SIGTERM a parked image caught goes with the run when it execs the
// switch's target (Handover.Continue): the next image's run takes it first,
// and forwards it to the harness once adopted. Without it the second exec
// dropped the handler that held it (found in review, November 2, 2026).
func TestASignalCaughtWhileParkedReachesTheNextImagesRun(t *testing.T) {
	if os.Getenv(signalChild) != "" {
		parkedImage()
		return
	}
	command := exec.Command(os.Args[0], "-test.run=^TestASignalCaughtWhileParkedReachesTheNextImagesRun$", "-test.v")
	for _, entry := range os.Environ() {
		if !strings.HasPrefix(entry, "REWAKE_") {
			command.Env = append(command.Env, entry)
		}
	}
	command.Env = append(command.Env, signalChild+"=1")
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("%v: %s", err, output)
	}
	if !strings.Contains(string(output), "the next image's run took terminated") {
		t.Fatalf("the SIGTERM caught while parked did not reach the next image's run: %s", output)
	}
}

// parkedImage catches a SIGTERM and continues to its own binary, which reads
// the handover as rewake adopt does and says what its run takes.
func parkedImage() {
	CatchSignals()
	spec := os.Getenv(HandoverEnv)
	if spec == "" {
		if err := syscall.Kill(os.Getpid(), syscall.SIGTERM); err != nil {
			panic(err)
		}
		for deadline := time.Now().Add(2 * time.Second); len(caught.incoming) == 0; {
			if time.Now().After(deadline) {
				panic("the parked image did not catch its SIGTERM")
			}
			time.Sleep(time.Millisecond)
		}
		h := &Handover{Version: HandoverVersion}
		panic(h.Continue("/proc/self/exe"))
	}
	h, err := ReadHandover(spec)
	if err != nil {
		panic(err)
	}
	incoming, restore := takeSignals(h)
	defer restore()
	select {
	case got := <-incoming:
		fmt.Printf("the next image's run took %s\n", got)
	case <-time.After(time.Second):
		fmt.Println("the next image's run took nothing")
	}
}
