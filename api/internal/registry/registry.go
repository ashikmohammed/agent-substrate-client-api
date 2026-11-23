// Package registry keeps the clusters this process can talk to.
package registry

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"sync"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/kube"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/memory"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/remote"
)

const localID = "local"

// Registry is the set of named clusters.
type Registry struct {
	mu    sync.Mutex
	dir   string
	path  string
	items map[string]*entry
}

type entry struct {
	cfg    model.Cluster
	driver substrate.Cluster
	closer interface{ Close() error }
}

type fileShape struct {
	Clusters []model.Cluster `json:"clusters"`
}

// Open loads clusters from dir/clusters.json, creating the local demo if needed,
// and auto-registers a kubeconfig-backed Substrate cluster when one is reachable.
func Open(dir string) (*Registry, error) {
	if dir == "" {
		dir = "."
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	r := &Registry{
		dir:   dir,
		path:  filepath.Join(dir, "clusters.json"),
		items: map[string]*entry{},
	}
	if err := r.load(); err != nil {
		return nil, err
	}
	if _, ok := r.items[localID]; !ok {
		if err := r.add(model.Cluster{ID: localID, Name: "Local demo", Driver: "memory"}, true); err != nil {
			return nil, err
		}
	}
	r.ensureKubeCluster()
	return r, nil
}

func (r *Registry) ensureKubeCluster() {
	hasKube := false
	for _, e := range r.items {
		if e.cfg.Driver == "kube" {
			hasKube = true
			break
		}
	}
	if hasKube {
		return
	}
	detected, err := kube.Detect(os.Getenv("KUBECONFIG"), os.Getenv("KUBE_CONTEXT"))
	if err != nil {
		log.Printf("no live Substrate cluster on kubeconfig: %v", err)
		return
	}
	if _, exists := r.items[detected.ID]; exists {
		detected.ID = "kube"
	}
	if err := r.add(detected, false); err != nil {
		log.Printf("could not register kube cluster %q: %v", detected.ID, err)
		return
	}
	log.Printf("registered live Substrate cluster %q from kubeconfig context %q", detected.ID, detected.KubeContext)
}

// DefaultID prefers a live kube cluster over the in-memory demo.
func (r *Registry) DefaultID() string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, e := range r.items {
		if e.cfg.Driver == "kube" {
			return e.cfg.ID
		}
	}
	for _, e := range r.items {
		if e.cfg.Driver == "grpc" {
			return e.cfg.ID
		}
	}
	return localID
}

// List returns clusters with credentials removed.
func (r *Registry) List() []model.PublicCluster {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]model.PublicCluster, 0, len(r.items))
	for _, e := range r.items {
		out = append(out, e.cfg.Public())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

// Get returns one cluster with the credential removed.
func (r *Registry) Get(id string) (model.PublicCluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	if !ok {
		return model.PublicCluster{}, substrate.NotFound("cluster %q not found", id)
	}
	return e.cfg.Public(), nil
}

// Cluster returns the driver for id.
func (r *Registry) Cluster(id string) (substrate.Cluster, model.PublicCluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	e, ok := r.items[id]
	if !ok {
		return nil, model.PublicCluster{}, substrate.NotFound("cluster %q not found", id)
	}
	return e.driver, e.cfg.Public(), nil
}

// Create adds a cluster and connects it.
func (r *Registry) Create(cfg model.Cluster) (model.PublicCluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.items[cfg.ID]; ok {
		return model.PublicCluster{}, substrate.AlreadyExists("cluster %q already exists", cfg.ID)
	}
	if err := r.add(cfg, false); err != nil {
		return model.PublicCluster{}, err
	}
	return r.items[cfg.ID].cfg.Public(), nil
}

// Update replaces connection settings for an existing cluster.
func (r *Registry) Update(cfg model.Cluster) (model.PublicCluster, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	cur, ok := r.items[cfg.ID]
	if !ok {
		return model.PublicCluster{}, substrate.NotFound("cluster %q not found", cfg.ID)
	}
	if cfg.ID == localID {
		return model.PublicCluster{}, substrate.FailedPrecondition("the local demo cluster cannot be reconfigured")
	}
	if cfg.Token == "" {
		cfg.Token = cur.cfg.Token
	}
	if cfg.CACertPEM == "" {
		cfg.CACertPEM = cur.cfg.CACertPEM
	}
	if cfg.Driver == "" {
		cfg.Driver = cur.cfg.Driver
	}
	next, err := build(r.dir, cfg, false)
	if err != nil {
		return model.PublicCluster{}, err
	}
	if cur.closer != nil {
		_ = cur.closer.Close()
	}
	r.items[cfg.ID] = next
	if err := r.save(); err != nil {
		return model.PublicCluster{}, err
	}
	return next.cfg.Public(), nil
}

// Delete removes a cluster. The local demo stays.
func (r *Registry) Delete(id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if id == localID {
		return substrate.FailedPrecondition("the local demo cluster cannot be deleted")
	}
	cur, ok := r.items[id]
	if !ok {
		return substrate.NotFound("cluster %q not found", id)
	}
	if cur.closer != nil {
		_ = cur.closer.Close()
	}
	delete(r.items, id)
	if cur.cfg.Driver == "memory" {
		_ = os.Remove(filepath.Join(r.dir, "memory-"+id+".json"))
	}
	return r.save()
}

func (r *Registry) add(cfg model.Cluster, seeded bool) error {
	e, err := build(r.dir, cfg, seeded)
	if err != nil {
		return err
	}
	r.items[cfg.ID] = e
	return r.save()
}

func (r *Registry) load() error {
	b, err := os.ReadFile(r.path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	var shape fileShape
	if err := json.Unmarshal(b, &shape); err != nil {
		return fmt.Errorf("reading clusters: %w", err)
	}
	for _, cfg := range shape.Clusters {
		e, err := build(r.dir, cfg, cfg.ID == localID)
		if err != nil {
			log.Printf("skipping cluster %q: %v", cfg.ID, err)
			continue
		}
		r.items[cfg.ID] = e
	}
	return nil
}

func (r *Registry) save() error {
	shape := fileShape{}
	for _, e := range r.items {
		saved := e.cfg
		if saved.Driver == "kube" {
			// Runtime credentials are minted from kubeconfig; do not persist them.
			saved.Token = ""
			saved.CACertPEM = ""
			saved.Endpoint = ""
		}
		shape.Clusters = append(shape.Clusters, saved)
	}
	sort.Slice(shape.Clusters, func(i, j int) bool { return shape.Clusters[i].ID < shape.Clusters[j].ID })
	b, err := json.MarshalIndent(shape, "", "  ")
	if err != nil {
		return err
	}
	tmp := r.path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, r.path)
}

var idPattern = regexp.MustCompile(`^[a-z0-9]([-a-z0-9]{0,61}[a-z0-9])?$`)

func build(dir string, cfg model.Cluster, seed bool) (*entry, error) {
	if !idPattern.MatchString(cfg.ID) {
		return nil, substrate.Invalid("cluster id must be a DNS label")
	}
	if cfg.Name == "" {
		cfg.Name = cfg.ID
	}
	switch cfg.Driver {
	case "memory":
		store, err := memory.New(filepath.Join(dir, "memory-"+cfg.ID+".json"), seed)
		if err != nil {
			return nil, err
		}
		cfg.Endpoint = ""
		cfg.Token = ""
		cfg.TLS = false
		return &entry{cfg: cfg, driver: store}, nil
	case "grpc":
		if cfg.Endpoint == "" {
			return nil, substrate.Invalid("endpoint is required for a grpc cluster")
		}
		driver := remote.New(cfg)
		return &entry{cfg: cfg, driver: driver, closer: driver}, nil
	case "kube":
		driver, err := kube.New(cfg)
		if err != nil {
			return nil, err
		}
		return &entry{cfg: cfg, driver: driver, closer: driver}, nil
	default:
		return nil, substrate.Invalid("driver must be memory, grpc, or kube")
	}
}
