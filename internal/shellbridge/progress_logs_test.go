package shellbridge

import (
	"context"
	"sync"
	"testing"
	"time"
)

type collector struct {
	mu    sync.Mutex
	lines []string // "stream|data"
}

func (c *collector) onLine(stream, data string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.lines = append(c.lines, stream+"|"+data)
}

func (c *collector) snapshot() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.lines...)
}

func closedChan(vals ...string) <-chan string {
	ch := make(chan string, len(vals))
	for _, v := range vals {
		ch <- v
	}
	close(ch)
	return ch
}

func handoffBackend(stream, data, reason string) *fakeLogsBackend {
	return &fakeLogsBackend{
		scriptedLines: []struct {
			stream string
			data   string
		}{{stream, data}},
		endReason: reason,
	}
}

func TestProgressLogsBackend_ReplayThenHandoff(t *testing.T) {
	rt := fakeRuntime{
		known:       map[string]bool{"dep": true},
		progReplay:  map[string][]string{"dep": {"Pulling image x …", "Pulling image… 50% (5 GB / 10 GB)"}},
		progLines:   map[string]<-chan string{"dep": closedChan()},
		progOutcome: map[string]func() (string, bool){"dep": func() (string, bool) { return "cid-1", false }},
	}
	handoff := handoffBackend("stdout", "INFO engine ready", "container exited")
	b := newProgressLogsBackend(rt, "dep", func(string) LogsBackend { return handoff })

	c := &collector{}
	reason, err := b.Stream(context.Background(), c.onLine)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if reason != "container exited" {
		t.Errorf("reason = %q, want container exited", reason)
	}
	want := []string{
		"stdout|Pulling image x …",
		"stdout|Pulling image… 50% (5 GB / 10 GB)",
		"stdout|INFO engine ready",
	}
	got := c.snapshot()
	if len(got) != len(want) {
		t.Fatalf("lines = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("line[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestProgressLogsBackend_FailedEndsWithoutHandoff(t *testing.T) {
	rt := fakeRuntime{
		known:       map[string]bool{"dep": true},
		progReplay:  map[string][]string{"dep": {"Failed: pull: context deadline exceeded"}},
		progLines:   map[string]<-chan string{"dep": closedChan()},
		progOutcome: map[string]func() (string, bool){"dep": func() (string, bool) { return "", true }},
	}
	handoffCalled := false
	b := newProgressLogsBackend(rt, "dep", func(string) LogsBackend {
		handoffCalled = true
		return &fakeLogsBackend{}
	})

	reason, _ := b.Stream(context.Background(), (&collector{}).onLine)
	if reason != "deployment failed" {
		t.Errorf("reason = %q, want deployment failed", reason)
	}
	if handoffCalled {
		t.Errorf("handoff must not run on failure")
	}
}

func TestProgressLogsBackend_FollowsLiveThenHandsOff(t *testing.T) {
	lines := make(chan string)
	rt := fakeRuntime{
		known:       map[string]bool{"dep": true},
		progReplay:  map[string][]string{"dep": {}},
		progLines:   map[string]<-chan string{"dep": lines},
		progOutcome: map[string]func() (string, bool){"dep": func() (string, bool) { return "cid", false }},
	}
	b := newProgressLogsBackend(rt, "dep", func(string) LogsBackend {
		return handoffBackend("stdout", "served", "container exited")
	})
	c := &collector{}
	done := make(chan string, 1)
	go func() {
		r, _ := b.Stream(context.Background(), c.onLine)
		done <- r
	}()
	lines <- "Starting container…"
	close(lines)

	select {
	case r := <-done:
		if r != "container exited" {
			t.Errorf("reason = %q", r)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Stream did not finish")
	}
	got := c.snapshot()
	if len(got) != 2 || got[0] != "stdout|Starting container…" || got[1] != "stdout|served" {
		t.Errorf("lines = %v", got)
	}
}

func TestProgressLogsBackend_NoProgressIsClean(t *testing.T) {
	rt := fakeRuntime{} // SubscribeProgress returns ok=false
	b := newProgressLogsBackend(rt, "dep", func(string) LogsBackend { return &fakeLogsBackend{} })
	reason, err := b.Stream(context.Background(), (&collector{}).onLine)
	if err != nil || reason != "deployment has no progress" {
		t.Errorf("reason=%q err=%v", reason, err)
	}
}
