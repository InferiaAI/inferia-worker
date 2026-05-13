package recipes

import (
	"strings"
	"testing"
)

func TestCLIArg_AllScalarTypes(t *testing.T) {
	cases := []struct {
		in   any
		want string
	}{
		{"abc", "abc"},
		{true, "true"},
		{false, "false"},
		{42, "42"},
		{int32(7), "7"},
		{int64(99), "99"},
		{float32(0.5), "0.5"},
		{float64(0.9), "0.9"},
		{struct{ N int }{N: 1}, "{1}"}, // fallback
	}
	for _, tc := range cases {
		if got := cliArg(tc.in); got != tc.want {
			t.Errorf("cliArg(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestStripScheme_NoSchemePassthrough(t *testing.T) {
	if got := stripScheme("no-scheme"); got != "no-scheme" {
		t.Errorf("got %q", got)
	}
	if got := stripScheme("hf://org/m"); got != "org/m" {
		t.Errorf("got %q", got)
	}
}

func TestVLLM_EnforceEagerAndTrustRemoteCode(t *testing.T) {
	r, _ := Get("vllm")
	plan, err := r.BuildPlan(BuildInput{
		DeploymentID: "d",
		ArtifactURI:  "hf://o/m",
		GPUIndices:   []int{0},
		HostPort:     1234,
		Config: map[string]any{
			"enforce_eager":     true,
			"trust_remote_code": true,
		},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "--enforce-eager") {
		t.Errorf("missing --enforce-eager: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--trust-remote-code") {
		t.Errorf("missing --trust-remote-code: %v", plan.Cmd)
	}
}

func TestVLLM_BoolFalseDoesNotAddFlag(t *testing.T) {
	r, _ := Get("vllm")
	plan, _ := r.BuildPlan(BuildInput{
		DeploymentID: "d",
		ArtifactURI:  "hf://o/m",
		GPUIndices:   []int{0},
		HostPort:     1234,
		Config:       map[string]any{"enforce_eager": false, "trust_remote_code": false},
	})
	joined := strings.Join(plan.Cmd, " ")
	if strings.Contains(joined, "--enforce-eager") || strings.Contains(joined, "--trust-remote-code") {
		t.Errorf("bool=false should not emit flag: %v", plan.Cmd)
	}
}

func TestInfinity_BatchAndDtype(t *testing.T) {
	r, _ := Get("infinity")
	plan, _ := r.BuildPlan(BuildInput{
		DeploymentID: "d",
		ArtifactURI:  "hf://o/m",
		GPUIndices:   []int{0},
		HostPort:     1234,
		Config:       map[string]any{"max_batch_size": 32, "dtype": "float16"},
	})
	joined := strings.Join(plan.Cmd, " ")
	if !strings.Contains(joined, "--batch-size 32") {
		t.Errorf("missing batch-size: %v", plan.Cmd)
	}
	if !strings.Contains(joined, "--dtype float16") {
		t.Errorf("missing dtype: %v", plan.Cmd)
	}
}

// Verify validate() rejects propagate from each recipe (cover BuildPlan's err return).
func TestAllRecipes_RejectBadInput(t *testing.T) {
	bad := BuildInput{} // empty: no DeploymentID, no GPUs, no URI
	for _, name := range Names() {
		r, _ := Get(name)
		if _, err := r.BuildPlan(bad); err == nil {
			t.Errorf("%s: expected validate error", name)
		}
	}
}

func TestContainerName_Sanitises(t *testing.T) {
	got := containerName("p", "abc/def:ghi..jkl")
	if got != "p-abc-def-ghi-jkl" {
		t.Errorf("got %q", got)
	}
}
