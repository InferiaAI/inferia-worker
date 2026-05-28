package recipes

import "fmt"

// vllmRecipe builds an invocation of vllm/vllm-openai. It is also reused for
// vllm-omni, which has the same CLI surface.
type vllmRecipe struct {
	image     string
	port      int
	readyPath string
}

func (r vllmRecipe) BuildPlan(in BuildInput) (Plan, error) {
	if err := validate(in); err != nil {
		return Plan{}, err
	}
	// vLLM is GPU-only: enforce at least one index after shared validation.
	if err := requireGPU(in); err != nil {
		return Plan{}, err
	}
	model := stripScheme(in.ArtifactURI)
	cfg := sanitiseConfig(in.Config)

	cmd := []string{
		model,
		"--served-model-name", model,
		"--host", "0.0.0.0",
		"--port", fmt.Sprintf("%d", r.port),
	}
	// Apply config flags. Use --kebab-case to match vLLM convention.
	for _, k := range []string{
		"tensor_parallel_size", "pipeline_parallel_size",
		"dtype", "max_model_len", "max_num_seqs",
		"gpu_memory_utilization", "quantization",
		"max_batch_size", "max_input_length", "max_total_tokens",
	} {
		if v, ok := cfg[k]; ok {
			cmd = append(cmd, dashed(k), cliArg(v))
		}
	}
	if v, ok := cfg["enforce_eager"].(bool); ok && v {
		cmd = append(cmd, "--enforce-eager")
	}
	if v, ok := cfg["trust_remote_code"].(bool); ok && v {
		cmd = append(cmd, "--trust-remote-code")
	}

	env := mergeEnv(in.Env, map[string]string{
		"CUDA_MODULE_LOADING": "LAZY",
	})

	return Plan{
		Image:         r.image,
		ContainerName: containerName("inferia-vllm", in.DeploymentID),
		Cmd:           cmd,
		Env:           env,
		ContainerPort: r.port,
		HostPort:      in.HostPort,
		GPUIndices:    in.GPUIndices,
		ReadyPath:     r.readyPath,
	}, nil
}

func mergeEnv(user, defaults map[string]string) map[string]string {
	out := map[string]string{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range user {
		out[k] = v
	}
	return out
}
