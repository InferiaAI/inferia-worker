package sglang_test

import (
	"testing"

	"github.com/inferia/inferia-worker/internal/config/sglang"
)

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

func checkNotSet(t *testing.T, cfg map[string]any, keys ...string) {
	t.Helper()
	for _, k := range keys {
		if _, ok := cfg[k]; ok {
			t.Errorf("cfg[%q] should not be set, got %v", k, cfg[k])
		}
	}
}

func TestSMForGPU_Known(t *testing.T) {
	tests := []struct {
		name string
		want int
	}{
		{"Tesla T4", 75},
		{"Tesla V100-SXM2-32GB", 70},
		{"NVIDIA A10G", 86},
		{"NVIDIA L4", 89},
		{"NVIDIA A100-SXM4-80GB", 80},
		{"NVIDIA H100 80GB HBM3", 90},
		{"NVIDIA H200", 90},
		{"NVIDIA GH200", 90},
	}
	for _, tc := range tests {
		if got := sglang.SMForGPU(tc.name); got != tc.want {
			t.Errorf("SMForGPU(%q) = %d, want %d", tc.name, got, tc.want)
		}
	}
}

func TestSMForGPU_Unknown(t *testing.T) {
	if got := sglang.SMForGPU("AMD Instinct MI250"); got != 0 {
		t.Errorf("expected 0 for unknown GPU, got %d", got)
	}
}

func TestGPUOptimalConfig_T4(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("Tesla T4", 16384, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "float16"},
		{"kv_cache_dtype", "auto"},
		{"attention_backend", "triton"},
		{"sampling_backend", "pytorch"},
		{"mem_fraction_static", 0.88},
		{"chunked_prefill_size", 4096},
		{"enable_prefix_caching", true},
	})
	checkNotSet(t, cfg, "quantization", "tensor_parallel_size")
}

func TestGPUOptimalConfig_V100(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("Tesla V100-SXM2-32GB", 32768, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "float16"},
		{"kv_cache_dtype", "auto"},
		{"attention_backend", "triton"},
		{"sampling_backend", "pytorch"},
		{"mem_fraction_static", 0.88},
		{"chunked_prefill_size", 3072},
	})
}

func TestGPUOptimalConfig_A10G(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA A10G", 24576, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "auto"},
		{"mem_fraction_static", 0.90},
		{"chunked_prefill_size", 8192},
		{"enable_prefix_caching", true},
	})
	checkNotSet(t, cfg, "attention_backend", "sampling_backend", "quantization")
}

func TestGPUOptimalConfig_L4(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA L4", 24576, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8_e4m3"},
		{"quantization", "fp8"},
		{"mem_fraction_static", 0.92},
		{"chunked_prefill_size", 8192},
	})
}

func TestGPUOptimalConfig_A100_40GB(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA A100-SXM4-40GB", 40960, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8_e4m3"},
		{"mem_fraction_static", 0.88},
		{"chunked_prefill_size", 8192},
	})
}

func TestGPUOptimalConfig_A100_80GB(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA A100-SXM4-80GB", 81920, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8_e4m3"},
		{"mem_fraction_static", 0.90},
		{"chunked_prefill_size", 8192},
	})
}

func TestGPUOptimalConfig_H100(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA H100 80GB HBM3", 81920, 1)

	checkConfig(t, cfg, []configCheck{
		{"dtype", "bfloat16"},
		{"kv_cache_dtype", "fp8_e4m3"},
		{"quantization", "fp8"},
		{"mem_fraction_static", 0.92},
		{"chunked_prefill_size", 8192},
	})
}

func TestGPUOptimalConfig_UnknownGPU(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("AMD Instinct MI250", 131072, 1)

	checkNotSet(t, cfg, "dtype", "kv_cache_dtype")
	checkConfig(t, cfg, []configCheck{
		{"mem_fraction_static", 0.90},
		{"chunked_prefill_size", 8192},
		{"enable_prefix_caching", true},
	})
}

func TestGPUOptimalConfig_EmptyName(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("", 0, 1)

	checkConfig(t, cfg, []configCheck{
		{"mem_fraction_static", 0.90},
		{"chunked_prefill_size", 8192},
		{"enable_prefix_caching", true},
	})
	checkNotSet(t, cfg, "dtype", "kv_cache_dtype", "quantization", "tensor_parallel_size")
}

func TestGPUOptimalConfig_MultiGPU(t *testing.T) {
	cfg, _ := sglang.GPUOptimalConfig("NVIDIA A10G", 24576, 4)
	checkConfig(t, cfg, []configCheck{
		{"tensor_parallel_size", 4},
		{"dtype", "bfloat16"},
		{"mem_fraction_static", 0.90},
	})
}
