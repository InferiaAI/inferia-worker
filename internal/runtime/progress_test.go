package runtime

import (
	"testing"
	"time"
)

func recvWithin(t *testing.T, ch <-chan string, d time.Duration) (string, bool) {
	t.Helper()
	select {
	case s, ok := <-ch:
		return s, ok
	case <-time.After(d):
		t.Fatalf("timed out waiting for channel")
		return "", false
	}
}

func TestProgressLog_ReplayThenFollow(t *testing.T) {
	p := newProgressLog()
	p.append("Pulling image x …")
	p.append("Pulling image… 50% (5 GB / 10 GB)")

	replay, ch := p.subscribe()
	if len(replay) != 2 || replay[0] != "Pulling image x …" {
		t.Fatalf("replay = %v", replay)
	}
	p.append("Image ready. Creating container…")
	got, ok := recvWithin(t, ch, time.Second)
	if !ok || got != "Image ready. Creating container…" {
		t.Errorf("follow got %q ok=%v", got, ok)
	}
}

func TestProgressLog_ContainerStartedClosesAndHandsOff(t *testing.T) {
	p := newProgressLog()
	_, ch := p.subscribe()
	p.markContainerStarted("cid-123")

	if _, ok := recvWithin(t, ch, time.Second); ok {
		t.Errorf("channel should be closed after markContainerStarted")
	}
	cid, failed := p.outcome()
	if cid != "cid-123" || failed {
		t.Errorf("outcome = (%q,%v), want (cid-123,false)", cid, failed)
	}
}

func TestProgressLog_FailedClosesWithFailure(t *testing.T) {
	p := newProgressLog()
	_, ch := p.subscribe()
	p.append("Failed: pull: context deadline exceeded")
	p.markFailed()

	// drain: the appended line may arrive before close.
	for {
		if _, ok := recvWithin(t, ch, time.Second); !ok {
			break
		}
	}
	cid, failed := p.outcome()
	if cid != "" || !failed {
		t.Errorf("outcome = (%q,%v), want (\"\",true)", cid, failed)
	}
}

func TestProgressLog_SubscribeAfterDoneIsPreClosed(t *testing.T) {
	p := newProgressLog()
	p.markContainerStarted("c")
	_, ch := p.subscribe()
	if _, ok := recvWithin(t, ch, time.Second); ok {
		t.Errorf("subscribe after terminal should return a closed channel")
	}
}

func TestProgressLog_AppendAfterDoneIsNoop(t *testing.T) {
	p := newProgressLog()
	p.markFailed()
	p.append("late") // must not panic / must be ignored
	replay, _ := p.subscribe()
	for _, l := range replay {
		if l == "late" {
			t.Errorf("append after done should be ignored")
		}
	}
}

func TestRuntime_DeploymentKnownAndSubscribe(t *testing.T) {
	r := New(Config{})
	if r.DeploymentKnown("dep") {
		t.Errorf("unknown deployment reported known")
	}
	if _, _, _, ok := r.SubscribeProgress("dep"); ok {
		t.Errorf("SubscribeProgress should be ok=false for unknown deployment")
	}

	d := r.getOrCreate("dep")
	if !r.DeploymentKnown("dep") {
		t.Errorf("registered deployment not reported known")
	}
	r.appendProgress("dep", "hello")
	replay, lines, outcome, ok := r.SubscribeProgress("dep")
	if !ok {
		t.Fatalf("SubscribeProgress ok=false for known deployment")
	}
	if len(replay) != 1 || replay[0] != "hello" {
		t.Errorf("replay = %v", replay)
	}
	d.prog.markContainerStarted("cid")
	if _, open := recvWithin(t, lines, time.Second); open {
		t.Errorf("lines should close after container start")
	}
	if cid, failed := outcome(); cid != "cid" || failed {
		t.Errorf("outcome = (%q,%v)", cid, failed)
	}
}
