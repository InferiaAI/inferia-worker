package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// Bootstrapper performs the one-shot HTTP exchange to swap a bootstrap token
// for a long-lived worker JWT.
type Bootstrapper struct {
	ControlPlaneURL string
	BootstrapToken  string
	HTTP            *http.Client
}

// Register posts the worker's identity to /v1/workers/register and returns
// the issued node_id + worker_jwt.
func (b *Bootstrapper) Register(ctx context.Context, req RegisterRequest) (*RegisterResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, "POST",
		b.ControlPlaneURL+"/v1/workers/register", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+b.BootstrapToken)

	httpClient := b.HTTP
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	resp, err := httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("register dial: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("register failed: status=%d body=%s", resp.StatusCode, string(respBody))
	}
	var out RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("register decode: %w", err)
	}
	return &out, nil
}
