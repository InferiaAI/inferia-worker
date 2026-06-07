package shellbridge

import "context"

// progressLogsBackend streams a deployment's pre-container lifecycle / image
// pull progress (from the runtime's progress buffer) as logs, then hands off
// to real container logs once the container has started. It's used when a logs
// request arrives for a deployment that is loading but whose model container
// does not yet exist (e.g. during a multi-minute image pull), where the docker
// logs backend would otherwise fail with "no running container".
type progressLogsBackend struct {
	rt  Runtime
	dep string
	// handoff builds the real container-logs backend once the container id is
	// known. Injected so tests don't need a docker daemon.
	handoff func(containerID string) LogsBackend
}

func newProgressLogsBackend(rt Runtime, deployment string, handoff func(containerID string) LogsBackend) LogsBackend {
	return &progressLogsBackend{rt: rt, dep: deployment, handoff: handoff}
}

func (b *progressLogsBackend) Stream(ctx context.Context, onLine func(stream, data string)) (string, error) {
	replay, lines, outcome, ok := b.rt.SubscribeProgress(b.dep)
	if !ok {
		return "deployment has no progress", nil
	}

	for _, l := range replay {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
			onLine("stdout", l)
		}
	}

	following := true
	for following {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case l, open := <-lines:
			if !open {
				following = false
				break
			}
			onLine("stdout", l)
		}
	}

	cid, failed := outcome()
	switch {
	case failed:
		return "deployment failed", nil
	case cid == "":
		return "deployment ended", nil
	default:
		// Container started — stream its real logs from here on.
		return b.handoff(cid).Stream(ctx, onLine)
	}
}
