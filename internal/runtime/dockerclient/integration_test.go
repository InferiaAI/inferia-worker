//go:build integration

package dockerclient

import (
	"context"
	"os"
	"testing"
	"time"
)

// realEngine returns a Client connected to the host's docker socket. The test
// is skipped if the socket isn't reachable.
func realEngine(t *testing.T) Client {
	t.Helper()
	host := os.Getenv("DOCKER_HOST")
	if host == "" {
		host = "unix:///var/run/docker.sock"
	}
	c, err := NewEngine(host)
	if err != nil {
		t.Skipf("docker host unreachable: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := c.Ping(ctx); err != nil {
		t.Skipf("docker ping failed: %v", err)
	}
	return c
}

func TestIntegration_PingPullCreateStartStopRemove(t *testing.T) {
	c := realEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	if err := c.EnsureNetwork(ctx, "inferia-integration-test"); err != nil {
		t.Fatalf("EnsureNetwork: %v", err)
	}
	// Pull a small image. Skip if the host has no network.
	if err := c.Pull(ctx, "docker.io/library/hello-world:latest", nil); err != nil {
		t.Skipf("Pull skipped (offline?): %v", err)
	}

	spec := &ContainerSpec{
		Name:  "inferia-it-" + time.Now().Format("150405"),
		Image: "docker.io/library/hello-world:latest",
		Env:   map[string]string{"FOO": "bar"},
		PortBinding: PortBinding{
			HostIP: "127.0.0.1", HostPort: "0", ContainerPort: "8080/tcp",
		},
		NetworkName:     "inferia-integration-test",
		RestartPolicy:   "no",
		Labels:          map[string]string{"inferia.test": "true"},
		GPUDeviceIDs:    nil, // no GPU on most CI hosts
		GPUCapabilities: nil,
	}

	id, err := c.Create(ctx, spec)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	defer func() {
		_ = c.Remove(ctx, id)
	}()

	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("Start: %v", err)
	}
	// hello-world exits immediately; give it a moment.
	time.Sleep(2 * time.Second)

	st, err := c.Inspect(ctx, id)
	if err != nil {
		t.Fatalf("Inspect: %v", err)
	}
	if st.ID == "" {
		t.Errorf("expected ID")
	}
	// hello-world exits with 0 quickly; we accept either still-starting or exited.
	if st.Running {
		_ = c.Stop(ctx, id, 5)
	}
	if _, err := c.Logs(ctx, id, 100); err != nil {
		t.Errorf("Logs: %v", err)
	}
	if err := c.Remove(ctx, id); err != nil {
		t.Errorf("Remove: %v", err)
	}
}

func TestIntegration_StopOnLongRunning(t *testing.T) {
	c := realEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	if err := c.EnsureNetwork(ctx, "inferia-it-stop"); err != nil {
		t.Fatalf("%v", err)
	}
	if err := c.Pull(ctx, "docker.io/library/alpine:latest", nil); err != nil {
		t.Skipf("Pull skipped: %v", err)
	}
	spec := &ContainerSpec{
		Name:  "inferia-it-stop-" + time.Now().Format("150405"),
		Image: "docker.io/library/alpine:latest",
		Cmd:   []string{"sh", "-c", "sleep 60"},
		Env:   map[string]string{},
		PortBinding: PortBinding{
			HostIP: "127.0.0.1", HostPort: "0", ContainerPort: "8080/tcp",
		},
		NetworkName:   "inferia-it-stop",
		RestartPolicy: "no",
		Labels:        map[string]string{"inferia.test": "true"},
	}
	id, err := c.Create(ctx, spec)
	if err != nil {
		t.Fatalf("%v", err)
	}
	defer func() { _ = c.Remove(ctx, id) }()

	if err := c.Start(ctx, id); err != nil {
		t.Fatalf("%v", err)
	}
	st, _ := c.Inspect(ctx, id)
	if !st.Running {
		t.Errorf("expected running")
	}
	if err := c.Stop(ctx, id, 2); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	st, _ = c.Inspect(ctx, id)
	if st.Running {
		t.Errorf("expected stopped")
	}
}

func TestIntegration_EnsureNetworkIdempotent(t *testing.T) {
	c := realEngine(t)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	name := "inferia-it-net"
	if err := c.EnsureNetwork(ctx, name); err != nil {
		t.Fatalf("first: %v", err)
	}
	if err := c.EnsureNetwork(ctx, name); err != nil {
		t.Errorf("second call should be ok, got %v", err)
	}
}
