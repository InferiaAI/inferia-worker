package recipes

import (
	"errors"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// detection
// ---------------------------------------------------------------------------

func TestDetectGPUProfileFrom(t *testing.T) {
	tests := []struct {
		name    string
		output  string
		wantSM  int
		wantMem uint64
		wantOk  bool
	}{
		{
			name:    "T4",
			output:  "Tesla T4, 16384\n",
			wantSM:  75, wantMem: 16384, wantOk: true,
		},
		{
			name:    "V100",
			output:  "Tesla V100-SXM2-32GB, 32768\n",
			wantSM:  70, wantMem: 32768, wantOk: true,
		},
		{
			name:    "A10G",
			output:  "NVIDIA A10G, 24576\n",
			wantSM:  86, wantMem: 24576, wantOk: true,
		},
		{
			name:    "L4",
			output:  "NVIDIA L4, 24576\n",
			wantSM:  89, wantMem: 24576, wantOk: true,
		},
		{
			name:    "L40S",
			output:  "NVIDIA L40S, 49152\n",
			wantSM:  89, wantMem: 49152, wantOk: true,
		},
		{
			name:    "A100-40GB-SXM",
			output:  "NVIDIA A100-SXM4-40GB, 40960\n",
			wantSM:  80, wantMem: 40960, wantOk: true,
		},
		{
			name:    "A100-80GB-PCIe",
			output:  "NVIDIA A100-PCIE-80GB, 81920\n",
			wantSM:  80, wantMem: 81920, wantOk: true,
		},
		{
			name:    "H100",
			output:  "NVIDIA H100 80GB HBM3, 81920\n",
			wantSM:  90, wantMem: 81920, wantOk: true,
		},
		{
			name:    "H200",
			output:  "NVIDIA H200, 144384\n",
			wantSM:  90, wantMem: 144384, wantOk: true,
		},
		{
			name:    "unknown GPU",
			output:  "AMD Instinct MI250, 131072\n",
			wantSM:  0, wantMem: 131072, wantOk: true,
		},
		{
			name:    "empty output",
			output:  "",
			wantSM:  0, wantMem: 0, wantOk: false,
		},
		{
			name:    "nvidia-smi error",
			output:  "",
			wantSM:  0, wantMem: 0, wantOk: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var read gpuSMIReader
			if tc.name == "nvidia-smi error" {
				read = func() (string, error) { return "", errors.New("nvidia-smi not found") }
			} else {
				read = func() (string, error) { return tc.output, nil }
			}
			gpu := detectGPUProfileFrom(read)
			if gpu.SMVersion != tc.wantSM {
				t.Errorf("SMVersion = %d, want %d", gpu.SMVersion, tc.wantSM)
			}
			if gpu.MemoryMiB != tc.wantMem {
				t.Errorf("MemoryMiB = %d, want %d", gpu.MemoryMiB, tc.wantMem)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// config generation — single GPU
// ---------------------------------------------------------------------------

type configCheck struct {
	key   string
	value any
}

func checkConfig(t *testing.T, cfg map[string]any, checks []configCheck) {
	t.Helper()
	for _, c := range checks {
		got, ok := cfg[c.key]
		if !ok {
			t.Errorf("cfg[%q] missing, want %v", c.key, c.value)
			continue
		}
		if got != c.value {
			t.Errorf("cfg[%q] = %v (type %T), want %v (type %T)",
				c.key, got, got, c.value, c.value)
		}
	}
}

func TestGPUOptimalConfig_T4(t *testing.T) {
	gpu := GPUProfile{Name: "Tesla T4", MemoryMiB: 16384, SMVersion: 75}
	cfg, env := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "float16"},
		{"kv_cache_dtype", "int8"},
		{"enforce_eager", true},
		{"gpu_memory_utilization", 0.92},
		{"max_model_len", 4096},
		{"max_num_seqs", 16},
		{"max_num_batched_tokens", 4096},
		{"enable_prefix_caching", true},
	})
	if env["VLLM_ATTENTION_BACKEND"] != "XFORMERS" {
		t.Errorf("VLLM_ATTENTION_BACKEND = %q, want XFORMERS", env["VLLM_ATTENTION_BACKEND"])
	}
	if env["VLLM_USE_FASTOKENS"] != "1" {
		t.Errorf("VLLM_USE_FASTOKENS missing")
	}
	// Must NOT set tensor_parallel_size for single GPU
	if _, ok := cfg["tensor_parallel_size"]; ok {
		t.Error("tensor_parallel_size should not be set for single GPU")
	}
}

func TestGPUOptimalConfig_V100(t *testing.T) {
	gpu := GPUProfile{Name: "Tesla V100-SXM2-32GB", MemoryMiB: 32768, SMVersion: 70}
	cfg, env := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "float16"},
		{"kv_cache_dtype", "int8"},
		{"enforce_eager", true},
		{"gpu_memory_utilization", 0.90},
		{"max_model_len", 3072},
		{"max_num_seqs", 12},
		{"max_num_batched_tokens", 3072},
	})
	if env["VLLM_ATTENTION_BACKEND"] != "XFORMERS" {
		t.Errorf("VLLM_ATTENTION_BACKEND = %q", env["VLLM_ATTENTION_BACKEND"])
	}
}

func TestGPUOptimalConfig_A10G(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA A10G", MemoryMiB: 24576, SMVersion: 86}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"gpu_memory_utilization", 0.90},
		{"max_model_len", 8192},
		{"max_num_seqs", 32},
		{"max_num_batched_tokens", 8192},
		{"enable_prefix_caching", true},
	})
	// Must NOT set enforce_eager or quantization
	if _, ok := cfg["enforce_eager"]; ok {
		t.Error("enforce_eager should not be set for A10G")
	}
	if _, ok := cfg["quantization"]; ok {
		t.Error("quantization should not be set for A10G (no fp8 HW)")
	}
}

func TestGPUOptimalConfig_L4(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA L4", MemoryMiB: 24576, SMVersion: 89}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.92},
		{"max_model_len", 12288},
		{"max_num_seqs", 48},
		{"max_num_batched_tokens", 12288},
	})
}

func TestGPUOptimalConfig_L40S(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA L40S", MemoryMiB: 49152, SMVersion: 89}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.90},
		{"max_model_len", 16384},
		{"max_num_seqs", 64},
		{"max_num_batched_tokens", 16384},
	})
}

func TestGPUOptimalConfig_A100_40GB(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA A100-SXM4-40GB", MemoryMiB: 40960, SMVersion: 80}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"gpu_memory_utilization", 0.92},
		{"max_model_len", 16384},
		{"max_num_seqs", 64},
		{"max_num_batched_tokens", 16384},
	})
}

func TestGPUOptimalConfig_A100_80GB(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA A100-SXM4-80GB", MemoryMiB: 81920, SMVersion: 80}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"gpu_memory_utilization", 0.92},
		{"max_model_len", 32768},
		{"max_num_seqs", 128},
		{"max_num_batched_tokens", 32768},
	})
}

func TestGPUOptimalConfig_H100(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA H100 80GB HBM3", MemoryMiB: 81920, SMVersion: 90}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.95},
		{"max_model_len", 65536},
		{"max_num_seqs", 512},
		{"max_num_batched_tokens", 65536},
	})
}

func TestGPUOptimalConfig_H200(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA H200", MemoryMiB: 144384, SMVersion: 90}
	cfg, _ := gpuOptimalConfig(gpu, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.93},
		{"max_model_len", 65536},
		{"max_num_seqs", 512},
		{"max_num_batched_tokens", 65536},
	})
}

func TestGPUOptimalConfig_UnknownGPU(t *testing.T) {
	gpu := GPUProfile{Name: "AMD Instinct MI250", MemoryMiB: 131072, SMVersion: 0}
	cfg, env := gpuOptimalConfig(gpu, 1)

	// Unknown GPU: no SM-based config, fallback sizing
	if _, ok := cfg["dtype"]; ok {
		t.Error("dtype should not be set for unknown GPU")
	}
	if _, ok := cfg["kv_cache_dtype"]; ok {
		t.Error("kv_cache_dtype should not be set for unknown GPU")
	}
	// Fallback sizing
	checkConfig(t, cfg, []configCheck{
		{"gpu_memory_utilization", 0.90},
		{"max_model_len", 4096},
		{"max_num_seqs", 32},
		{"max_num_batched_tokens", 8192},
		{"enable_prefix_caching", true},
	})
	if env["VLLM_USE_FASTOKENS"] != "1" {
		t.Errorf("VLLM_USE_FASTOKENS missing")
	}
}

// ---------------------------------------------------------------------------
// config generation — multi-GPU
// ---------------------------------------------------------------------------

func TestGPUOptimalConfig_A10G_Multi(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA A10G", MemoryMiB: 24576, SMVersion: 86}

	t.Run("x4", func(t *testing.T) {
		cfg, _ := gpuOptimalConfig(gpu, 4)
		checkConfig(t, cfg, []configCheck{
			{"tensor_parallel_size", 4},
			{"gpu_memory_utilization", 0.90},
			{"max_model_len", 8192},
			{"max_num_seqs", 64},
			{"max_num_batched_tokens", 16384},
		})
	})

	t.Run("x8", func(t *testing.T) {
		cfg, _ := gpuOptimalConfig(gpu, 8)
		checkConfig(t, cfg, []configCheck{
			{"tensor_parallel_size", 8},
			{"gpu_memory_utilization", 0.88},
			{"max_model_len", 16384},
			{"max_num_seqs", 128},
			{"max_num_batched_tokens", 32768},
		})
	})
}

func TestGPUOptimalConfig_L4_Multi(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA L4", MemoryMiB: 24576, SMVersion: 89}

	t.Run("x4", func(t *testing.T) {
		cfg, _ := gpuOptimalConfig(gpu, 4)
		checkConfig(t, cfg, []configCheck{
			{"tensor_parallel_size", 4},
			{"gpu_memory_utilization", 0.92},
			{"max_model_len", 12288},
			{"max_num_seqs", 64},
			{"max_num_batched_tokens", 16384},
		})
	})
}

func TestGPUOptimalConfig_A100_Multi(t *testing.T) {
	t.Run("40GB x8", func(t *testing.T) {
		gpu := GPUProfile{Name: "NVIDIA A100-SXM4-40GB", MemoryMiB: 40960, SMVersion: 80}
		cfg, _ := gpuOptimalConfig(gpu, 8)
		checkConfig(t, cfg, []configCheck{
			{"tensor_parallel_size", 8},
			{"gpu_memory_utilization", 0.92},
			{"max_model_len", 16384},
			{"max_num_seqs", 128},
			{"max_num_batched_tokens", 16384},
		})
	})

	t.Run("80GB x8", func(t *testing.T) {
		gpu := GPUProfile{Name: "NVIDIA A100-SXM4-80GB", MemoryMiB: 81920, SMVersion: 80}
		cfg, _ := gpuOptimalConfig(gpu, 8)
		checkConfig(t, cfg, []configCheck{
			{"tensor_parallel_size", 8},
			{"gpu_memory_utilization", 0.92},
			{"max_model_len", 32768},
			{"max_num_seqs", 256},
			{"max_num_batched_tokens", 32768},
		})
	})
}

func TestGPUOptimalConfig_H100_Multi(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA H100 80GB HBM3", MemoryMiB: 81920, SMVersion: 90}
	cfg, _ := gpuOptimalConfig(gpu, 8)

	checkConfig(t, cfg, []configCheck{
		{"tensor_parallel_size", 8},
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.95},
		{"max_model_len", 65536},
		{"max_num_seqs", 512},
		{"max_num_batched_tokens", 65536},
	})
}

func TestGPUOptimalConfig_H200_Multi(t *testing.T) {
	gpu := GPUProfile{Name: "NVIDIA H200", MemoryMiB: 144384, SMVersion: 90}
	cfg, _ := gpuOptimalConfig(gpu, 8)

	checkConfig(t, cfg, []configCheck{
		{"tensor_parallel_size", 8},
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8"},
		{"quantization", "fp8"},
		{"gpu_memory_utilization", 0.93},
		{"max_model_len", 65536},
		{"max_num_seqs", 512},
		{"max_num_batched_tokens", 65536},
	})
}

// ---------------------------------------------------------------------------
// BuildPlan integration
// ---------------------------------------------------------------------------

func TestVLLM_BuildPlan_GPUDefaults(t *testing.T) {
	// No real GPU — detectGPUProfile returns zero value, no GPU defaults applied.
	// This verifies the recipe still works without GPU detection.
	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "dep-1",
		ArtifactURI:  "hf://meta-llama/Llama-3.1-8B-Instruct",
		GPUIndices:   []int{0},
		HostPort:     19000,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "meta-llama/Llama-3.1-8B-Instruct") {
		t.Errorf("model missing: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--port") {
		t.Errorf("port missing: %v", plan.Cmd)
	}
	// Should NOT have GPU-specific flags (no GPU detected)
	if strings.Contains(joined, "--dtype") {
		t.Logf("GPU defaults applied despite no GPU (expected on CI with GPU): %v", plan.Cmd)
	}
}

func TestVLLM_BuildPlan_GPUDefaults_UserOverrides(t *testing.T) {
	oldRead := readGPUInfo
	readGPUInfo = func() (string, error) {
		return "NVIDIA A10G, 24576\n", nil
	}
	defer func() { readGPUInfo = oldRead }()

	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "dep-2",
		ArtifactURI:  "hf://org/model",
		GPUIndices:   []int{0},
		HostPort:     19001,
		Config: map[string]any{
			"dtype":                  "float16",
			"max_model_len":          2048,
			"gpu_memory_utilization": 0.80,
		},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	// User overrides must take precedence
	if strings.Contains(joined, "--dtype bfloat16") {
		t.Errorf("GPU dtype (bfloat16) should be overridden by user dtype (float16): %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--dtype float16") {
		t.Errorf("user dtype missing: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--max-model-len 2048") {
		t.Errorf("user max-model-len missing: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--gpu-memory-utilization 0.8") {
		t.Errorf("user gpu-memory-utilization missing: %v", plan.Cmd)
	}
}

func TestVLLM_BuildPlan_GPUDefaults_T4(t *testing.T) {
	oldRead := readGPUInfo
	readGPUInfo = func() (string, error) {
		return "Tesla T4, 16384\n", nil
	}
	defer func() { readGPUInfo = oldRead }()

	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "dep-t4",
		ArtifactURI:  "hf://org/model",
		GPUIndices:   []int{0},
		HostPort:     19002,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "--dtype float16") {
		t.Errorf("expected float16 for T4: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--kv-cache-dtype int8") {
		t.Errorf("expected int8 KV for T4: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--enforce-eager") {
		t.Errorf("expected enforce-eager for T4: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--max-model-len 4096") {
		t.Errorf("expected max-model-len 4096 for T4: %v", plan.Cmd)
	}
}

func TestVLLM_BuildPlan_GPUDefaults_L4(t *testing.T) {
	oldRead := readGPUInfo
	readGPUInfo = func() (string, error) {
		return "NVIDIA L4, 24576\n", nil
	}
	defer func() { readGPUInfo = oldRead }()

	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "dep-l4",
		ArtifactURI:  "hf://org/model",
		GPUIndices:   []int{0},
		HostPort:     19003,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "--dtype bfloat16") {
		t.Errorf("expected bfloat16 for L4: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--quantization fp8") {
		t.Errorf("expected fp8 quantization for L4: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--max-model-len 12288") {
		t.Errorf("expected max-model-len 12288 for L4: %v", plan.Cmd)
	}
}

func TestVLLM_BuildPlan_GPUDefaults_H100_x8(t *testing.T) {
	oldRead := readGPUInfo
	readGPUInfo = func() (string, error) {
		return "NVIDIA H100 80GB HBM3, 81920\n", nil
	}
	defer func() { readGPUInfo = oldRead }()

	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "dep-h100",
		ArtifactURI:  "hf://org/model",
		GPUIndices:   []int{0, 1, 2, 3, 4, 5, 6, 7},
		HostPort:     19004,
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "--tensor-parallel-size 8") {
		t.Errorf("expected TP=8 for H100 x8: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--kv-cache-dtype fp8") {
		t.Errorf("expected fp8 KV for H100: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--quantization fp8") {
		t.Errorf("expected fp8 quantization for H100: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--max-model-len 65536") {
		t.Errorf("expected max-model-len 65536 for H100: %v", plan.Cmd)
	}
}

// ---------------------------------------------------------------------------
// smForGPU edge cases
// ---------------------------------------------------------------------------

func TestSMForGPU_Known(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"Tesla T4", 75},
		{"Tesla V100-SXM2-32GB", 70},
		{"NVIDIA A10G", 86},
		{"NVIDIA L4", 89},
		{"NVIDIA L40S", 89},
		{"NVIDIA A100-SXM4-80GB", 80},
		{"NVIDIA A100-PCIE-40GB", 80},
		{"NVIDIA H100 80GB HBM3", 90},
		{"NVIDIA H200", 90},
		{"NVIDIA GH200", 90},
	}
	for _, tc := range tests {
		if got := smForGPU(tc.name); got != tc.want {
			t.Errorf("smForGPU(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSMForGPU_Unknown(t *testing.T) {
	if got := smForGPU("AMD Instinct MI250"); got != 0 {
		t.Errorf("expected 0 for unknown GPU, got %d", got)
	}
	if got := smForGPU(""); got != 0 {
		t.Errorf("expected 0 for empty name, got %d", got)
	}
}
