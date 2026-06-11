package recipes

// KvRole is the mooncake KV cache role assigned to a container in a
// disagg deployment group.
type KvRole string

const (
	KvRoleProducer KvRole = "kv_producer"
	KvRoleConsumer KvRole = "kv_consumer"
)

// ContainerPlan describes one container within a multi-container deployment.
// Fields mirror Plan but omit ContainerName and HostPort — the runtime
// generates those dynamically.
type ContainerPlan struct {
	Image         string
	Cmd           []string
	Entrypoint    []string
	Env           map[string]string
	Mounts        []Mount
	ContainerPort int
	GPUIndices    []int
	ShmSize       int64
	ReadyPath     string
	Role          KvRole
	ReplicaIdx    int
}

// ToPlan converts the ContainerPlan into a runnable Plan by filling in the
// runtime-assigned container name and host port.
func (cp ContainerPlan) ToPlan(containerName string, hostPort int) Plan {
	return Plan{
		Image:         cp.Image,
		ContainerName: containerName,
		Cmd:           cp.Cmd,
		Entrypoint:    cp.Entrypoint,
		Env:           cp.Env,
		Mounts:        cp.Mounts,
		ContainerPort: cp.ContainerPort,
		HostPort:      hostPort,
		GPUIndices:    cp.GPUIndices,
		ShmSize:       cp.ShmSize,
		ReadyPath:     cp.ReadyPath,
	}
}

// DeploymentPlan is the blueprint for a full disagg deployment group.
// The runtime uses it to launch N prefill + M decode containers.
type DeploymentPlan struct {
	DeploymentID string
	Model        string
	Prefill      []ContainerPlan
	Decode       []ContainerPlan
	// ContainerPrefix is used by the runtime to name containers,
	// e.g. "<prefix>-p-<id>-<i>". If empty, defaults to "inferia-vllm"
	// for backward compatibility.
	ContainerPrefix string
}
