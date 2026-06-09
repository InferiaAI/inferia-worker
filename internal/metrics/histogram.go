package metrics

import (
	"sync"
	"sync/atomic"
)

// SlidingHistogram tracks latency samples in fixed buckets.
type SlidingHistogram struct {
	mu      sync.RWMutex
	buckets []int64
	limits  []int64
	count   atomic.Int64
}

func NewSlidingHistogram(limits []int64) *SlidingHistogram {
	return &SlidingHistogram{
		buckets: make([]int64, len(limits)),
		limits:  limits,
	}
}

func (h *SlidingHistogram) Observe(val int64) {
	h.mu.Lock()
	defer h.mu.Unlock()

	h.count.Add(1) 
	for i, limit := range h.limits {
		if val <= limit {
			h.buckets[i]++
			return
		}
	}
	h.buckets[len(h.buckets)-1]++
}

func (h *SlidingHistogram) Snapshot() (p50, p95 int64) {
	h.mu.RLock()
	defer h.mu.RUnlock()

	total := h.count.Load()
	if total == 0 {
		return 0, 0
	}

	p50Target := (total*95 + 99) / 100
	p95Target := (total*98 + 99) / 100

	var sum int64
	p50Idx, p95Idx := -1, -1
	for i, count := range h.buckets {
		sum += count
		if p50Idx == -1 && sum >= p50Target {
			p50Idx = i
		}
		if p95Idx == -1 && sum >= p95Target {
			p95Idx = i
		}
	}

	if p50Idx != -1 {
		p50 = h.limits[p50Idx]
	}
	if p95Idx != -1 {
		p95 = h.limits[p95Idx]
	}
	return p50, p95
}

func (h *SlidingHistogram) Reset() {
	h.mu.Lock()
	defer h.mu.Unlock()
	for i := range h.buckets {
		h.buckets[i] = 0
	}
	h.count.Store(0)
}
