package recipes

import (
	"fmt"

	"github.com/inferia/inferia-worker/internal/config/sglang"
)

const sglangShmSize = int64(32) * 1024 * 1024 * 1024 // 32 GB

// sglangRecipe builds an invocation of lmsysorg/sglang. The entrypoint is
// python3 -m sglang.launch_server.
type sglangRecipe struct {
	image     string
	port      int
	readyPath string
}

func (r sglangRecipe) BuildPlan(in BuildInput) (Plan, error) {
	if err := validate(in); err != nil {
		return Plan{}, err
	}
	if err := requireGPU(in); err != nil {
		return Plan{}, err
	}
	model := stripScheme(in.ArtifactURI)
	cfg := sanitiseConfig(in.Config)

	envDefaults := map[string]string{}
	gpuCfg, gpuEnv := sglang.GPUOptimalConfig(in.GPUName, in.GPUMemoryMiB, len(in.GPUIndices))
	for k, v := range gpuCfg {
		if _, ok := cfg[k]; !ok {
			cfg[k] = v
		}
	}
	for k, v := range gpuEnv {
		envDefaults[k] = v
	}

	cmd := []string{
		"python3", "-m", "sglang.launch_server",
		"--model-path", model,
		"--host", "0.0.0.0",
		"--port", fmt.Sprintf("%d", r.port),
	}
	for _, k := range []string{
		"tensor_parallel_size",
		"dtype", "kv_cache_dtype",
		"attention_backend", "sampling_backend",
		"mem_fraction_static", "chunked_prefill_size",
		"quantization",
		"max_model_len", "max_running_requests", "max_total_tokens",
	} {
		if v, ok := cfg[k]; ok {
			cmd = append(cmd, dashed(k), cliArg(v))
		}
	}
	if v, ok := cfg["enable_prefix_caching"].(bool); ok && v {
		cmd = append(cmd, "--enable-prefix-caching")
	}
	if v, ok := cfg["trust_remote_code"].(bool); ok && v {
		cmd = append(cmd, "--trust-remote-code")
	}

	var planMounts []Mount
	var planEntrypoint []string
	if sglang.MooncakeEnabled() {
		sglang.ApplySGLangMooncakePrefillFlags(cfg, envDefaults, &cmd)
		planEntrypoint = sglang.MooncakeEntrypoint()
	}

	env := mergeEnv(in.Env, envDefaults)

	return Plan{
		Image:         r.image,
		ContainerName: containerName("inferia-sglang", in.DeploymentID),
		Cmd:           cmd,
		Entrypoint:    planEntrypoint,
		Env:           env,
		Mounts:        planMounts,
		ContainerPort: r.port,
		HostPort:      in.HostPort,
		GPUIndices:    in.GPUIndices,
		ShmSize:       sglangShmSize,
		ReadyPath:     r.readyPath,
	}, nil
}
