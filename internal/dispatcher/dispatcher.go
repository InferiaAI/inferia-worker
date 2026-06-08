// Package dispatcher adapts the runtime to the control.Dispatcher interface.
// Split into its own package so cmd/worker stays a thin wiring layer and the
// adapter logic gets unit coverage.
package dispatcher

import (
	"context"
	"fmt"

	"github.com/inferia/inferia-worker/internal/control"
	"github.com/inferia/inferia-worker/internal/metrics"
	"github.com/inferia/inferia-worker/internal/runtime"
	"github.com/inferia/inferia-worker/internal/runtime/recipes"
)

// Dispatcher implements control.Dispatcher. It is the primary adapter between
// the control plane (WS channel) and the worker's local runtime.
type Dispatcher struct {
	Rt        runtime.Runtime
	Telemetry TelemetryReader
	Metrics   *metrics.Collector
	GPUName   string
	GPUMemMiB uint64
}

func NewDispatcher(rt runtime.Runtime, tel TelemetryReader, mc *metrics.Collector, gpuName string, gpuMem uint64) *Dispatcher {
	return &Dispatcher{
		Rt:        rt,
		Telemetry: tel,
		Metrics:   mc,
		GPUName:   gpuName,
		GPUMemMiB: gpuMem,
	}
}


// LoadResult mirrors runtime.LoadResult so this package doesn't import runtime
// (avoiding cycles when runtime imports dispatcher in some future refactor).
type LoadResult struct {
	EndpointURL string
}

// TelemetryReader returns one snapshot of host CPU/memory/GPU usage.
type TelemetryReader interface {
	Read() (used map[string]string)
}

// Dispatcher implements control.Dispatcher.
type Dispatcher struct {
	Rt        Runtime
	Telemetry TelemetryReader
	GPUName   string // populated by main.go from telemetry.ReadGPU()
	GPUMemMiB uint64 // populated by main.go from telemetry.ReadGPU()
}

// LoadModel converts the WS body into a recipes.Plan and asks the runtime to
// load it. Returning a non-nil error becomes CommandResult{status:"failed"}
// in the control package.
func (d *Dispatcher) LoadModel(ctx context.Context, body control.LoadModelBody) (string, error) {
	r, err := recipes.Get(body.Recipe)
	if err != nil {
		return "", err
	}
	port := body.Port
	if port == 0 {
		// recipes.BuildPlan rejects HostPort=0, so use a placeholder and let
		// the runtime allocator override it.
		port = 1
	}
	plan, err := r.BuildPlan(recipes.BuildInput{
		DeploymentID: body.DeploymentID,
		ArtifactURI:  body.Model.ArtifactURI,
		Config:       body.Config,
		GPUIndices:   body.GPUIndices,
		HostPort:     port,
		Env:          body.Env,
		GPUName:      d.GPUName,
		GPUMemoryMiB: d.GPUMemMiB,
	})
	if err != nil {
		return "", err
	}
	if body.Port == 0 {
		plan.HostPort = 0 // signal runtime to allocate
	}
	res, err := d.Rt.LoadModel(ctx, body.DeploymentID, plan)
	if err != nil {
		return "", err
	}
	return res.EndpointURL, nil
}

// UnloadModel is a direct passthrough.
func (d *Dispatcher) UnloadModel(ctx context.Context, body control.UnloadModelBody) error {
	err := d.Rt.UnloadModel(ctx, body.DeploymentID)
	if err == nil && d.Metrics != nil {
		d.Metrics.RemoveDeployment(body.DeploymentID)
	}
	return err
}

func (d *Dispatcher) HeartbeatSnapshot() control.HeartbeatBody {
	used := d.Telemetry.Read()
	models := d.Rt.LoadedDeployments()

	body := control.HeartbeatBody{
		Used:         used,
		LoadedModels: models,
	}

	if d.Metrics != nil {
		// Gather runtime info for all loaded deployments to enrich metrics
		infoMap := make(map[string]struct {
			recipe, model, phase string
			pullDur, startDur    time.Duration
		})
		for _, id := range models {
			r, m, p, pd, sd, ok := d.Rt.DeploymentInfo(id)
			if ok {
				infoMap[id] = struct {
					recipe, model, phase string
					pullDur, startDur    time.Duration
				}{r, m, p, pd, sd}
			}
		}
		body.DeployMetrics = d.Metrics.Snapshot(infoMap)
	}

	return body
}


// HeartbeatSnapshot composes the periodic heartbeat body.
func (d *Dispatcher) HeartbeatSnapshot() control.HeartbeatBody {
	var used map[string]string
	if d.Telemetry != nil {
		used = d.Telemetry.Read()
	} else {
		used = map[string]string{}
	}
	return control.HeartbeatBody{
		Used:         used,
		LoadedModels: d.Rt.LoadedDeployments(),
	}
}

// SafeFmt is a tiny convenience wrapper exposed so cmd/worker can build its
// own TelemetryReader without re-importing fmt. Kept package-private otherwise.
func SafeFmt(format string, args ...any) string { return fmt.Sprintf(format, args...) }
