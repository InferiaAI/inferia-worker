package sglang

import (
	"sort"
	"strconv"
	"strings"
)

type GPUProfile struct {
	Name      string
	MemoryMiB uint64
	SMVersion int
}

type sglangProfile struct {
	dtype            string
	kvDtype          string
	attentionBackend string
	samplingBackend  string
	quant            string
}

type sizing struct {
	memFraction    float64
	chunkedPrefill int
}

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

func SMForGPU(name string) int {
	for _, e := range smTable {
		if strings.HasPrefix(name, e.prefix) {
			return e.sm
		}
	}
	return 0
}

var gpuProfiles = map[string]sglangProfile{
	"Tesla T4":      {dtype: "float16", kvDtype: "auto", attentionBackend: "triton", samplingBackend: "pytorch"},
	"Tesla V100":    {dtype: "float16", kvDtype: "auto", attentionBackend: "triton", samplingBackend: "pytorch"},
	"NVIDIA A10G":   {dtype: "bfloat16", kvDtype: "auto"},
	"NVIDIA L4":     {dtype: "bfloat16", kvDtype: "fp8_e4m3", quant: "fp8"},
	"NVIDIA L40S":   {dtype: "bfloat16", kvDtype: "fp8_e4m3", quant: "fp8"},
	"NVIDIA A100":   {dtype: "bfloat16", kvDtype: "fp8_e4m3"},
	"NVIDIA H100":   {dtype: "bfloat16", kvDtype: "fp8_e4m3", quant: "fp8"},
	"NVIDIA H200":   {dtype: "bfloat16", kvDtype: "fp8_e4m3", quant: "fp8"},
	"NVIDIA GH200":  {dtype: "bfloat16", kvDtype: "fp8_e4m3", quant: "fp8"},
}

var sizingTable = map[string]sizing{
	"Tesla T4":              {memFraction: 0.88, chunkedPrefill: 4096},
	"Tesla V100":            {memFraction: 0.88, chunkedPrefill: 3072},
	"NVIDIA A10G":           {memFraction: 0.90, chunkedPrefill: 8192},
	"NVIDIA L4":             {memFraction: 0.92, chunkedPrefill: 8192},
	"NVIDIA L40S":           {memFraction: 0.90, chunkedPrefill: 8192},
	"NVIDIA A100":           {memFraction: 0.90, chunkedPrefill: 8192},
	"NVIDIA H100":           {memFraction: 0.92, chunkedPrefill: 8192},
	"NVIDIA H200":           {memFraction: 0.93, chunkedPrefill: 8192},
	"NVIDIA GH200":          {memFraction: 0.93, chunkedPrefill: 8192},
}

var sizingByVRAM = map[string]sizing{
	"NVIDIA A100:1:40960": {memFraction: 0.88, chunkedPrefill: 8192},
	"NVIDIA A100:1:81920": {memFraction: 0.90, chunkedPrefill: 8192},
}

func GPUOptimalConfig(name string, memoryMiB uint64, numGPUs int) (cfg map[string]any, env map[string]string) {
	cfg = map[string]any{}
	env = map[string]string{}

	if name == "" {
		cfg["mem_fraction_static"] = 0.90
		cfg["chunked_prefill_size"] = 8192
		cfg["enable_prefix_caching"] = true
		return cfg, env
	}

	if p, ok := lookupProfile(name); ok {
		if p.dtype != "" {
			cfg["dtype"] = p.dtype
		}
		if p.kvDtype != "" {
			cfg["kv_cache_dtype"] = p.kvDtype
		}
		if p.attentionBackend != "" {
			cfg["attention_backend"] = p.attentionBackend
		}
		if p.samplingBackend != "" {
			cfg["sampling_backend"] = p.samplingBackend
		}
		if p.quant != "" {
			cfg["quantization"] = p.quant
		}
	}

	gpu := GPUProfile{Name: name, MemoryMiB: memoryMiB, SMVersion: SMForGPU(name)}
	s := lookupSizing(gpu, numGPUs)
	cfg["mem_fraction_static"] = s.memFraction
	cfg["chunked_prefill_size"] = s.chunkedPrefill

	cfg["enable_prefix_caching"] = true

	if numGPUs > 1 {
		cfg["tensor_parallel_size"] = numGPUs
	}

	return cfg, env
}

func lookupProfile(name string) (sglangProfile, bool) {
	for _, entry := range sortedProfileKeys {
		if strings.HasPrefix(name, entry) {
			p, ok := gpuProfiles[entry]
			return p, ok
		}
	}
	return sglangProfile{}, false
}

func lookupSizing(gpu GPUProfile, numGPUs int) sizing {
	name := gpu.Name
	vram := gpu.MemoryMiB

	prefix := ""
	for _, p := range sortedProfileKeys {
		if strings.HasPrefix(name, p) {
			prefix = p
			break
		}
	}
	if prefix == "" {
		return sizing{memFraction: 0.90, chunkedPrefill: 8192}
	}

	if vram > 0 {
		key := prefix + ":" + strconv.Itoa(numGPUs) + ":" + strconv.FormatUint(vram, 10)
		if s, ok := sizingByVRAM[key]; ok {
			return s
		}
	}

	if s, ok := sizingTable[prefix]; ok {
		return s
	}

	return sizing{memFraction: 0.90, chunkedPrefill: 8192}
}

var sortedProfileKeys []string

func init() {
	sortedProfileKeys = make([]string, 0, len(gpuProfiles))
	for k := range gpuProfiles {
		sortedProfileKeys = append(sortedProfileKeys, k)
	}
	sort.Slice(sortedProfileKeys, func(i, j int) bool {
		return len(sortedProfileKeys[j]) < len(sortedProfileKeys[i])
	})
}
