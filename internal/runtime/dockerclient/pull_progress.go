package dockerclient

import (
	"fmt"
	"time"
)

// pullMessage is the subset of docker's image-pull progress JSON we read.
// The daemon streams one such object per line during ImagePull.
type pullMessage struct {
	Status         string `json:"status"`
	ID             string `json:"id"`
	ProgressDetail struct {
		Current int64 `json:"current"`
		Total   int64 `json:"total"`
	} `json:"progressDetail"`
	Error string `json:"error"`
}

type layerProgress struct{ current, total int64 }

// pullProgressTracker folds the per-layer docker pull stream into an overall
// "downloaded / total" figure and produces throttled, human-readable summary
// lines (e.g. "Pulling image… 42% (14.7 GB / 35.0 GB)") so the worker can
// surface pull progress as deployment logs without spamming a line per chunk.
type pullProgressTracker struct {
	layers   map[string]layerProgress
	lastPct  int
	lastEmit time.Time
	now      func() time.Time
	minGap   time.Duration
}

func newPullProgressTracker() *pullProgressTracker {
	return &pullProgressTracker{
		layers:  map[string]layerProgress{},
		lastPct: -1,
		now:     time.Now,
		minGap:  2500 * time.Millisecond,
	}
}

// update folds one pull message into the running totals and returns a summary
// line plus whether it should be emitted now. Emission is throttled to a >=5%
// jump or one line every minGap, and always fires once at 100%.
func (t *pullProgressTracker) update(m pullMessage) (string, bool) {
	switch m.Status {
	case "Downloading":
		if m.ID != "" && m.ProgressDetail.Total > 0 {
			t.layers[m.ID] = layerProgress{
				current: m.ProgressDetail.Current,
				total:   m.ProgressDetail.Total,
			}
		}
	case "Download complete", "Pull complete":
		if m.ID != "" {
			lp := t.layers[m.ID]
			if lp.total > 0 {
				lp.current = lp.total
				t.layers[m.ID] = lp
			}
		}
	default:
		// Verbose status lines (Extracting, Waiting, Pulling fs layer, the
		// final "Status: Downloaded…") don't move the byte counters.
		return "", false
	}

	var cur, tot int64
	for _, lp := range t.layers {
		cur += lp.current
		tot += lp.total
	}
	if tot == 0 {
		return "", false
	}
	pct := int(cur * 100 / tot)
	if pct > 100 {
		pct = 100
	}

	now := t.now()
	forced := pct >= 100 && t.lastPct < 100
	// Emit on a >=5% move in either direction — the overall pct can dip when a
	// new layer's total is first learned, and showing that is fine.
	bigJump := pct-t.lastPct >= 5 || t.lastPct-pct >= 5
	timeUp := now.Sub(t.lastEmit) >= t.minGap
	if !forced && !bigJump && !timeUp {
		return "", false
	}
	if pct == t.lastPct && !forced && !timeUp {
		return "", false
	}

	t.lastPct = pct
	t.lastEmit = now
	return fmt.Sprintf("Pulling image… %d%% (%s / %s)", pct, humanBytes(cur), humanBytes(tot)), true
}

func humanBytes(b int64) string {
	const (
		gb = 1 << 30
		mb = 1 << 20
	)
	switch {
	case b >= gb:
		return fmt.Sprintf("%.1f GB", float64(b)/gb)
	default:
		return fmt.Sprintf("%.0f MB", float64(b)/mb)
	}
}
