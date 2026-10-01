package cutover

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/praline-labs/rewake/internal/registry"
	"github.com/praline-labs/rewake/internal/state"
)

// status is what the look reads from /proc/<pid>/status.
type status struct {
	uids   []int
	hasUID bool
	caps   capabilities
}

// capabilities are a process's permitted and inheritable sets.
type capabilities struct {
	permitted, inheritable       uint64
	hasPermitted, hasInheritable bool
}

// beyond says the sets hold a capability that own lacks.
func (c capabilities) beyond(own capabilities) bool {
	return c.hasPermitted && c.permitted&^own.permitted != 0 || c.hasInheritable && c.inheritable&^own.inheritable != 0
}

// user says the process runs as uid by any of its real, effective, saved or
// filesystem ids: any of them may be the one it writes with.
func (s status) user(uid int) bool {
	for _, id := range s.uids {
		if id == uid {
			return true
		}
	}
	return false
}

func readStatus(path string) (status, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return status{}, err
	}
	var parsed status
	scanner := bufio.NewScanner(bytes.NewReader(raw))
	for scanner.Scan() {
		key, value, ok := strings.Cut(scanner.Text(), ":")
		if !ok {
			continue
		}
		switch key {
		case "Uid":
			fields := strings.Fields(value)
			for _, field := range fields {
				id, err := strconv.Atoi(field)
				if err != nil {
					return status{}, fmt.Errorf("its Uid line %q is not numbers", value)
				}
				parsed.uids = append(parsed.uids, id)
			}
			parsed.hasUID = len(fields) > 0
		case "CapPrm", "CapInh":
			caps, err := strconv.ParseUint(strings.TrimSpace(value), 16, 64)
			if err != nil {
				return status{}, fmt.Errorf("its %s line %q is not hexadecimal", key, value)
			}
			if key == "CapPrm" {
				parsed.caps.permitted, parsed.caps.hasPermitted = caps, true
			} else {
				parsed.caps.inheritable, parsed.caps.hasInheritable = caps, true
			}
		}
	}
	return parsed, nil
}

// attribute names the session an earlier-build rewake belongs to: the one
// its environment names — a hook or a shell call of a session carries its
// name — or the one whose record names the process itself as its wrapper,
// which carries no name of its own. It answers why when it cannot.
func (s Scanner) attribute(dir string, pid int, start uint64) (*Address, string) {
	raw, err := os.ReadFile(filepath.Join(dir, "environ"))
	if err != nil {
		return nil, fmt.Sprintf("a rewake of the earlier build whose environment cannot be read (%v), so which session it belongs to is unknown", err)
	}
	env := map[string]string{}
	for _, entry := range bytes.Split(raw, []byte{0}) {
		if key, value, ok := strings.Cut(string(entry), "="); ok {
			env[key] = value
		}
	}
	root := env[state.DirEnv]
	if root == "" {
		// The default an earlier build takes, from that process's own view.
		temp := env["TMPDIR"]
		if temp == "" {
			temp = "/tmp"
		}
		root = filepath.Join(temp, "rewake-"+strconv.Itoa(s.UID))
	}
	if name := env[state.SessionEnv]; name != "" {
		room := env[state.RoomEnv]
		if room == "" {
			room = state.DefaultRoom
		}
		return &Address{Root: filepath.Clean(root), Room: room, Name: name}, "a rewake of the earlier build run for a session"
	}
	rooms, _ := os.ReadDir(filepath.Join(root, "rooms"))
	for _, room := range rooms {
		roomDir := filepath.Join(root, "rooms", room.Name())
		records, _ := os.ReadDir(state.SessionsPath(roomDir))
		for _, record := range records {
			name, ok := strings.CutSuffix(record.Name(), ".json")
			if !ok || strings.HasPrefix(name, ".") {
				continue
			}
			session, err := registry.Load(roomDir, name)
			if err == nil && session.ServicePID == pid && session.ServiceStart == start {
				return &Address{Root: filepath.Clean(root), Room: room.Name(), Name: name}, "the wrapper of a session started by the earlier build"
			}
		}
	}
	return nil, "a rewake of the earlier build that neither its environment nor any session record attributes to a session"
}

// Blockers is what stops a launch at here among what the look found: every
// process not proven another user's or another program, and every
// earlier-build rewake but one proven to belong to another session.
func Blockers(found []Process, here Address) []Process {
	here.Root = filepath.Clean(here.Root)
	var blocking []Process
	for _, process := range found {
		if process.Kind == EarlierRewake && process.Owner != nil && *process.Owner != here {
			continue
		}
		if process.Kind == EarlierRewake && process.Owner != nil {
			process.Detail += ", this one"
		}
		blocking = append(blocking, process)
	}
	return blocking
}
