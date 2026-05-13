package control

import (
	"testing"
	"time"
)

func TestDedup_NewIDExecutesOnce(t *testing.T) {
	d := newDedup(50 * time.Millisecond)
	calls := 0
	res, hit := d.Run("id-1", func() CommandResultBody {
		calls++
		return CommandResultBody{Status: "ok"}
	})
	if hit {
		t.Errorf("first call should not be a cache hit")
	}
	if res.Status != "ok" {
		t.Errorf("status: %q", res.Status)
	}

	// Same id again within window: returns cached.
	res2, hit2 := d.Run("id-1", func() CommandResultBody {
		calls++
		return CommandResultBody{Status: "ok"}
	})
	if !hit2 {
		t.Errorf("expected cache hit on replay")
	}
	if res2.Status != res.Status {
		t.Errorf("cached result differs")
	}
	if calls != 1 {
		t.Errorf("expected 1 underlying call, got %d", calls)
	}
}

func TestDedup_ExpiresAfterTTL(t *testing.T) {
	d := newDedup(20 * time.Millisecond)
	d.Run("id-1", func() CommandResultBody { return CommandResultBody{Status: "ok"} })
	time.Sleep(40 * time.Millisecond)
	_, hit := d.Run("id-1", func() CommandResultBody { return CommandResultBody{Status: "ok"} })
	if hit {
		t.Errorf("expected cache to expire")
	}
}

func TestDedup_DifferentIDsIndependent(t *testing.T) {
	d := newDedup(50 * time.Millisecond)
	d.Run("a", func() CommandResultBody { return CommandResultBody{Status: "ok"} })
	_, hit := d.Run("b", func() CommandResultBody { return CommandResultBody{Status: "failed"} })
	if hit {
		t.Errorf("different ids should not collide")
	}
}

func TestDedup_EmptyIDAlwaysExecutes(t *testing.T) {
	d := newDedup(50 * time.Millisecond)
	calls := 0
	d.Run("", func() CommandResultBody { calls++; return CommandResultBody{} })
	d.Run("", func() CommandResultBody { calls++; return CommandResultBody{} })
	if calls != 2 {
		t.Errorf("empty-id calls must not be deduped, got %d", calls)
	}
}
