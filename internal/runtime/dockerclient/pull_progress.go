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
	// download aggregate (status "Downloading" / "Download complete")
	layers   map[string]layerProgress
	lastPct  int
	lastEmit time.Time
	// extraction aggregate (status "Extracting"; finalized by "Pull complete")
	ex         map[string]layerProgress
	lastExPct  int
	lastExEmit time.Time

	now    func() time.Time
	minGap time.Duration
}

func newPullProgressTracker() *pullProgressTracker {
	return &pullProgressTracker{
		layers:    map[string]layerProgress{},
		lastPct:   -1,
		ex:        map[string]layerProgress{},
		lastExPct: -1,
		now:       time.Now,
		minGap:    2500 * time.Millisecond,
	}
}

// update folds one pull message into the running totals and returns a summary
// line plus whether it should be emitted now. Download and extraction are
// tracked + throttled independently; docker interleaves them (early layers
// extract while later layers still download).
func (t *pullProgressTracker) update(m pullMessage) (string, bool) {
	switch m.Status {
	case "Downloading":
		if m.ID != "" && m.ProgressDetail.Total > 0 {
			t.layers[m.ID] = layerProgress{
				current: m.ProgressDetail.Current,
				total:   m.ProgressDetail.Total,
			}
		}
		return t.emitPhase(t.layers, &t.lastPct, &t.lastEmit, "Pulling image")
	case "Download complete":
		finalizeLayer(t.layers, m.ID)
		return t.emitPhase(t.layers, &t.lastPct, &t.lastEmit, "Pulling image")
	case "Extracting":
		if m.ID != "" && m.ProgressDetail.Total > 0 {
			t.ex[m.ID] = layerProgress{
				current: m.ProgressDetail.Current,
				total:   m.ProgressDetail.Total,
			}
		}
		return t.emitPhase(t.ex, &t.lastExPct, &t.lastExEmit, "Extracting image")
	case "Pull complete":
		// A layer is fully extracted. Finalize BOTH aggregates (download is
		// normally already 100% via "Download complete"; do it defensively),
		// and emit the EXTRACTION line — the meaningful late-phase signal that
		// fills the dead-air after download hits 100%.
		finalizeLayer(t.layers, m.ID)
		finalizeLayer(t.ex, m.ID)
		return t.emitPhase(t.ex, &t.lastExPct, &t.lastExEmit, "Extracting image")
	default:
		// Verbose status lines (Waiting, Pulling fs layer, Already exists, the
		// final "Status: Downloaded…") don't move byte counters.
		return "", false
	}
}

// finalizeLayer pins a layer to its known total (100% for that layer).
func finalizeLayer(layers map[string]layerProgress, id string) {
	if id == "" {
		return
	}
	lp := layers[id]
	if lp.total > 0 {
		lp.current = lp.total
		layers[id] = lp
	}
}

// emitPhase aggregates one phase's layers into an overall percentage and
// returns a throttled summary line. Emission fires on a >=5% move, once every
// minGap, or once forced at 100%. lastPct/lastEmit are the phase's own state
// (passed by pointer) so download and extraction throttle independently.
func (t *pullProgressTracker) emitPhase(
	layers map[string]layerProgress,
	lastPct *int,
	lastEmit *time.Time,
	label string,
) (string, bool) {
	var cur, tot int64
	for _, lp := range layers {
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
	forced := pct >= 100 && *lastPct < 100
	bigJump := pct-*lastPct >= 5 || *lastPct-pct >= 5
	timeUp := now.Sub(*lastEmit) >= t.minGap
	if !forced && !bigJump && !timeUp {
		return "", false
	}
	*lastPct = pct
	*lastEmit = now
	return fmt.Sprintf("%s… %d%% (%s / %s)", label, pct, humanBytes(cur), humanBytes(tot)), true
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
