package gateway

import (
	"sync"
	"testing"
	"time"
)

func TestNoticeDisplayConcurrentBindingAndOwnerLoss(t *testing.T) {
	c, b := passiveNoticeDisplay(t)
	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 8; j++ {
				c.displayNotice(b, "turn", "notice")
			}
		}()
	}
	wg.Add(3)
	go func() { defer wg.Done(); c.owner.mu.Lock(); c.owner.current = nil; c.owner.mu.Unlock() }()
	go func() {
		defer wg.Done()
		c.mu.Lock()
		c.state.Generation++
		c.state.Connection++
		c.state.Epoch = "replacement"
		c.mu.Unlock()
	}()
	go func() { defer wg.Done(); c.cancel() }()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("display publication deadlocked during owner change")
	}
	if c.displayNotice(b, "turn", "notice") {
		t.Fatal("old binding published after owner loss")
	}
}

func TestNoticeDisplayNeverBlocksOrDisplacesNativeQueue(t *testing.T) {
	c, b := passiveNoticeDisplay(t)
	for len(c.toUI) < cap(c.toUI) {
		c.toUI <- []byte("native")
	}
	done := make(chan bool, 1)
	go func() { done <- c.displayNotice(b, "turn", "notice") }()
	select {
	case accepted := <-done:
		if accepted {
			t.Fatal("display accepted on full queue")
		}
	case <-time.After(time.Second):
		t.Fatal("display blocked on full queue")
	}
	if len(c.toUI) != cap(c.toUI) {
		t.Fatal("display displaced native traffic")
	}
	for len(c.toUI) > 0 {
		if string(<-c.toUI) != "native" {
			t.Fatal("native frame changed")
		}
	}
}
