package runtime

import "sync"

// progressLog is a per-deployment buffer of human-readable lifecycle lines
// (pulling the image, pull progress, creating/starting the container, …) that
// the worker surfaces as "logs" while the model container does not yet exist.
// Followers replay the buffered lines, then stream new ones until the load
// reaches a terminal point: the container started (switch to real container
// logs) or the load failed (end the stream).
type progressLog struct {
	mu     sync.Mutex
	lines  []string
	subs   map[chan string]struct{}
	cid    string // set once the container is started → handoff signal
	failed bool
	done   bool // terminal: container started or failed
}

const progressLogCap = 1000

func newProgressLog() *progressLog {
	return &progressLog{subs: map[chan string]struct{}{}}
}

// append records a line and fans it out to live followers. No-op once terminal.
func (p *progressLog) append(line string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.lines = append(p.lines, line)
	if len(p.lines) > progressLogCap {
		p.lines = p.lines[len(p.lines)-progressLogCap:]
	}
	for ch := range p.subs {
		select {
		case ch <- line:
		default: // slow follower — drop rather than block the loader
		}
	}
}

// markContainerStarted records the started container id and ends progress
// streaming so followers hand off to real container logs.
func (p *progressLog) markContainerStarted(cid string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.cid = cid
	p.done = true
	p.closeSubsLocked()
}

// markFailed ends progress streaming after a pre-container failure.
func (p *progressLog) markFailed() {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.done {
		return
	}
	p.failed = true
	p.done = true
	p.closeSubsLocked()
}

func (p *progressLog) closeSubsLocked() {
	for ch := range p.subs {
		close(ch)
		delete(p.subs, ch)
	}
}

// subscribe returns the buffered lines plus a channel of new ones. The channel
// is closed when the load terminates (container started or failed); if it's
// already terminal the channel is returned pre-closed.
func (p *progressLog) subscribe() (replay []string, ch chan string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	replay = append([]string(nil), p.lines...)
	ch = make(chan string, 64)
	if p.done {
		close(ch)
		return replay, ch
	}
	p.subs[ch] = struct{}{}
	return replay, ch
}

func (p *progressLog) outcome() (cid string, failed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.cid, p.failed
}

// --- Runtime methods consumed by shellbridge (see shellbridge.Runtime). ---

// appendProgress adds a lifecycle/progress line for an in-flight load. Safe to
// call for an unknown deployment (no-op).
func (r *Runtime) appendProgress(deploymentID, line string) {
	r.mu.Lock()
	d, ok := r.deployments[deploymentID]
	r.mu.Unlock()
	if ok && d.prog != nil {
		d.prog.append(line)
	}
}

func (r *Runtime) progressOf(deploymentID string) *progressLog {
	r.mu.Lock()
	defer r.mu.Unlock()
	if d, ok := r.deployments[deploymentID]; ok {
		return d.prog
	}
	return nil
}

// DeploymentKnown reports whether the worker is currently loading or running
// the deployment (so a logs request can fall back to lifecycle/progress output
// when no container exists yet).
func (r *Runtime) DeploymentKnown(deploymentID string) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	_, ok := r.deployments[deploymentID]
	return ok
}

// SubscribeProgress implements shellbridge.Runtime. See that interface for the
// contract. ok is false when the deployment has no progress log.
func (r *Runtime) SubscribeProgress(deploymentID string) (replay []string, lines <-chan string, outcome func() (string, bool), ok bool) {
	p := r.progressOf(deploymentID)
	if p == nil {
		return nil, nil, nil, false
	}
	rep, ch := p.subscribe()
	return rep, ch, p.outcome, true
}
