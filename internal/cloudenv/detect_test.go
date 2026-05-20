package cloudenv

import (
	"sync"
	"testing"
)

func TestDetect_EnvOverrideSetsKind(t *testing.T) {
	t.Setenv("INFERIA_RUNTIME_ENV", "aws-ec2")
	t.Setenv("INFERIA_INSTANCE_ID", "i-test-1234")
	t.Setenv("INFERIA_REGION", "us-east-1")
	t.Setenv("INFERIA_AZ", "us-east-1a")
	// Force IMDS path off so we don't depend on real network.
	t.Setenv("INFERIA_CLOUDENV_IMDS_URL", "http://127.0.0.1:1") // unreachable

	got := detectFresh() // bypasses cache for tests
	if got.Kind != KindAWSEC2 {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindAWSEC2)
	}
	if got.InstanceID != "i-test-1234" {
		t.Errorf("InstanceID = %q", got.InstanceID)
	}
	if got.Region != "us-east-1" {
		t.Errorf("Region = %q", got.Region)
	}
	if got.AvailabilityZone != "us-east-1a" {
		t.Errorf("AZ = %q", got.AvailabilityZone)
	}
}

func TestDetect_NoEnvNoIMDSReturnsLocal(t *testing.T) {
	t.Setenv("INFERIA_RUNTIME_ENV", "")
	t.Setenv("INFERIA_CLOUDENV_IMDS_URL", "http://127.0.0.1:1") // unreachable
	got := detectFresh()
	if got.Kind != KindLocal {
		t.Fatalf("Kind = %q, want %q", got.Kind, KindLocal)
	}
}

// TestDetect_CacheReturnsSameValue verifies that Detect() caches the result and
// returns identical values on repeated calls.
// NOTE: cacheOnce and cached are process-global (sync.Once). Any test that calls
// Detect() MUST reset them first:
//
//	cacheOnce = sync.Once{}
//	cached = RuntimeInfo{}
func TestDetect_CacheReturnsSameValue(t *testing.T) {
	// Reset the process-global cache so this test owns the first call.
	cacheOnce = sync.Once{}
	cached = RuntimeInfo{}
	t.Cleanup(func() {
		cacheOnce = sync.Once{}
		cached = RuntimeInfo{}
	})

	t.Setenv("INFERIA_RUNTIME_ENV", "aws-ec2")
	t.Setenv("INFERIA_INSTANCE_ID", "i-cache-test")
	t.Setenv("INFERIA_REGION", "us-west-2")
	t.Setenv("INFERIA_AZ", "us-west-2b")
	// Prevent any real IMDS call.
	t.Setenv("INFERIA_CLOUDENV_IMDS_URL", "http://127.0.0.1:1")

	first := Detect()
	if first.Kind != KindAWSEC2 {
		t.Fatalf("first call: Kind = %q, want %q", first.Kind, KindAWSEC2)
	}
	if first.InstanceID != "i-cache-test" {
		t.Fatalf("first call: InstanceID = %q, want %q", first.InstanceID, "i-cache-test")
	}

	second := Detect()
	third := Detect()

	if second != first {
		t.Errorf("second call differs: got %+v, want %+v", second, first)
	}
	if third != first {
		t.Errorf("third call differs: got %+v, want %+v", third, first)
	}
}
