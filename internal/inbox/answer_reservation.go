package inbox

import (
	"os"
	"path/filepath"
	"time"

	"github.com/iiiokojiadbi/rewake/internal/state"
)

// ReserveAnswer starts the lease before a question can produce a reply. The
// heartbeat runs during delivery and printing too; a stopped process loses
// its lease, while a live one waiting for stdout keeps its reservation.
func ReserveAnswer(dir, name, question string) (func(), error) {
	marks := state.AnsweringPath(dir, name)
	if err := state.EnsureSubdir(marks); err != nil {
		return nil, err
	}
	mark := filepath.Join(marks, question)
	if err := os.WriteFile(mark, nil, 0o600); err != nil {
		return nil, err
	}
	stop, done := make(chan struct{}), make(chan struct{})
	go func() {
		defer close(done)
		ticker := time.NewTicker(answerPoll)
		defer ticker.Stop()
		for {
			select {
			case <-stop:
				return
			case now := <-ticker.C:
				_ = os.Chtimes(mark, now, now)
			}
		}
	}()
	return func() {
		close(stop)
		<-done
		removeMark(dir, name, mark)
	}, nil
}

// AvailableUnread excludes reports owned by waiting sends. The caller holds
// the mailbox lock through printing and marking, just like a waiting send.
func AvailableUnread(dir, name, epoch string) ([]Message, error) {
	messages, err := PeekUnread(dir, name, epoch)
	if err != nil {
		return nil, err
	}
	available := make([]Message, 0, len(messages))
	for _, message := range messages {
		if !awaitedHere(dir, name, message) {
			available = append(available, message)
		}
	}
	return available, nil
}

func answerReceiptsPath(dir, name string) string {
	return filepath.Join(state.InboxPath(dir, name), "received")
}

// A missing lease can mean either successful output or an abandoned command.
// Receipts distinguish the two so one consumer cannot archive a shared report
// before all its questions have received that specific report. A stopped
// receipt cannot confirm the later finished outcome for the same question.
// Unclaimed ids leave the report
// available to ordinary inbox delivery when no active lease remains.
func receiveAnswer(dir, name, epoch, question string, message Message) error {
	receipts := answerReceiptsPath(dir, name)
	if err := state.EnsureSubdir(receipts); err != nil {
		return err
	}
	if err := state.WriteAtomic(filepath.Join(receipts, question), []byte(message.ID)); err != nil {
		return err
	}
	removeMark(dir, name, filepath.Join(state.AnsweringPath(dir, name), question))
	if awaitedHere(dir, name, message) {
		return nil
	}
	for _, id := range message.InReplyTo {
		received, err := os.ReadFile(filepath.Join(receipts, id))
		if err != nil || string(received) != message.ID {
			return nil
		}
	}
	return MarkRead(dir, name, epoch, message, false)
}
