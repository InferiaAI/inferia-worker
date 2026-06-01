package recipes

import (
	"os/exec"
	"strconv"
	"strings"
)

// GPUProfile describes a detected GPU on the host.
type GPUProfile struct {
	Name      string
	MemoryMiB uint64
	SMVersion int
}

// sizing holds the per-GPU scaling parameters for a given GPU count.
type sizing struct {
	gmu float64
	mml int
	mns int
	mbt int
}

// ---------------------------------------------------------------------------
// nvidia-smi wrapper (injectable for tests — same pattern as telemetry/gpu.go)
// ---------------------------------------------------------------------------

type gpuSMIReader func() (string, error)

var readGPUInfo gpuSMIReader = runNvidiaSMI

func runNvidiaSMI() (string, error) {
	out, err := exec.Command(
		"nvidia-smi",
		"--query-gpu=name,memory.total",
		"--format=csv,noheader,nounits",
	).Output()
	return string(out), err
}

// detectGPUProfile returns the profile of GPU 0 (the first device).
// Returns a zero-value profile (SMVersion = 0) when no GPU is found.
func detectGPUProfile() GPUProfile {
	return detectGPUProfileFrom(readGPUInfo)
}

func detectGPUProfileFrom(read gpuSMIReader) GPUProfile {
	out, err := read()
	if err != nil {
		return GPUProfile{}
	}
	line, _, _ := strings.Cut(out, "\n")
	line = strings.TrimSpace(line)
	if line == "" {
		return GPUProfile{}
	}
	parts := strings.Split(line, ",")
	if len(parts) < 2 {
		return GPUProfile{}
	}
	name := strings.TrimSpace(parts[0])
	memRaw := strings.TrimSpace(parts[len(parts)-1])
	mem, _ := strconv.ParseUint(memRaw, 10, 64)
	sm := smForGPU(name)
	return GPUProfile{Name: name, MemoryMiB: mem, SMVersion: sm}
}

// ---------------------------------------------------------------------------
// SM lookup table
// ---------------------------------------------------------------------------

var smTable = []struct {
	prefix string
	sm     int
}{
	{"Tesla T4", 75},
	{"Tesla V100", 70},
	{"NVIDIA A10G", 86},
	{"NVIDIA L4", 89},
	{"NVIDIA L40S", 89},
	{"NVIDIA A100", 80},
	{"NVIDIA H100", 90},
	{"NVIDIA H200", 90},
	{"NVIDIA GH200", 90},
}

func smForGPU(name string) int {
	for _, e := range smTable {
		if strings.HasPrefix(name, e.prefix) {
			return e.sm
		}
	}
	return 0
}

// ---------------------------------------------------------------------------
// GPU profile table — per-GPU family default config + sizing
// ---------------------------------------------------------------------------

// profile holds the SM-dependent config defaults.
type profile struct {
	dtype   string
	kvDtype string
	quant   string // "" means no default
	eager   bool
	env     map[string]string
}

// gpuProfiles maps GPU name prefix → SM-dependent config.
var gpuProfiles = map[string]profile{
	"Tesla T4":   {dtype: "float16", kvDtype: "int8", eager: true, env: map[string]string{"VLLM_ATTENTION_BACKEND": "XFORMERS"}},
	"Tesla V100": {dtype: "float16", kvDtype: "int8", eager: true, env: map[string]string{"VLLM_ATTENTION_BACKEND": "XFORMERS"}},
	"NVIDIA A10G": {dtype: "bfloat16", kvDtype: "auto"},
	"NVIDIA L4":   {dtype: "bfloat16", kvDtype: "auto", quant: "fp8"},
	"NVIDIA L40S": {dtype: "bfloat16", kvDtype: "auto", quant: "fp8"},
	"NVIDIA A100": {dtype: "bfloat16", kvDtype: "auto"},
	"NVIDIA H100": {dtype: "bfloat16", kvDtype: "fp8", quant: "fp8"},
	"NVIDIA H200": {dtype: "bfloat16", kvDtype: "fp8", quant: "fp8"},
	"NVIDIA GH200": {dtype: "bfloat16", kvDtype: "fp8", quant: "fp8"},
}

// sizingTable maps (GPU prefix, numGPUs) → sizing. Key "prefix" alone is the
// single-GPU default. Key "prefix:N" overrides for exactly N GPUs.
var sizingTable = map[string]sizing{
	// --- single GPU ---
	"Tesla T4":              {gmu: 0.92, mml: 4096, mns: 16, mbt: 4096},
	"Tesla V100":            {gmu: 0.90, mml: 3072, mns: 12, mbt: 3072},
	"NVIDIA A10G":           {gmu: 0.90, mml: 8192, mns: 32, mbt: 8192},
	"NVIDIA L4":             {gmu: 0.92, mml: 12288, mns: 48, mbt: 12288},
	"NVIDIA L40S":           {gmu: 0.90, mml: 16384, mns: 64, mbt: 16384},
	"NVIDIA A100":           {gmu: 0.92, mml: 8192, mns: 32, mbt: 8192},
	"NVIDIA H100":           {gmu: 0.95, mml: 65536, mns: 512, mbt: 65536},
	"NVIDIA H200":           {gmu: 0.93, mml: 65536, mns: 512, mbt: 65536},
	"NVIDIA GH200":          {gmu: 0.93, mml: 65536, mns: 512, mbt: 65536},

	// --- multi-GPU overrides (key = "prefix:N") ---
	"NVIDIA A10G:4":         {gmu: 0.90, mml: 8192, mns: 64, mbt: 16384},
	"NVIDIA A10G:8":         {gmu: 0.88, mml: 16384, mns: 128, mbt: 32768},
	"NVIDIA L4:4":           {gmu: 0.92, mml: 12288, mns: 64, mbt: 16384},
	"NVIDIA A100:8":         {gmu: 0.92, mml: 16384, mns: 128, mbt: 16384},
	"NVIDIA H100:8":         {gmu: 0.95, mml: 65536, mns: 512, mbt: 65536},
	"NVIDIA H200:8":         {gmu: 0.93, mml: 65536, mns: 512, mbt: 65536},
}

// A100 VRAM-aware sizing overrides (keyed on "prefix:numGPUs:vramMiB").
// Prefix must match the full gpuProfiles key, e.g. "NVIDIA A100".
// A100-PCIe-40GB vs A100-SXM4-80GB have very different capacities.
var sizingByVRAM = map[string]sizing{
	"NVIDIA A100:8:40960": {gmu: 0.92, mml: 16384, mns: 128, mbt: 16384},
	"NVIDIA A100:8:81920": {gmu: 0.92, mml: 32768, mns: 256, mbt: 32768},
	"NVIDIA A100:1:40960": {gmu: 0.92, mml: 16384, mns: 64, mbt: 16384},
	"NVIDIA A100:1:81920": {gmu: 0.92, mml: 32768, mns: 128, mbt: 32768},
}

// ---------------------------------------------------------------------------
// Public API
// ---------------------------------------------------------------------------

// gpuOptimalConfig returns the recommended vLLM config and env vars for the
// given GPU profile and GPU count. The caller is expected to layer user config
// overrides on top.
func gpuOptimalConfig(gpu GPUProfile, numGPUs int) (cfg map[string]any, env map[string]string) {
	cfg = map[string]any{}
	env = map[string]string{"VLLM_USE_FASTOKENS": "1"}

	// 1. SM-dependent config from profile table
	if p, ok := lookupProfile(gpu.Name); ok {
		if p.dtype != "" {
			cfg["dtype"] = p.dtype
		}
		if p.kvDtype != "" {
			cfg["kv_cache_dtype"] = p.kvDtype
		}
		if p.quant != "" {
			cfg["quantization"] = p.quant
		}
		if p.eager {
			cfg["enforce_eager"] = true
		}
		for k, v := range p.env {
			env[k] = v
		}
	}

	// 2. Sizing
	s := lookupSizing(gpu, numGPUs)
	cfg["gpu_memory_utilization"] = s.gmu
	cfg["max_model_len"] = s.mml
	cfg["max_num_seqs"] = s.mns
	cfg["max_num_batched_tokens"] = s.mbt

	// 3. Always-on
	cfg["enable_prefix_caching"] = true

	// 4. Multi-GPU
	if numGPUs > 1 {
		cfg["tensor_parallel_size"] = numGPUs
	}

	return cfg, env
}

// ---------------------------------------------------------------------------
// internal helpers
// ---------------------------------------------------------------------------

func lookupProfile(name string) (profile, bool) {
	for _, entry := range sortedProfileKeys {
		if strings.HasPrefix(name, entry) {
			p, ok := gpuProfiles[entry]
			return p, ok
		}
	}
	return profile{}, false
}

func lookupSizing(gpu GPUProfile, numGPUs int) sizing {
	name := gpu.Name
	vram := gpu.MemoryMiB

	// 1. Find the matching profile prefix.
	prefix := ""
	for _, p := range sortedProfileKeys {
		if strings.HasPrefix(name, p) {
			prefix = p
			break
		}
	}
	if prefix == "" {
		return sizing{gmu: 0.90, mml: 4096, mns: 32, mbt: 8192}
	}

	// 2. VRAM-specific match by prefix (e.g. A100:1:40960).
	if vram > 0 {
		key := prefix + ":" + strconv.Itoa(numGPUs) + ":" + strconv.FormatUint(vram, 10)
		if s, ok := sizingByVRAM[key]; ok {
			return s
		}
	}

	// 3. Multi-GPU override (e.g. A10G:4).
	if numGPUs > 1 {
		key := prefix + ":" + strconv.Itoa(numGPUs)
		if s, ok := sizingTable[key]; ok {
			return s
		}
	}

	// 4. Single-GPU default.
	if s, ok := sizingTable[prefix]; ok {
		return s
	}

	return sizing{gmu: 0.90, mml: 4096, mns: 32, mbt: 8192}
}

// sortedProfileKeys is the profile prefix list ordered longest-first so
// "NVIDIA A100" doesn't match before "NVIDIA A10G".
var sortedProfileKeys []string

func init() {
	sortedProfileKeys = make([]string, 0, len(gpuProfiles))
	for k := range gpuProfiles {
		sortedProfileKeys = append(sortedProfileKeys, k)
	}
	// Sort descending by length so longer prefixes (more specific) match first.
	for i := 0; i < len(sortedProfileKeys); i++ {
		for j := i + 1; j < len(sortedProfileKeys); j++ {
			if len(sortedProfileKeys[j]) > len(sortedProfileKeys[i]) {
				sortedProfileKeys[i], sortedProfileKeys[j] = sortedProfileKeys[j], sortedProfileKeys[i]
			}
		}
	}
}
