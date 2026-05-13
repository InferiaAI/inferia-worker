package fake

import (
	"context"
	"errors"
	"testing"

	"github.com/inferia/inferia-worker/internal/runtime/dockerclient"
)

func TestFake_Lifecycle(t *testing.T) {
	c := New()
	ctx := context.Background()
	if err := c.Ping(ctx); err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.EnsureNetwork(ctx, "inferia-models"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.Pull(ctx, "img"); err != nil {
		t.Fatalf("%v", err)
	}
	id, err := c.Create(ctx, &dockerclient.ContainerSpec{Name: "foo"})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("%v", err)
	}
	st, _ := c.Inspect(ctx, id)
	if !st.Running {
		t.Errorf("expected running")
	}
	if err := c.Stop(ctx, id, 5); err != nil {
		t.Fatalf("%v", err)
	}
	st, _ = c.Inspect(ctx, id)
	if st.Running {
		t.Errorf("expected stopped")
	}
	c.MarkExited(id, 137)
	st, _ = c.Inspect(ctx, id)
	if st.ExitCode != 137 {
		t.Errorf("ExitCode: %d", st.ExitCode)
	}
	if err := c.Remove(ctx, id); err != nil {
		t.Fatalf("%v", err)
	}
	if _, err := c.Inspect(ctx, id); err == nil {
		t.Errorf("expected not found after remove")
	}
}

func TestFake_ErrorInjection(t *testing.T) {
	c := New()
	ctx := context.Background()
	boom := errors.New("boom")
	cases := map[string]func() error{
		"Ping":          func() error { c.PingErr = boom; return c.Ping(ctx) },
		"EnsureNetwork": func() error { c.EnsureNetworkErr = boom; return c.EnsureNetwork(ctx, "n") },
		"Pull":          func() error { c.PullErr = boom; return c.Pull(ctx, "img") },
		"Create": func() error {
			c.CreateErr = boom
			_, err := c.Create(ctx, &dockerclient.ContainerSpec{Name: "x"})
			return err
		},
		"Start":   func() error { c.StartErr = boom; return c.Start(ctx, "x") },
		"Stop":    func() error { c.StopErr = boom; return c.Stop(ctx, "x", 5) },
		"Remove":  func() error { c.RemoveErr = boom; return c.Remove(ctx, "x") },
		"Inspect": func() error { c.InspectErr = boom; _, err := c.Inspect(ctx, "x"); return err },
		"Logs":    func() error { c.LogsErr = boom; _, err := c.Logs(ctx, "x", 10); return err },
	}
	for name, fn := range cases {
		t.Run(name, func(t *testing.T) {
			c := New()
			// re-bind by setting the same fields fresh per subtest
			_ = c
			// the closures captured the outer c; create a fresh fake per case:
			f := New()
			switch name {
			case "Ping":
				f.PingErr = boom
				if err := f.Ping(ctx); err == nil {
					t.Errorf("expected error")
				}
			case "EnsureNetwork":
				f.EnsureNetworkErr = boom
				if err := f.EnsureNetwork(ctx, "n"); err == nil {
					t.Errorf("expected error")
				}
			case "Pull":
				f.PullErr = boom
				if err := f.Pull(ctx, "img"); err == nil {
					t.Errorf("expected error")
				}
			case "Create":
				f.CreateErr = boom
				if _, err := f.Create(ctx, &dockerclient.ContainerSpec{Name: "x"}); err == nil {
					t.Errorf("expected error")
				}
			case "Start":
				f.StartErr = boom
				if err := f.Start(ctx, "x"); err == nil {
					t.Errorf("expected error")
				}
			case "Stop":
				f.StopErr = boom
				if err := f.Stop(ctx, "x", 5); err == nil {
					t.Errorf("expected error")
				}
			case "Remove":
				f.RemoveErr = boom
				if err := f.Remove(ctx, "x"); err == nil {
					t.Errorf("expected error")
				}
			case "Inspect":
				f.InspectErr = boom
				if _, err := f.Inspect(ctx, "x"); err == nil {
					t.Errorf("expected error")
				}
			case "Logs":
				f.LogsErr = boom
				if _, err := f.Logs(ctx, "x", 5); err == nil {
					t.Errorf("expected error")
				}
			}
			_ = fn // silence unused
		})
	}
}

func TestFake_Logs(t *testing.T) {
	c := New()
	c.FakeLogs = []byte("hello")
	got, err := c.Logs(context.Background(), "x", 10)
	if err != nil {
		t.Fatalf("%v", err)
	}
	if string(got) != "hello" {
		t.Errorf("got %q", string(got))
	}
}
