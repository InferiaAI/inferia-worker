package dockerclient

import (
	"strings"
	"testing"
	"time"
)

func dl(id string, cur, tot int64) pullMessage {
	m := pullMessage{Status: "Downloading", ID: id}
	m.ProgressDetail.Current = cur
	m.ProgressDetail.Total = tot
	return m
}

func TestPullProgress_AggregatesAcrossLayers(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }

	// First layer at 50% of 10 GB → 5 GB of 10 GB total = 50%.
	line, ok := tr.update(dl("a", 5<<30, 10<<30))
	if !ok {
		t.Fatalf("expected first update to emit")
	}
	if !strings.Contains(line, "50%") || !strings.Contains(line, "GB") {
		t.Errorf("line = %q, want 50%% and GB", line)
	}

	// Second layer appears (10 GB total, 0 downloaded). Overall now 5/20 = 25%.
	// pct dropped >5% → emits.
	line, ok = tr.update(dl("b", 0, 10<<30))
	if !ok || !strings.Contains(line, "25%") {
		t.Errorf("after 2nd layer: ok=%v line=%q, want 25%%", ok, line)
	}
}

func TestPullProgress_ThrottlesSamePercent(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }

	if _, ok := tr.update(dl("a", 1<<30, 10<<30)); !ok {
		t.Fatalf("first emit expected")
	}
	// Same byte counts, no time advance → no re-emit.
	if _, ok := tr.update(dl("a", 1<<30, 10<<30)); ok {
		t.Errorf("expected throttle (same pct, no time passed)")
	}
	// Advance past minGap → emits a keep-alive even at same pct.
	clock = clock.Add(3 * time.Second)
	if _, ok := tr.update(dl("a", 1<<30, 10<<30)); !ok {
		t.Errorf("expected emit after minGap elapsed")
	}
}

func TestPullProgress_BigJumpEmitsWithoutTime(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(100, 0)
	tr.now = func() time.Time { return clock } // never advances

	tr.update(dl("a", 1<<30, 100<<30)) // 1%
	// Jump to ~20% with no time passing → big-jump path emits.
	line, ok := tr.update(dl("a", 20<<30, 100<<30))
	if !ok || !strings.Contains(line, "20%") {
		t.Errorf("big jump: ok=%v line=%q, want 20%%", ok, line)
	}
}

func TestPullProgress_CompleteForces100(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }

	tr.update(dl("a", 9<<30, 10<<30)) // 90%
	// "Download complete" pins the layer to its total → 100%, forced emit.
	line, ok := tr.update(pullMessage{Status: "Download complete", ID: "a"})
	if !ok || !strings.Contains(line, "100%") {
		t.Errorf("complete: ok=%v line=%q, want 100%%", ok, line)
	}
}

func TestPullProgress_IgnoresNonByteStatuses(t *testing.T) {
	tr := newPullProgressTracker()
	for _, s := range []string{"Pulling fs layer", "Waiting", "Status: Downloaded newer image"} {
		if line, ok := tr.update(pullMessage{Status: s, ID: "a"}); ok {
			t.Errorf("status %q should not emit, got %q", s, line)
		}
	}
}

func ex(id string, cur, tot int64) pullMessage {
	m := pullMessage{Status: "Extracting", ID: id}
	m.ProgressDetail.Current = cur
	m.ProgressDetail.Total = tot
	return m
}

func TestPullProgress_ExtractionEmitsSeparately(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }

	line, ok := tr.update(ex("a", 3<<30, 10<<30))
	if !ok || !strings.Contains(line, "Extracting image") || !strings.Contains(line, "30%") || !strings.Contains(line, "GB") {
		t.Errorf("extract: ok=%v line=%q, want 'Extracting image' + 30%% + GB", ok, line)
	}
}

func TestPullProgress_DownloadAndExtractInterleave(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }

	dline, dok := tr.update(dl("a", 5<<30, 10<<30))
	if !dok || !strings.Contains(dline, "Pulling image") || !strings.Contains(dline, "50%") {
		t.Errorf("download line=%q ok=%v", dline, dok)
	}
	eline, eok := tr.update(ex("b", 2<<30, 10<<30))
	if !eok || !strings.Contains(eline, "Extracting image") || !strings.Contains(eline, "20%") {
		t.Errorf("extract line=%q ok=%v", eline, eok)
	}
}

func TestPullProgress_ExtractNoTotalDoesNotEmit(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }
	if line, ok := tr.update(pullMessage{Status: "Extracting", ID: "a"}); ok {
		t.Errorf("extract with no total should not emit, got %q", line)
	}
}

func TestPullProgress_PullCompleteFinalizesExtraction(t *testing.T) {
	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }
	tr.update(ex("a", 9<<30, 10<<30))
	line, ok := tr.update(pullMessage{Status: "Pull complete", ID: "a"})
	if !ok || !strings.Contains(line, "Extracting image") || !strings.Contains(line, "100%") {
		t.Errorf("pull-complete: ok=%v line=%q, want Extracting 100%%", ok, line)
	}
}

func TestHumanBytes(t *testing.T) {
	if got := humanBytes(2 << 30); got != "2.0 GB" {
		t.Errorf("humanBytes(2GiB) = %q", got)
	}
	if got := humanBytes(512 << 20); got != "512 MB" {
		t.Errorf("humanBytes(512MiB) = %q", got)
	}
}

func TestPullProgress_FinalizeEmptyIdAndClamp(t *testing.T) {
	// finalizeLayer with empty id must be a no-op (not panic).
	finalizeLayer(map[string]layerProgress{}, "")

	tr := newPullProgressTracker()
	clock := time.Unix(0, 0)
	tr.now = func() time.Time { return clock }
	// Docker can over-report current beyond total; the pct is clamped to 100.
	line, ok := tr.update(dl("a", 12<<30, 10<<30))
	if !ok || !strings.Contains(line, "100%") {
		t.Errorf("clamp: ok=%v line=%q, want clamped 100%%", ok, line)
	}
}
