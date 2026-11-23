// Package model is the JSON shape AgentSubstrateAPI exposes for Substrate resources.
// Field names follow the public ateapi proto JSON mapping so a live cluster
// and this API speak the same words.
package model

import (
	"bytes"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	ActorUnspecified = "ACTOR_STATE_UNSPECIFIED"
	ActorResuming    = "ACTOR_STATE_RESUMING"
	ActorRunning     = "ACTOR_STATE_RUNNING"
	ActorSuspending  = "ACTOR_STATE_SUSPENDING"
	ActorSuspended   = "ACTOR_STATE_SUSPENDED"
	ActorCrashed     = "ACTOR_STATE_CRASHED"
	ActorDeleting    = "ACTOR_STATE_DELETING"
	ActorReverting   = "ACTOR_STATE_REVERTING"

	WorkerUnspecified = "WORKER_STATE_UNSPECIFIED"
	WorkerActive      = "WORKER_STATE_ACTIVE"
	WorkerDraining    = "WORKER_STATE_DRAINING"
	WorkerUnavailable = "WORKER_STATE_UNAVAILABLE"
)

// FlexInt decodes the int64 values protojson emits as strings, and the
// plain numbers this API emits itself.
type FlexInt int64

func (n FlexInt) MarshalJSON() ([]byte, error) {
	return []byte(strconv.FormatInt(int64(n), 10)), nil
}

func (n *FlexInt) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		if s == "" {
			return nil
		}
		v, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return err
		}
		*n = FlexInt(v)
		return nil
	}
	v, err := strconv.ParseInt(string(b), 10, 64)
	if err != nil {
		return err
	}
	*n = FlexInt(v)
	return nil
}

// ResourceMetadata is the identity every Substrate resource carries.
type ResourceMetadata struct {
	Atespace   string     `json:"atespace,omitempty"`
	Name       string     `json:"name"`
	UID        string     `json:"uid,omitempty"`
	Version    FlexInt    `json:"version,omitempty"`
	CreateTime time.Time  `json:"createTime,omitempty"`
	UpdateTime time.Time  `json:"updateTime,omitempty"`
	DeleteTime *time.Time `json:"deleteTime,omitempty"`
}

// ObjectRef points at a resource by atespace and name.
type ObjectRef struct {
	Atespace string `json:"atespace,omitempty"`
	Name     string `json:"name"`
}

// Selector matches worker pools by exact labels.
type Selector struct {
	MatchLabels map[string]string `json:"matchLabels,omitempty"`
}

// Preconditions are the uid and version a mutation must observe.
type Preconditions struct {
	UID     string
	Version int64
}

// Atespace is the isolation boundary actors and templates live in.
// Description is a console annotation. Live clusters that have no such
// field keep the rest of the resource and ignore it.
type Atespace struct {
	Metadata    ResourceMetadata `json:"metadata"`
	Description string           `json:"description,omitempty"`
}

// EnvVar is one environment variable.
type EnvVar struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// Resources is a compute quantity.
type Resources struct {
	CPUMillicores FlexInt `json:"cpuMillicores,omitempty"`
	MemoryBytes   FlexInt `json:"memoryBytes,omitempty"`
}

// ResourceRequirements declares the limits an actor template requires.
type ResourceRequirements struct {
	Limits Resources `json:"limits"`
}

// Container is one process inside an actor sandbox.
type Container struct {
	Name      string                `json:"name"`
	Image     string                `json:"image"`
	Command   []string              `json:"command,omitempty"`
	Args      []string              `json:"args,omitempty"`
	Env       []EnvVar              `json:"env,omitempty"`
	Resources *ResourceRequirements `json:"resources,omitempty"`
}

// ActorTemplate is the immutable-on-cluster blueprint for an actor.
type ActorTemplate struct {
	Metadata       ResourceMetadata      `json:"metadata"`
	Containers     []Container           `json:"containers,omitempty"`
	WorkerSelector *Selector             `json:"workerSelector,omitempty"`
	Resources      *ResourceRequirements `json:"resources,omitempty"`
	Status         ActorTemplateStatus   `json:"status,omitempty"`
}

// ActorTemplateStatus is server-owned template state.
type ActorTemplateStatus struct {
	GoldenSnapshotStatus *GoldenSnapshotStatus `json:"goldenSnapshotStatus,omitempty"`
}

// GoldenSnapshotStatus points at the template's golden tag once it exists.
type GoldenSnapshotStatus struct {
	GoldenTag    *ObjectRef `json:"goldenTag,omitempty"`
	SnapshotTime time.Time  `json:"snapshotTime,omitempty"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
}

// Actor is a running or suspended agent derived from a template.
type Actor struct {
	Metadata       ResourceMetadata `json:"metadata"`
	ActorTemplate  *ObjectRef       `json:"actorTemplate,omitempty"`
	WorkerSelector *Selector        `json:"workerSelector,omitempty"`
	SourceTag      *ObjectRef       `json:"sourceTag,omitempty"`
	Status         ActorStatus      `json:"status,omitempty"`
}

// ActorStatus is the system-managed lifecycle of an actor.
type ActorStatus struct {
	State            string            `json:"state,omitempty"`
	WorkerAssignment *WorkerAssignment `json:"workerAssignment,omitempty"`
	Crash            *ActorCrash       `json:"crash,omitempty"`
	AssignedNode     string            `json:"assignedNode,omitempty"`
}

// ActorCrash explains a transition to CRASHED.
type ActorCrash struct {
	Message   string    `json:"message,omitempty"`
	CrashTime time.Time `json:"crashTime,omitempty"`
}

// WorkerAssignment is the denormalized placement of an actor.
type WorkerAssignment struct {
	Worker          ObjectRef            `json:"worker"`
	WorkerNamespace string               `json:"workerNamespace,omitempty"`
	WorkerPool      string               `json:"workerPool,omitempty"`
	WorkerPod       string               `json:"workerPod,omitempty"`
	NodeName        string               `json:"nodeName,omitempty"`
	WorkerPodIPs    []string             `json:"workerPodIps,omitempty"`
	SandboxRuntime  *ActorSandboxRuntime `json:"sandboxRuntime,omitempty"`
}

// ActorSandboxRuntime is the class an actor was started under.
type ActorSandboxRuntime struct {
	SandboxClass string `json:"sandboxClass,omitempty"`
}

// Worker is a global standby host that runs actors.
type Worker struct {
	Metadata        ResourceMetadata  `json:"metadata"`
	WorkerNamespace string            `json:"workerNamespace,omitempty"`
	WorkerPool      string            `json:"workerPool,omitempty"`
	WorkerPod       string            `json:"workerPod,omitempty"`
	WorkerPodUID    string            `json:"workerPodUid,omitempty"`
	NodeName        string            `json:"nodeName,omitempty"`
	WorkerSpiffeID  string            `json:"workerSpiffeId,omitempty"`
	Labels          map[string]string `json:"labels,omitempty"`
	Status          WorkerStatus      `json:"status,omitempty"`
}

// WorkerStatus is the capacity and readiness of a worker.
type WorkerStatus struct {
	State           string               `json:"state,omitempty"`
	Capacity        WorkerResources      `json:"capacity,omitempty"`
	Allocated       WorkerResources      `json:"allocated,omitempty"`
	SandboxClasses  []WorkerSandboxClass `json:"sandboxClasses,omitempty"`
	IPs             []string             `json:"ips,omitempty"`
	RegisteredEpoch FlexInt              `json:"registeredEpoch,omitempty"`
}

// WorkerResources is CPU, memory, and an actor slot count.
type WorkerResources struct {
	CPUMillicores FlexInt `json:"cpuMillicores,omitempty"`
	MemoryBytes   FlexInt `json:"memoryBytes,omitempty"`
	Actors        FlexInt `json:"actors,omitempty"`
}

// WorkerSandboxClass is one sandbox family a worker can run.
type WorkerSandboxClass struct {
	SandboxClass string `json:"sandboxClass,omitempty"`
}

// Cluster is a connection the API can route resource calls through.
type Cluster struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Driver             string `json:"driver"`
	Endpoint           string `json:"endpoint,omitempty"`
	Token              string `json:"token,omitempty"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	ServerName         string `json:"serverName,omitempty"`
	CACertPEM          string `json:"caCertPem,omitempty"`
	Kubeconfig         string `json:"kubeconfig,omitempty"`
	KubeContext        string `json:"kubeContext,omitempty"`
	SystemNamespace    string `json:"systemNamespace,omitempty"`
	ServiceAccount     string `json:"serviceAccount,omitempty"`
	Audience           string `json:"audience,omitempty"`
}

// PublicCluster is a cluster with the credential removed.
type PublicCluster struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Driver             string `json:"driver"`
	Endpoint           string `json:"endpoint,omitempty"`
	TLS                bool   `json:"tls"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
	ServerName         string `json:"serverName,omitempty"`
	KubeContext        string `json:"kubeContext,omitempty"`
	SystemNamespace    string `json:"systemNamespace,omitempty"`
	HasToken           bool   `json:"hasToken"`
}

// Probe is the result of asking a cluster whether its control plane answers.
type Probe struct {
	OK        bool   `json:"ok"`
	Service   string `json:"service,omitempty"`
	LatencyMS int64  `json:"latencyMs"`
	Detail    string `json:"detail,omitempty"`
}

// Overview is the landing summary for one cluster.
type Overview struct {
	Cluster       PublicCluster  `json:"cluster"`
	Counts        Counts         `json:"counts"`
	ActorsByState map[string]int `json:"actorsByState"`
	Workers       []Worker       `json:"workers"`
	Atespaces     []Atespace     `json:"atespaces"`
}

// Counts are the four resources the console is built around.
type Counts struct {
	Atespaces      int `json:"atespaces"`
	ActorTemplates int `json:"actorTemplates"`
	Actors         int `json:"actors"`
	Workers        int `json:"workers"`
}

// RouteRequest asks the API to open a local ingress path to an actor.
type RouteRequest struct {
	LocalPort  int `json:"localPort,omitempty"`
	RemotePort int `json:"remotePort,omitempty"`
}

// RouteSession is an active local proxy into an actor.
type RouteSession struct {
	ID         string    `json:"id"`
	Atespace   string    `json:"atespace"`
	Actor      string    `json:"actor"`
	LocalURL   string    `json:"localUrl"`
	LocalPort  int       `json:"localPort"`
	RemotePort int       `json:"remotePort"`
	Status     string    `json:"status"`
	Detail     string    `json:"detail,omitempty"`
	CreatedAt  time.Time `json:"createdAt"`
}

// ExecuteRequest is an HTTP call to send through an actor route.
type ExecuteRequest struct {
	Method     string            `json:"method,omitempty"`
	Path       string            `json:"path,omitempty"`
	Headers    map[string]string `json:"headers,omitempty"`
	Body       string            `json:"body,omitempty"`
	RemotePort int               `json:"remotePort,omitempty"`
	TimeoutMs  int               `json:"timeoutMs,omitempty"`
}

// ExecuteResult is the actor's HTTP response.
type ExecuteResult struct {
	Status    int               `json:"status"`
	Headers   map[string]string `json:"headers,omitempty"`
	Body      string            `json:"body"`
	LatencyMs int64             `json:"latencyMs"`
	LocalURL  string            `json:"localUrl,omitempty"`
	RouteID   string            `json:"routeId,omitempty"`
}

// CommandRequest runs a process inside the actor (sandbox /process protocol).
// Provide either Shell (wrapped as sh -c) or an argv Command.
type CommandRequest struct {
	Shell      string            `json:"shell,omitempty"`
	ShellBin   string            `json:"shellBin,omitempty"`
	Command    []string          `json:"command,omitempty"`
	EnvVars    map[string]string `json:"envvars,omitempty"`
	Cwd        string            `json:"cwd,omitempty"`
	Timeout    string            `json:"timeout,omitempty"`
	Path       string            `json:"path,omitempty"`
	RemotePort int               `json:"remotePort,omitempty"`
	TimeoutMs  int               `json:"timeoutMs,omitempty"`
}

// CommandResult is stdout/stderr from a Command Execute.
type CommandResult struct {
	Stdout    string `json:"stdout"`
	Stderr    string `json:"stderr"`
	ExitCode  int    `json:"exitCode"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
	LocalURL  string `json:"localUrl,omitempty"`
	RouteID   string `json:"routeId,omitempty"`
	RawBody   string `json:"rawBody,omitempty"`
}

var dnsLabel = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

// ValidName reports whether name is a DNS label Substrate will accept.
func ValidName(name string) bool {
	return dnsLabel.MatchString(name)
}

// ValidateAtespace checks a create or update body.
func ValidateAtespace(a Atespace) error {
	if a.Metadata.Atespace != "" {
		return fmt.Errorf("atespace resources are global; metadata.atespace must be empty")
	}
	if !ValidName(a.Metadata.Name) {
		return fmt.Errorf("metadata.name must be a DNS label")
	}
	if len(a.Description) > 512 {
		return fmt.Errorf("description must be at most 512 characters")
	}
	return nil
}

// ValidateActorTemplate checks the fields a client is allowed to set.
func ValidateActorTemplate(t ActorTemplate) error {
	if !ValidName(t.Metadata.Atespace) {
		return fmt.Errorf("metadata.atespace must be a DNS label")
	}
	if !ValidName(t.Metadata.Name) {
		return fmt.Errorf("metadata.name must be a DNS label")
	}
	if len(t.Containers) == 0 {
		return fmt.Errorf("at least one container is required")
	}
	seen := map[string]bool{}
	for _, c := range t.Containers {
		if !ValidName(c.Name) {
			return fmt.Errorf("container name %q must be a DNS label", c.Name)
		}
		if seen[c.Name] {
			return fmt.Errorf("duplicate container name %q", c.Name)
		}
		seen[c.Name] = true
		if strings.TrimSpace(c.Image) == "" {
			return fmt.Errorf("container %q needs an image", c.Name)
		}
	}
	if t.Resources == nil || t.Resources.Limits.CPUMillicores <= 0 || t.Resources.Limits.MemoryBytes <= 0 {
		return fmt.Errorf("resources.limits.cpuMillicores and memoryBytes are required")
	}
	return nil
}

// ValidateActor checks identity and the template reference.
func ValidateActor(a Actor) error {
	if !ValidName(a.Metadata.Atespace) {
		return fmt.Errorf("metadata.atespace must be a DNS label")
	}
	if !ValidName(a.Metadata.Name) {
		return fmt.Errorf("metadata.name must be a DNS label")
	}
	if a.ActorTemplate == nil || !ValidName(a.ActorTemplate.Name) {
		return fmt.Errorf("actorTemplate.name is required")
	}
	space := a.ActorTemplate.Atespace
	if space == "" {
		space = a.Metadata.Atespace
	}
	if !ValidName(space) {
		return fmt.Errorf("actorTemplate.atespace must be a DNS label")
	}
	return nil
}

// ValidateWorker checks the identity a client assigns.
func ValidateWorker(w Worker) error {
	if w.Metadata.Atespace != "" {
		return fmt.Errorf("workers are global; metadata.atespace must be empty")
	}
	if !ValidName(w.Metadata.Name) {
		return fmt.Errorf("metadata.name must be a DNS label")
	}
	if strings.TrimSpace(w.WorkerNamespace) == "" {
		return fmt.Errorf("workerNamespace is required")
	}
	return nil
}

// Public returns the cluster without its credential.
func (c Cluster) Public() PublicCluster {
	return PublicCluster{
		ID:                 c.ID,
		Name:               c.Name,
		Driver:             c.Driver,
		Endpoint:           c.Endpoint,
		TLS:                c.TLS,
		InsecureSkipVerify: c.InsecureSkipVerify,
		ServerName:         c.ServerName,
		KubeContext:        c.KubeContext,
		SystemNamespace:    c.SystemNamespace,
		HasToken:           c.Token != "" || c.Driver == "kube",
	}
}
