package control

import (
	"context"
	"encoding/json"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"nhooyr.io/websocket"
)

func TestChannel_LoadModelFailureSurfacedAsCommandResult(t *testing.T) {
	disp := newFakeDispatcher()
	disp.loadErr = errors.New("pull boom")
	done := make(chan CommandResultBody, 1)

	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		_ = writeJSON(ctx, c, Envelope{
			Type: MsgLoadModel, ID: "cmd-fail",
			Body: LoadModelBody{DeploymentID: "dep-1", Recipe: "vllm",
				Model: ModelRef{ArtifactURI: "hf://o/m"}, GPUIndices: []int{0}, Port: 1234},
		})
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var env Envelope
			if json.Unmarshal(data, &env) == nil && env.Type == MsgCommandResult {
				b, _ := json.Marshal(env.Body)
				var r CommandResultBody
				_ = json.Unmarshal(b, &r)
				done <- r
				return
			}
		}
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go ch.Run(ctx)
	select {
	case r := <-done:
		if r.Status != "failed" || r.Detail != "pull boom" || r.InReplyTo != "cmd-fail" {
			t.Errorf("got %+v", r)
		}
	case <-ctx.Done():
		t.Fatalf("no CommandResult")
	}
}

func TestChannel_UnloadModelFailureSurfaced(t *testing.T) {
	disp := newFakeDispatcher()
	disp.unloadErr = errors.New("stop boom")
	done := make(chan CommandResultBody, 1)

	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		_ = writeJSON(ctx, c, Envelope{Type: MsgUnloadModel, ID: "u-1", Body: UnloadModelBody{DeploymentID: "dep-1"}})
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var env Envelope
			if json.Unmarshal(data, &env) == nil && env.Type == MsgCommandResult {
				b, _ := json.Marshal(env.Body)
				var r CommandResultBody
				_ = json.Unmarshal(b, &r)
				done <- r
				return
			}
		}
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go ch.Run(ctx)
	select {
	case r := <-done:
		if r.Status != "failed" || r.InReplyTo != "u-1" {
			t.Errorf("got %+v", r)
		}
	case <-ctx.Done():
		t.Fatalf("no CommandResult")
	}
}

func TestChannel_BadBodyReturnsReplyFailed(t *testing.T) {
	disp := newFakeDispatcher()
	done := make(chan CommandResultBody, 1)

	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		// Body is a string where we expect an object → JSON re-unmarshal fails.
		_ = writeJSON(ctx, c, Envelope{Type: MsgLoadModel, ID: "bad-1", Body: "not-an-object"})
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var env Envelope
			if json.Unmarshal(data, &env) == nil && env.Type == MsgCommandResult {
				b, _ := json.Marshal(env.Body)
				var r CommandResultBody
				_ = json.Unmarshal(b, &r)
				done <- r
				return
			}
		}
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go ch.Run(ctx)
	select {
	case r := <-done:
		if r.Status != "failed" || r.Detail != "bad LoadModel body" {
			t.Errorf("got %+v", r)
		}
	case <-ctx.Done():
		t.Fatalf("no CommandResult")
	}
}

func TestChannel_BadUnloadBody(t *testing.T) {
	disp := newFakeDispatcher()
	done := make(chan CommandResultBody, 1)

	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		_ = writeJSON(ctx, c, Envelope{Type: MsgUnloadModel, ID: "bad-u", Body: "not-an-object"})
		for {
			_, data, err := c.Read(ctx)
			if err != nil {
				return
			}
			var env Envelope
			if json.Unmarshal(data, &env) == nil && env.Type == MsgCommandResult {
				b, _ := json.Marshal(env.Body)
				var r CommandResultBody
				_ = json.Unmarshal(b, &r)
				done <- r
				return
			}
		}
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 1500*time.Millisecond)
	defer cancel()
	go ch.Run(ctx)
	select {
	case r := <-done:
		if r.Status != "failed" || r.Detail != "bad UnloadModel body" {
			t.Errorf("got %+v", r)
		}
	case <-ctx.Done():
		t.Fatalf("no CommandResult")
	}
}

func TestChannel_MalformedFrameIgnored(t *testing.T) {
	disp := newFakeDispatcher()
	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		// Send raw garbage (not valid JSON envelope).
		_ = c.Write(ctx, websocket.MessageText, []byte("garbage not json"))
		// Then send a valid Unload so we can verify the channel continues.
		_ = writeJSON(ctx, c, Envelope{Type: MsgUnloadModel, ID: "u-after", Body: UnloadModelBody{DeploymentID: "x"}})
		time.Sleep(80 * time.Millisecond)
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	_ = ch.Run(ctx)
	if atomic.LoadInt32(&disp.unloadCalls) != 1 {
		t.Errorf("expected 1 unload despite the garbage frame, got %d", disp.unloadCalls)
	}
}

func TestChannel_ReconnectsAfterPeerClose(t *testing.T) {
	disp := newFakeDispatcher()
	var connections int32
	cp := newFakeCPServer(func(ctx context.Context, c *websocket.Conn, s *fakeCPServer) {
		atomic.AddInt32(&connections, 1)
		// First connect: close immediately.
		_ = c.Close(websocket.StatusNormalClosure, "")
	})
	defer cp.srv.Close()

	ch := &Channel{
		ChannelURL:        wsURL(cp.srv.URL) + "/v1/workers/channel",
		Token:             func() string { return "jwt" },
		HeartbeatInterval: 1 * time.Hour,
		Dispatcher:        disp,
		DedupTTL:          time.Minute,
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2200*time.Millisecond)
	defer cancel()
	_ = ch.Run(ctx)
	if atomic.LoadInt32(&connections) < 2 {
		t.Errorf("expected ≥2 connection attempts, got %d", connections)
	}
}

func TestMinInt(t *testing.T) {
	cases := map[[2]int]int{{1, 2}: 1, {3, 2}: 2, {5, 5}: 5, {-1, 0}: -1}
	for in, want := range cases {
		if got := minInt(in[0], in[1]); got != want {
			t.Errorf("minInt(%d,%d)=%d want %d", in[0], in[1], got, want)
		}
	}
}

func TestNewID_UniqueLikely(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 1000; i++ {
		id := newID()
		if seen[id] {
			t.Fatalf("dup id %q at i=%d", id, i)
		}
		seen[id] = true
	}
}

func TestRemarshal_BadInput(t *testing.T) {
	// json.Marshal can never fail on a string, but unmarshal to a struct can.
	var out struct{ X int }
	if err := remarshal("not-an-object", &out); err == nil {
		t.Errorf("expected error")
	}
}

func TestRunStopsOnCtxCancel(t *testing.T) {
	disp := newFakeDispatcher()
	ch := &Channel{
		ChannelURL: "ws://127.0.0.1:1/x",
		Token:      func() string { return "" },
		Dispatcher: disp,
		DedupTTL:   time.Minute,
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := ch.Run(ctx); err == nil {
		t.Errorf("expected ctx error")
	}
}
