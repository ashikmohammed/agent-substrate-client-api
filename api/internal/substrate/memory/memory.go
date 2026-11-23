// Package memory is a Cluster that keeps Substrate resources in process.
// It is the demo backend and the reference for how a driver should behave.
package memory

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
)

// Store implements substrate.Cluster.
type Store struct {
	mu        sync.Mutex
	path      string
	atespaces map[string]model.Atespace
	templates map[string]model.ActorTemplate
	actors    map[string]model.Actor
	workers   map[string]model.Worker
	routes    map[string]model.RouteSession
	counters  map[string]int
}

// New returns an empty store. When path is set, mutations are written there
// and an existing file is loaded. seed fills the demo cluster when the file
// is missing.
func New(path string, seed bool) (*Store, error) {
	s := &Store{
		path:      path,
		atespaces: map[string]model.Atespace{},
		templates: map[string]model.ActorTemplate{},
		actors:    map[string]model.Actor{},
		workers:   map[string]model.Worker{},
	}
	if path != "" {
		if err := s.load(); err != nil {
			return nil, err
		}
	}
	if seed && len(s.atespaces) == 0 && len(s.workers) == 0 {
		s.seed()
		if err := s.save(); err != nil {
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) Probe(context.Context) (model.Probe, error) {
	return model.Probe{
		OK:      true,
		Service: "memory.Cluster",
		Detail:  "in-process demo cluster",
	}, nil
}

func (s *Store) CreateAtespace(_ context.Context, item model.Atespace) (model.Atespace, error) {
	if err := model.ValidateAtespace(item); err != nil {
		return model.Atespace{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.atespaces[item.Metadata.Name]; ok {
		return model.Atespace{}, substrate.AlreadyExists("atespace %q already exists", item.Metadata.Name)
	}
	item.Metadata = newMeta("", item.Metadata.Name)
	s.atespaces[item.Metadata.Name] = item
	if err := s.save(); err != nil {
		return model.Atespace{}, err
	}
	return clone(item), nil
}

func (s *Store) GetAtespace(_ context.Context, name string) (model.Atespace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.atespaces[name]
	if !ok {
		return model.Atespace{}, substrate.NotFound("atespace %q not found", name)
	}
	return clone(item), nil
}

func (s *Store) ListAtespaces(context.Context) ([]model.Atespace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Atespace, 0, len(s.atespaces))
	for _, item := range s.atespaces {
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metadata.Name < out[j].Metadata.Name })
	return out, nil
}

func (s *Store) UpdateAtespace(_ context.Context, item model.Atespace) (model.Atespace, error) {
	if err := model.ValidateAtespace(item); err != nil {
		return model.Atespace{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.atespaces[item.Metadata.Name]
	if !ok {
		return model.Atespace{}, substrate.NotFound("atespace %q not found", item.Metadata.Name)
	}
	if err := checkPre(cur.Metadata, item.Metadata); err != nil {
		return model.Atespace{}, err
	}
	cur.Description = item.Description
	touch(&cur.Metadata)
	s.atespaces[cur.Metadata.Name] = cur
	if err := s.save(); err != nil {
		return model.Atespace{}, err
	}
	return clone(cur), nil
}

func (s *Store) DeleteAtespace(_ context.Context, name string, pre model.Preconditions) (model.Atespace, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.atespaces[name]
	if !ok {
		return model.Atespace{}, substrate.NotFound("atespace %q not found", name)
	}
	if err := checkPrecond(cur.Metadata, pre); err != nil {
		return model.Atespace{}, err
	}
	for _, t := range s.templates {
		if t.Metadata.Atespace == name {
			return model.Atespace{}, substrate.FailedPrecondition("atespace %q still has actor templates", name)
		}
	}
	for _, a := range s.actors {
		if a.Metadata.Atespace == name {
			return model.Atespace{}, substrate.FailedPrecondition("atespace %q still has actors", name)
		}
	}
	stampDelete(&cur.Metadata)
	delete(s.atespaces, name)
	if err := s.save(); err != nil {
		return model.Atespace{}, err
	}
	return clone(cur), nil
}

func (s *Store) CreateActorTemplate(_ context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	if err := model.ValidateActorTemplate(item); err != nil {
		return model.ActorTemplate{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.atespaces[item.Metadata.Atespace]; !ok {
		return model.ActorTemplate{}, substrate.FailedPrecondition("atespace %q not found", item.Metadata.Atespace)
	}
	key := ref(item.Metadata.Atespace, item.Metadata.Name)
	if _, ok := s.templates[key]; ok {
		return model.ActorTemplate{}, substrate.AlreadyExists("actor template %s already exists", key)
	}
	item.Metadata = newMeta(item.Metadata.Atespace, item.Metadata.Name)
	item.Status = model.ActorTemplateStatus{}
	s.templates[key] = item
	if err := s.save(); err != nil {
		return model.ActorTemplate{}, err
	}
	return clone(item), nil
}

func (s *Store) GetActorTemplate(_ context.Context, atespace, name string) (model.ActorTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.templates[ref(atespace, name)]
	if !ok {
		return model.ActorTemplate{}, substrate.NotFound("actor template %s not found", ref(atespace, name))
	}
	return clone(item), nil
}

func (s *Store) ListActorTemplates(_ context.Context, atespace string) ([]model.ActorTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.ActorTemplate, 0)
	for _, item := range s.templates {
		if atespace != "" && item.Metadata.Atespace != atespace {
			continue
		}
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Metadata.Atespace == out[j].Metadata.Atespace {
			return out[i].Metadata.Name < out[j].Metadata.Name
		}
		return out[i].Metadata.Atespace < out[j].Metadata.Atespace
	})
	return out, nil
}

func (s *Store) UpdateActorTemplate(_ context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	if err := model.ValidateActorTemplate(item); err != nil {
		return model.ActorTemplate{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ref(item.Metadata.Atespace, item.Metadata.Name)
	cur, ok := s.templates[key]
	if !ok {
		return model.ActorTemplate{}, substrate.NotFound("actor template %s not found", key)
	}
	if err := checkPre(cur.Metadata, item.Metadata); err != nil {
		return model.ActorTemplate{}, err
	}
	item.Metadata = cur.Metadata
	item.Status = cur.Status
	touch(&item.Metadata)
	s.templates[key] = item
	if err := s.save(); err != nil {
		return model.ActorTemplate{}, err
	}
	return clone(item), nil
}

func (s *Store) DeleteActorTemplate(_ context.Context, atespace, name string, pre model.Preconditions) (model.ActorTemplate, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ref(atespace, name)
	cur, ok := s.templates[key]
	if !ok {
		return model.ActorTemplate{}, substrate.NotFound("actor template %s not found", key)
	}
	if err := checkPrecond(cur.Metadata, pre); err != nil {
		return model.ActorTemplate{}, err
	}
	for _, a := range s.actors {
		if a.ActorTemplate != nil && a.ActorTemplate.Atespace == atespace && a.ActorTemplate.Name == name {
			return model.ActorTemplate{}, substrate.FailedPrecondition("actor template %s is still used by actor %q", key, a.Metadata.Name)
		}
	}
	stampDelete(&cur.Metadata)
	delete(s.templates, key)
	if err := s.save(); err != nil {
		return model.ActorTemplate{}, err
	}
	return clone(cur), nil
}

func (s *Store) CreateActor(_ context.Context, item model.Actor) (model.Actor, error) {
	if item.ActorTemplate != nil && item.ActorTemplate.Atespace == "" {
		item.ActorTemplate.Atespace = item.Metadata.Atespace
	}
	if err := model.ValidateActor(item); err != nil {
		return model.Actor{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.atespaces[item.Metadata.Atespace]; !ok {
		return model.Actor{}, substrate.FailedPrecondition("atespace %q not found", item.Metadata.Atespace)
	}
	if _, ok := s.templates[ref(item.ActorTemplate.Atespace, item.ActorTemplate.Name)]; !ok {
		return model.Actor{}, substrate.FailedPrecondition("actor template %s not found", ref(item.ActorTemplate.Atespace, item.ActorTemplate.Name))
	}
	key := ref(item.Metadata.Atespace, item.Metadata.Name)
	if _, ok := s.actors[key]; ok {
		return model.Actor{}, substrate.AlreadyExists("actor %s already exists", key)
	}
	item.Metadata = newMeta(item.Metadata.Atespace, item.Metadata.Name)
	if item.Status.State == "" {
		item.Status.State = model.ActorSuspended
	}
	s.actors[key] = item
	if err := s.save(); err != nil {
		return model.Actor{}, err
	}
	return clone(item), nil
}

func (s *Store) GetActor(_ context.Context, atespace, name string) (model.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.actors[ref(atespace, name)]
	if !ok {
		return model.Actor{}, substrate.NotFound("actor %s not found", ref(atespace, name))
	}
	return clone(item), nil
}

func (s *Store) ListActors(_ context.Context, atespace string) ([]model.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Actor, 0)
	for _, item := range s.actors {
		if atespace != "" && item.Metadata.Atespace != atespace {
			continue
		}
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Metadata.Atespace == out[j].Metadata.Atespace {
			return out[i].Metadata.Name < out[j].Metadata.Name
		}
		return out[i].Metadata.Atespace < out[j].Metadata.Atespace
	})
	return out, nil
}

func (s *Store) UpdateActor(_ context.Context, item model.Actor) (model.Actor, error) {
	if item.ActorTemplate != nil && item.ActorTemplate.Atespace == "" {
		item.ActorTemplate.Atespace = item.Metadata.Atespace
	}
	if err := model.ValidateActor(item); err != nil {
		return model.Actor{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ref(item.Metadata.Atespace, item.Metadata.Name)
	cur, ok := s.actors[key]
	if !ok {
		return model.Actor{}, substrate.NotFound("actor %s not found", key)
	}
	if err := checkPre(cur.Metadata, item.Metadata); err != nil {
		return model.Actor{}, err
	}
	if cur.ActorTemplate != nil && item.ActorTemplate != nil &&
		(cur.ActorTemplate.Name != item.ActorTemplate.Name || cur.ActorTemplate.Atespace != item.ActorTemplate.Atespace) {
		return model.Actor{}, substrate.Invalid("actorTemplate is immutable")
	}
	cur.WorkerSelector = item.WorkerSelector
	touch(&cur.Metadata)
	s.actors[key] = cur
	if err := s.save(); err != nil {
		return model.Actor{}, err
	}
	return clone(cur), nil
}

func (s *Store) DeleteActor(_ context.Context, atespace, name string, pre model.Preconditions) (model.Actor, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := ref(atespace, name)
	cur, ok := s.actors[key]
	if !ok {
		return model.Actor{}, substrate.NotFound("actor %s not found", key)
	}
	if err := checkPrecond(cur.Metadata, pre); err != nil {
		return model.Actor{}, err
	}
	stampDelete(&cur.Metadata)
	delete(s.actors, key)
	if err := s.save(); err != nil {
		return model.Actor{}, err
	}
	return clone(cur), nil
}

func (s *Store) CreateWorker(_ context.Context, item model.Worker) (model.Worker, error) {
	if err := model.ValidateWorker(item); err != nil {
		return model.Worker{}, substrate.Invalid("%s", err.Error())
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.workers[item.Metadata.Name]; ok {
		return model.Worker{}, substrate.AlreadyExists("worker %q already exists", item.Metadata.Name)
	}
	item.Metadata = newMeta("", item.Metadata.Name)
	if item.Status.State == "" {
		item.Status.State = model.WorkerUnavailable
	}
	if item.Status.Capacity.Actors == 0 {
		item.Status.Capacity.Actors = 1000
	}
	if len(item.Status.SandboxClasses) == 0 {
		item.Status.SandboxClasses = []model.WorkerSandboxClass{{SandboxClass: "gvisor"}}
	}
	s.workers[item.Metadata.Name] = item
	if err := s.save(); err != nil {
		return model.Worker{}, err
	}
	return clone(item), nil
}

func (s *Store) GetWorker(_ context.Context, name string) (model.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.workers[name]
	if !ok {
		return model.Worker{}, substrate.NotFound("worker %q not found", name)
	}
	return clone(item), nil
}

func (s *Store) ListWorkers(context.Context) ([]model.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]model.Worker, 0, len(s.workers))
	for _, item := range s.workers {
		out = append(out, clone(item))
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Metadata.Name < out[j].Metadata.Name })
	return out, nil
}

func (s *Store) UpdateWorker(_ context.Context, item model.Worker) (model.Worker, error) {
	if !model.ValidName(item.Metadata.Name) {
		return model.Worker{}, substrate.Invalid("metadata.name must be a DNS label")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.workers[item.Metadata.Name]
	if !ok {
		return model.Worker{}, substrate.NotFound("worker %q not found", item.Metadata.Name)
	}
	if err := checkPre(cur.Metadata, item.Metadata); err != nil {
		return model.Worker{}, err
	}
	if item.WorkerNamespace != "" && item.WorkerNamespace != cur.WorkerNamespace {
		return model.Worker{}, substrate.Invalid("workerNamespace is immutable")
	}
	cur.Labels = item.Labels
	touch(&cur.Metadata)
	s.workers[cur.Metadata.Name] = cur
	if err := s.save(); err != nil {
		return model.Worker{}, err
	}
	return clone(cur), nil
}

func (s *Store) DeleteWorker(_ context.Context, name string, pre model.Preconditions) (model.Worker, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cur, ok := s.workers[name]
	if !ok {
		return model.Worker{}, substrate.NotFound("worker %q not found", name)
	}
	if err := checkPrecond(cur.Metadata, pre); err != nil {
		return model.Worker{}, err
	}
	for _, a := range s.actors {
		if a.Status.WorkerAssignment != nil && a.Status.WorkerAssignment.Worker.Name == name {
			return model.Worker{}, substrate.FailedPrecondition("worker %q still hosts actor %s", name, ref(a.Metadata.Atespace, a.Metadata.Name))
		}
	}
	stampDelete(&cur.Metadata)
	delete(s.workers, name)
	if err := s.save(); err != nil {
		return model.Worker{}, err
	}
	return clone(cur), nil
}

func checkPre(cur, next model.ResourceMetadata) error {
	if next.UID == "" || next.Version == 0 {
		return substrate.Invalid("metadata.uid and metadata.version are required")
	}
	return checkPrecond(cur, model.Preconditions{UID: next.UID, Version: int64(next.Version)})
}

func checkPrecond(cur model.ResourceMetadata, pre model.Preconditions) error {
	if pre.UID != "" && pre.UID != cur.UID {
		return substrate.Aborted("uid precondition failed for %q", cur.Name)
	}
	if pre.Version != 0 && pre.Version != int64(cur.Version) {
		return substrate.Aborted("version precondition failed for %q", cur.Name)
	}
	return nil
}

func newMeta(atespace, name string) model.ResourceMetadata {
	t := now()
	return model.ResourceMetadata{
		Atespace:   atespace,
		Name:       name,
		UID:        newUID(),
		Version:    1,
		CreateTime: t,
		UpdateTime: t,
	}
}

func touch(m *model.ResourceMetadata) {
	m.Version++
	m.UpdateTime = now()
}

func now() time.Time {
	return time.Now().UTC().Truncate(time.Millisecond)
}

func stampDelete(m *model.ResourceMetadata) {
	t := now()
	m.DeleteTime = &t
}

func newUID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func ref(atespace, name string) string {
	return atespace + "/" + name
}

func clone[T any](v T) T {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		panic(err)
	}
	return out
}

type fileState struct {
	Atespaces []model.Atespace      `json:"atespaces"`
	Templates []model.ActorTemplate `json:"actorTemplates"`
	Actors    []model.Actor         `json:"actors"`
	Workers   []model.Worker        `json:"workers"`
}

func (s *Store) load() error {
	b, err := os.ReadFile(s.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var state fileState
	if err := json.Unmarshal(b, &state); err != nil {
		return fmt.Errorf("reading %s: %w", s.path, err)
	}
	for _, item := range state.Atespaces {
		s.atespaces[item.Metadata.Name] = item
	}
	for _, item := range state.Templates {
		s.templates[ref(item.Metadata.Atespace, item.Metadata.Name)] = item
	}
	for _, item := range state.Actors {
		s.actors[ref(item.Metadata.Atespace, item.Metadata.Name)] = item
	}
	for _, item := range state.Workers {
		s.workers[item.Metadata.Name] = item
	}
	return nil
}

func (s *Store) save() error {
	if s.path == "" {
		return nil
	}
	state := fileState{}
	for _, item := range s.atespaces {
		state.Atespaces = append(state.Atespaces, item)
	}
	for _, item := range s.templates {
		state.Templates = append(state.Templates, item)
	}
	for _, item := range s.actors {
		state.Actors = append(state.Actors, item)
	}
	for _, item := range s.workers {
		state.Workers = append(state.Workers, item)
	}
	b, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	tmp := s.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, s.path)
}

func (s *Store) seed() {
	const digest = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"
	created := now().Add(-48 * time.Hour)

	s.atespaces["prod"] = model.Atespace{
		Metadata:    metaAt("prod", "", created),
		Description: "Customer agents on the warm pool.",
	}
	s.atespaces["lab"] = model.Atespace{
		Metadata:    metaAt("lab", "", created.Add(time.Hour)),
		Description: "Experiments that can be thrown away.",
	}

	s.templates[ref("prod", "research-agent")] = model.ActorTemplate{
		Metadata: metaAt("research-agent", "prod", created.Add(2*time.Hour)),
		Containers: []model.Container{{
			Name:    "agent",
			Image:   "registry.example.com/agents/research@" + digest,
			Command: []string{"/agent"},
			Args:    []string{"--port=8080"},
			Env:     []model.EnvVar{{Name: "LOG_LEVEL", Value: "info"}},
		}},
		WorkerSelector: &model.Selector{MatchLabels: map[string]string{"sandbox": "gvisor", "tier": "1"}},
		Resources:      limits(2000, 4<<30),
		Status: model.ActorTemplateStatus{GoldenSnapshotStatus: &model.GoldenSnapshotStatus{
			GoldenTag:    &model.ObjectRef{Atespace: "prod", Name: "research-golden"},
			SnapshotTime: created.Add(3 * time.Hour),
		}},
	}
	s.templates[ref("prod", "ops-runner")] = model.ActorTemplate{
		Metadata: metaAt("ops-runner", "prod", created.Add(4*time.Hour)),
		Containers: []model.Container{{
			Name:  "runner",
			Image: "registry.example.com/agents/ops@" + digest,
			Args:  []string{"--role=ops"},
		}},
		WorkerSelector: &model.Selector{MatchLabels: map[string]string{"sandbox": "gvisor"}},
		Resources:      limits(1000, 2<<30),
	}
	s.templates[ref("lab", "sandbox-toy")] = model.ActorTemplate{
		Metadata: metaAt("sandbox-toy", "lab", created.Add(5*time.Hour)),
		Containers: []model.Container{{
			Name:  "toy",
			Image: "registry.example.com/agents/toy@" + digest,
		}},
		WorkerSelector: &model.Selector{MatchLabels: map[string]string{"sandbox": "microvm"}},
		Resources:      limits(500, 1<<30),
	}

	s.workers["worker-a7f3"] = worker("worker-a7f3", "substrate-workers", "general", "gvisor", "10.0.4.12", "node-a", model.WorkerActive, 8000, 16<<30, 1000, 3200, 6<<30, 2, created)
	s.workers["worker-b21c"] = worker("worker-b21c", "substrate-workers", "edge", "microvm", "10.0.4.18", "node-b", model.WorkerActive, 4000, 8<<30, 1000, 500, 1<<30, 1, created.Add(time.Hour))
	s.workers["worker-c90e"] = worker("worker-c90e", "substrate-workers", "general", "gvisor", "10.0.4.21", "node-c", model.WorkerDraining, 8000, 16<<30, 1000, 0, 0, 0, created.Add(2*time.Hour))

	s.actors[ref("prod", "atlas")] = actor("atlas", "prod", "research-agent", model.ActorRunning, &model.WorkerAssignment{
		Worker:          model.ObjectRef{Name: "worker-a7f3"},
		WorkerNamespace: "substrate-workers",
		WorkerPool:      "general",
		WorkerPod:       "worker-a7f3",
		NodeName:        "node-a",
		WorkerPodIPs:    []string{"10.0.4.12"},
		SandboxRuntime:  &model.ActorSandboxRuntime{SandboxClass: "gvisor"},
	}, nil, created.Add(6*time.Hour))
	s.actors[ref("prod", "nile")] = actor("nile", "prod", "ops-runner", model.ActorSuspended, nil, nil, created.Add(8*time.Hour))
	s.actors[ref("prod", "kepler")] = actor("kepler", "prod", "research-agent", model.ActorCrashed, nil, &model.ActorCrash{
		Message:   "wakeup probe timed out on :8080",
		CrashTime: created.Add(26 * time.Hour),
	}, created.Add(10*time.Hour))
	s.actors[ref("lab", "pebble")] = actor("pebble", "lab", "sandbox-toy", model.ActorRunning, &model.WorkerAssignment{
		Worker:          model.ObjectRef{Name: "worker-b21c"},
		WorkerNamespace: "substrate-workers",
		WorkerPool:      "edge",
		WorkerPod:       "worker-b21c",
		NodeName:        "node-b",
		WorkerPodIPs:    []string{"10.0.4.18"},
		SandboxRuntime:  &model.ActorSandboxRuntime{SandboxClass: "microvm"},
	}, nil, created.Add(12*time.Hour))
}

func metaAt(name, atespace string, created time.Time) model.ResourceMetadata {
	id := name
	if atespace != "" {
		id = atespace + "-" + name
	}
	return model.ResourceMetadata{
		Atespace:   atespace,
		Name:       name,
		UID:        "seed-" + id,
		Version:    3,
		CreateTime: created,
		UpdateTime: created.Add(30 * time.Minute),
	}
}

func limits(cpu, mem int64) *model.ResourceRequirements {
	return &model.ResourceRequirements{Limits: model.Resources{CPUMillicores: model.FlexInt(cpu), MemoryBytes: model.FlexInt(mem)}}
}

func worker(name, ns, pool, class, ip, node, state string, cpu, mem, slots, usedCPU, usedMem, usedActors int64, created time.Time) model.Worker {
	return model.Worker{
		Metadata:        metaAt(name, "", created),
		WorkerNamespace: ns,
		WorkerSpiffeID:  "spiffe://substrate.local/ns/" + ns + "/pod/" + name,
		Labels:          map[string]string{"sandbox": class, "pool": pool, "tier": "1"},
		Status: model.WorkerStatus{
			State:           state,
			Capacity:        model.WorkerResources{CPUMillicores: model.FlexInt(cpu), MemoryBytes: model.FlexInt(mem), Actors: model.FlexInt(slots)},
			Allocated:       model.WorkerResources{CPUMillicores: model.FlexInt(usedCPU), MemoryBytes: model.FlexInt(usedMem), Actors: model.FlexInt(usedActors)},
			SandboxClasses:  []model.WorkerSandboxClass{{SandboxClass: class}},
			IPs:             []string{ip},
			RegisteredEpoch: 1,
		},
	}
}

func actor(name, atespace, template, state string, assign *model.WorkerAssignment, crash *model.ActorCrash, created time.Time) model.Actor {
	a := model.Actor{
		Metadata:      metaAt(name, atespace, created),
		ActorTemplate: &model.ObjectRef{Atespace: atespace, Name: template},
		Status:        model.ActorStatus{State: state, WorkerAssignment: assign, Crash: crash},
	}
	if assign != nil {
		a.Status.AssignedNode = assign.NodeName
	}
	return a
}
