// Command standin does what rewake 1.x does to its tree, its bugs included,
// for the upgrade rig (docs/v2/stage3-upgrade.md#the-tests): it makes a room
// by MkdirAll in its tree — <base>/rooms, as 1.x names it, or a tree set aside
// that a 1.x process opened before the rename — takes the room's .launch.lock with a
// blocking flock by path and never checks the inode it locked, publishes a
// session record with its own pid, start and pid namespace, writes letters by
// path and holds a name or a mailbox lock. A 1.x binary cannot be built in
// this tree, so this is what the clearing is tested against.
//
//	standin room <tree> <room>               make the room and exit
//	standin session <tree> <room> <name>     launch: lock, publish, unlock, run until killed
//	standin launching <tree> <room>          take the launch lock and hold it, unregistered
//	standin hold <tree> <room> <rel>         take the lock at <room>/<rel> and hold it
//	standin letter <tree> <room> <to> <id>   write a waiting letter and exit
//
// A command that holds or runs prints "ready" once it does.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) < 4 {
		fail(fmt.Errorf("usage: standin <op> <tree> <room> [args]"))
	}
	op, tree, name := os.Args[1], os.Args[2], os.Args[3]
	room := filepath.Join(tree, name)
	args := os.Args[4:]
	makeRoom(room)
	switch op {
	case "room":
	case "session":
		lock := lockPath(filepath.Join(room, ".launch.lock"))
		publish(room, args[0])
		_ = lock.Close()
		run()
	case "launching":
		lockPath(filepath.Join(room, ".launch.lock"))
		run()
	case "hold":
		lockPath(filepath.Join(room, args[0]))
		run()
	case "letter":
		path := filepath.Join(room, "inbox", args[0], args[1]+".json")
		check(os.MkdirAll(filepath.Dir(path), 0o700))
		check(os.WriteFile(path, []byte(`{"id":"`+args[1]+`","to":"`+args[0]+`","text":"left by 1.x"}`), 0o600))
	default:
		fail(fmt.Errorf("unknown op %q", op))
	}
}

// makeRoom is 1.x's RoomDir: whatever is missing is made, by path.
func makeRoom(room string) {
	for _, sub := range []string{"sessions", "inbox", "sock"} {
		check(os.MkdirAll(filepath.Join(room, sub), 0o700))
	}
}

// lockPath takes a lock by path, waiting, as 1.x does: the file it opened is
// never compared with the one at the path afterwards.
func lockPath(path string) *os.File {
	check(os.MkdirAll(filepath.Dir(path), 0o700))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	check(err)
	check(syscall.Flock(int(file.Fd()), syscall.LOCK_EX))
	return file
}

func publish(room, name string) {
	stat, err := os.ReadFile("/proc/self/stat")
	check(err)
	fields := strings.Fields(string(stat)[strings.LastIndex(string(stat), ")")+1:])
	start, err := strconv.ParseUint(fields[19], 10, 64)
	check(err)
	namespace, err := os.Readlink("/proc/self/ns/pid")
	check(err)
	record, err := json.Marshal(map[string]any{
		"name": name, "room": filepath.Base(room), "servicePid": os.Getpid(),
		"serviceStart": start, "pidNamespace": namespace,
	})
	check(err)
	check(os.WriteFile(filepath.Join(room, "sessions", name+".json"), record, 0o600))
}

// run says it is ready and runs until it is killed.
func run() {
	fmt.Println("ready")
	for {
		time.Sleep(time.Hour)
	}
}

func check(err error) {
	if err != nil {
		fail(err)
	}
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "standin:", err)
	os.Exit(2)
}
