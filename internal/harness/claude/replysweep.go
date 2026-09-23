package claude

import (
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// replySuffix ends every reply socket rewake makes beside a session's own.
const replySuffix = ".reply.sock"

// replyGrace is how old a reply socket has to be before a refused dial counts
// as its owner's death.
const replyGrace = 5 * time.Second

// sweepReplies removes the reply sockets of wrappers that died without closing
// their lane — killed, say — which nothing else would ever remove. A socket
// counts as dead only when a dial is refused: nobody listens there any more.
// A file that merely exists, or a dial that fails some other way, proves
// nothing, and that socket stays. So does a young one: a socket is bound before
// it listens, and a wrapper starting beside this one may be between the two —
// refusing now, and left on an unlinked inode, deaf to its receipts, if removed.
func sweepReplies(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), replySuffix) || entry.Type()&os.ModeSocket == 0 {
			continue
		}
		path := filepath.Join(dir, entry.Name())
		if info, err := entry.Info(); err != nil || time.Since(info.ModTime()) < replyGrace {
			continue
		}
		connection, err := net.DialTimeout("unix", path, 100*time.Millisecond)
		if err == nil {
			_ = connection.Close()
			continue
		}
		if errors.Is(err, syscall.ECONNREFUSED) {
			_ = os.Remove(path)
		}
	}
}
