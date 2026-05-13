// Package control speaks the worker side of the control-plane protocol:
// register + token exchange (HTTPS), then a long-lived WebSocket carrying
// Heartbeat / LoadModel / UnloadModel / CommandResult / Hello / Ping frames.
package control

import "time"

// MessageType is one of the strings in the catalogue.
type MessageType string

const (
	MsgHello         MessageType = "Hello"
	MsgHeartbeat     MessageType = "Heartbeat"
	MsgLoadModel     MessageType = "LoadModel"
	MsgUnloadModel   MessageType = "UnloadModel"
	MsgCommandResult MessageType = "CommandResult"
	MsgPing          MessageType = "Ping"
)

// Envelope wraps every frame. id is a UUIDv4 used for command/response
// correlation. ts is RFC3339Nano.
type Envelope struct {
	Type MessageType `json:"type"`
	ID   string      `json:"id"`
	TS   string      `json:"ts"`
	Body any         `json:"body,omitempty"`
}

// RegisterRequest is the bootstrap-time POST body.
type RegisterRequest struct {
	NodeName     string            `json:"node_name"`
	PoolID       string            `json:"pool_id"`
	AdvertiseURL string            `json:"advertise_url"`
	Allocatable  map[string]string `json:"allocatable"`
}

// RegisterResponse is the control plane's reply.
type RegisterResponse struct {
	NodeID    string `json:"node_id"`
	WorkerJWT string `json:"worker_jwt"`
}

// HelloBody is sent by the control plane immediately after WS upgrade.
type HelloBody struct {
	ServerTime time.Time `json:"server_time"`
	ChannelID  string    `json:"channel_id"`
}

// HeartbeatBody is what the worker sends every interval. used is a map of
// resource → opaque string (matches Python compute_node.proto for migration).
type HeartbeatBody struct {
	Used         map[string]string `json:"used"`
	LoadedModels []string          `json:"loaded_models"`
	Events       []HeartbeatEvent  `json:"events,omitempty"`
}

// HeartbeatEvent represents asynchronous lifecycle facts piggybacked on the
// heartbeat (rather than a separate WS frame). MVP only emits ModelExited.
type HeartbeatEvent struct {
	Type         string `json:"type"`
	DeploymentID string `json:"deployment_id"`
	ExitCode     int    `json:"exit_code,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// LoadModelBody is the command from CP to worker.
type LoadModelBody struct {
	DeploymentID string            `json:"deployment_id"`
	Recipe       string            `json:"recipe"`
	Model        ModelRef          `json:"model"`
	Config       map[string]any    `json:"config,omitempty"`
	GPUIndices   []int             `json:"gpu_indices"`
	Port         int               `json:"port,omitempty"`
	Env          map[string]string `json:"env,omitempty"`
}

// ModelRef points at an artifact.
type ModelRef struct {
	ArtifactURI string `json:"artifact_uri"`
	Format      string `json:"format,omitempty"`
	Backend     string `json:"backend,omitempty"`
}

// UnloadModelBody is the command to free a model.
type UnloadModelBody struct {
	DeploymentID string `json:"deployment_id"`
}

// CommandResultBody is the worker's response to a command.
type CommandResultBody struct {
	InReplyTo   string `json:"in_reply_to"`
	Status      string `json:"status"` // "ok" | "failed"
	Detail      string `json:"detail,omitempty"`
	EndpointURL string `json:"endpoint_url,omitempty"`
}
