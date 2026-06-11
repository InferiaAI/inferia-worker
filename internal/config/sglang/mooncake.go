package sglang

import "os"

func MooncakeEnabled() bool {
	return os.Getenv("MOONCAKE_KV_ENABLED") == "true"
}

func ApplySGLangMooncakePrefillFlags(cfg map[string]any, env map[string]string, cmd *[]string) {
	cfg["enable_prefix_caching"] = true
	env["PYTHONHASHSEED"] = "0"
	*cmd = append(*cmd, "--disaggregation-mode", "prefill")

	if dev := os.Getenv("SGLANG_DISAGG_IB_DEVICE"); dev != "" {
		*cmd = append(*cmd, "--disaggregation-ib-device", dev)
	}
	forwardEnvVars(env, []string{
		"SGLANG_MOONCAKE_CUSTOM_MEM_POOL",
		"MC_INTRANODE_NVLINK",
		"MC_FORCE_MNNVL",
		"SGLANG_DISAGGREGATION_THREAD_POOL_SIZE",
		"SGLANG_DISAGGREGATION_QUEUE_SIZE",
		"SGLANG_DISAGGREGATION_BOOTSTRAP_TIMEOUT",
	})
}

func ApplySGLangMooncakeDecodeFlags(cfg map[string]any, env map[string]string, cmd *[]string) {
	cfg["enable_prefix_caching"] = true
	env["PYTHONHASHSEED"] = "0"
	*cmd = append(*cmd, "--disaggregation-mode", "decode")

	if dev := os.Getenv("SGLANG_DISAGG_IB_DEVICE"); dev != "" {
		*cmd = append(*cmd, "--disaggregation-ib-device", dev)
	}
	forwardEnvVars(env, []string{
		"SGLANG_MOONCAKE_CUSTOM_MEM_POOL",
		"MC_INTRANODE_NVLINK",
		"MC_FORCE_MNNVL",
		"SGLANG_DISAGGREGATION_HEARTBEAT_INTERVAL",
		"SGLANG_DISAGGREGATION_HEARTBEAT_MAX_FAILURE",
		"SGLANG_DISAGGREGATION_WAITING_TIMEOUT",
	})
}

func MooncakeEntrypoint() []string {
	return []string{
		"/bin/sh", "-c",
		"pip install --no-cache-dir mooncake-transfer-engine && exec \"$@\"",
		"entrypoint",
	}
}

func forwardEnvVars(dst map[string]string, keys []string) {
	for _, k := range keys {
		if v := os.Getenv(k); v != "" {
			dst[k] = v
		}
	}
}
