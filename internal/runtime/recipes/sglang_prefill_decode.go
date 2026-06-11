package recipes

import (
	"fmt"

	"github.com/inferia/inferia-worker/internal/config/sglang"
)

func init() {
	multiRegistry["sglang-prefill-decode"] = sglangPrefillDecodeRecipe{
		image:     "lmsysorg/sglang:latest-runtime",
		port:      30000,
		readyPath: "/health",
	}
}

type sglangPrefillDecodeRecipe struct {
	image     string
	port      int
	readyPath string
}

func (r sglangPrefillDecodeRecipe) BuildDeploymentPlan(in BuildInput) (DeploymentPlan, error) {
	if err := validate(in); err != nil {
		return DeploymentPlan{}, err
	}
	if err := requireGPU(in); err != nil {
		return DeploymentPlan{}, err
	}

	prefillReplicas := in.PrefillReplicas
	if prefillReplicas < 1 {
		prefillReplicas = 1
	}
	decodeReplicas := in.DecodeReplicas
	if decodeReplicas < 1 {
		decodeReplicas = 1
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

	baseCmd := []string{
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
			baseCmd = append(baseCmd, dashed(k), cliArg(v))
		}
	}
	if v, ok := cfg["enable_prefix_caching"].(bool); ok && v {
		baseCmd = append(baseCmd, "--enable-prefix-caching")
	}
	if v, ok := cfg["trust_remote_code"].(bool); ok && v {
		baseCmd = append(baseCmd, "--trust-remote-code")
	}

	var planMounts []Mount
	var planEntrypoint []string
	if sglang.MooncakeEnabled() {
		planEntrypoint = sglang.MooncakeEntrypoint()
	}

	env := mergeEnv(in.Env, envDefaults)

	prefills := make([]ContainerPlan, prefillReplicas)
	for i := range prefills {
		pCmd := make([]string, len(baseCmd))
		copy(pCmd, baseCmd)
		pEnv := make(map[string]string, len(env))
		for k, v := range env {
			pEnv[k] = v
		}
		if sglang.MooncakeEnabled() {
			sglang.ApplySGLangMooncakePrefillFlags(cfg, pEnv, &pCmd)
		}
		prefills[i] = ContainerPlan{
			Image:         r.image,
			Cmd:           pCmd,
			Entrypoint:    planEntrypoint,
			Env:           pEnv,
			Mounts:        planMounts,
			ContainerPort: r.port,
			GPUIndices:    in.GPUIndices,
			ShmSize:       sglangShmSize,
			ReadyPath:     r.readyPath,
			Role:          KvRoleProducer,
			ReplicaIdx:    i,
		}
	}

	decodes := make([]ContainerPlan, decodeReplicas)
	for i := range decodes {
		dCmd := make([]string, len(baseCmd))
		copy(dCmd, baseCmd)
		dEnv := make(map[string]string, len(env))
		for k, v := range env {
			dEnv[k] = v
		}
		if sglang.MooncakeEnabled() {
			sglang.ApplySGLangMooncakeDecodeFlags(cfg, dEnv, &dCmd)
		}
		decodes[i] = ContainerPlan{
			Image:         r.image,
			Cmd:           dCmd,
			Entrypoint:    planEntrypoint,
			Env:           dEnv,
			Mounts:        planMounts,
			ContainerPort: r.port,
			GPUIndices:    in.GPUIndices,
			ShmSize:       sglangShmSize,
			ReadyPath:     r.readyPath,
			Role:          KvRoleConsumer,
			ReplicaIdx:    i,
		}
	}

	return DeploymentPlan{
		DeploymentID:    in.DeploymentID,
		Model:           model,
		Prefill:         prefills,
		Decode:          decodes,
		ContainerPrefix: "inferia-sglang",
	}, nil
}
