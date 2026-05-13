package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBootstrap_HappyPath(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/workers/register" {
			http.Error(w, "wrong path", 404)
			return
		}
		if r.Method != "POST" {
			http.Error(w, "wrong method", 405)
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer bootstrap-x" {
			http.Error(w, "bad auth: "+got, 401)
			return
		}
		var req RegisterRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		if req.NodeName == "" || req.PoolID == "" {
			http.Error(w, "missing required fields", 400)
			return
		}
		_ = json.NewEncoder(w).Encode(RegisterResponse{
			NodeID:    "node-uuid",
			WorkerJWT: "jwt-token",
		})
	}))
	defer server.Close()

	b := &Bootstrapper{
		ControlPlaneURL: server.URL,
		BootstrapToken:  "bootstrap-x",
		HTTP:            server.Client(),
	}
	resp, err := b.Register(context.Background(), RegisterRequest{
		NodeName: "n", PoolID: "p", AdvertiseURL: "https://w", Allocatable: map[string]string{"gpu": "1"},
	})
	if err != nil {
		t.Fatalf("%v", err)
	}
	if resp.NodeID != "node-uuid" || resp.WorkerJWT != "jwt-token" {
		t.Errorf("got %+v", resp)
	}
}

func TestBootstrap_401(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "no", 401)
	}))
	defer server.Close()
	b := &Bootstrapper{ControlPlaneURL: server.URL, BootstrapToken: "x", HTTP: server.Client()}
	_, err := b.Register(context.Background(), RegisterRequest{NodeName: "n", PoolID: "p"})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401, got %v", err)
	}
}

func TestBootstrap_409Conflict(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "node name taken", 409)
	}))
	defer server.Close()
	b := &Bootstrapper{ControlPlaneURL: server.URL, BootstrapToken: "x", HTTP: server.Client()}
	_, err := b.Register(context.Background(), RegisterRequest{NodeName: "n", PoolID: "p"})
	if err == nil || !strings.Contains(err.Error(), "409") {
		t.Errorf("expected 409, got %v", err)
	}
}

func TestBootstrap_MalformedJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("not json"))
	}))
	defer server.Close()
	b := &Bootstrapper{ControlPlaneURL: server.URL, BootstrapToken: "x", HTTP: server.Client()}
	_, err := b.Register(context.Background(), RegisterRequest{NodeName: "n", PoolID: "p"})
	if err == nil {
		t.Errorf("expected json decode error")
	}
}

func TestBootstrap_NetworkError(t *testing.T) {
	b := &Bootstrapper{ControlPlaneURL: "http://127.0.0.1:1", BootstrapToken: "x", HTTP: http.DefaultClient}
	_, err := b.Register(context.Background(), RegisterRequest{NodeName: "n", PoolID: "p"})
	if err == nil {
		t.Errorf("expected dial error")
	}
}

func TestBootstrap_DefaultHTTPClient(t *testing.T) {
	b := &Bootstrapper{ControlPlaneURL: "http://127.0.0.1:1", BootstrapToken: "x"}
	// We just want to exercise the lazy default — won't connect.
	_, _ = b.Register(context.Background(), RegisterRequest{NodeName: "n", PoolID: "p"})
}
