package httpapi

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/registry"
)

func newTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	reg, err := registry.Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return httptest.NewServer(New(reg))
}

func TestResourceLifecycle(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	var overview model.Overview
	getJSON(t, srv.URL+"/api/v1/clusters/local/overview", &overview)
	if overview.Counts.Atespaces != 2 || overview.Counts.Actors != 4 || overview.Counts.Workers != 3 || overview.Counts.ActorTemplates != 3 {
		t.Fatalf("seed counts = %+v", overview.Counts)
	}
	if overview.Cluster.HasToken {
		t.Fatal("local cluster should not report a token")
	}

	status, _ := doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/atespaces", map[string]any{
		"metadata": map[string]string{"name": "Not_Valid"},
	})
	if status != http.StatusBadRequest {
		t.Fatalf("invalid name status = %d", status)
	}

	var space model.Atespace
	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/atespaces", map[string]any{
		"metadata":    map[string]string{"name": "field"},
		"description": "Field trials",
	})
	if status != http.StatusCreated {
		t.Fatalf("create atespace %d %s", status, body)
	}
	mustDecode(t, body, &space)
	if space.Metadata.UID == "" || space.Metadata.Version != 1 {
		t.Fatalf("metadata not assigned: %+v", space.Metadata)
	}

	space.Description = "Field trials, west"
	status, body = doJSON(t, http.MethodPut, srv.URL+"/api/v1/clusters/local/atespaces/field", space)
	if status != http.StatusOK {
		t.Fatalf("update atespace %d %s", status, body)
	}
	mustDecode(t, body, &space)
	if space.Metadata.Version != 2 || space.Description != "Field trials, west" {
		t.Fatalf("update did not apply: %+v", space)
	}

	var template model.ActorTemplate
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/actor-templates", map[string]any{
		"metadata": map[string]string{"atespace": "field", "name": "scout"},
		"containers": []map[string]string{{
			"name":  "agent",
			"image": "registry.example.com/agents/scout@sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08",
		}},
		"resources": map[string]any{"limits": map[string]int64{"cpuMillicores": 250, "memoryBytes": 536870912}},
	})
	if status != http.StatusCreated {
		t.Fatalf("create template %d %s", status, body)
	}
	mustDecode(t, body, &template)

	template.Resources.Limits.CPUMillicores = 500
	status, body = doJSON(t, http.MethodPut, srv.URL+"/api/v1/clusters/local/actor-templates/field/scout", template)
	if status != http.StatusOK {
		t.Fatalf("update template %d %s", status, body)
	}
	mustDecode(t, body, &template)
	if template.Resources.Limits.CPUMillicores != 500 || template.Metadata.Version != 2 {
		t.Fatalf("template update = %+v", template)
	}

	var actor model.Actor
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/actors", map[string]any{
		"metadata":      map[string]string{"atespace": "field", "name": "oriole"},
		"actorTemplate": map[string]string{"name": "scout"},
	})
	if status != http.StatusCreated {
		t.Fatalf("create actor %d %s", status, body)
	}
	mustDecode(t, body, &actor)
	if actor.Status.State != model.ActorSuspended || actor.ActorTemplate.Atespace != "field" {
		t.Fatalf("actor defaults = %+v", actor)
	}

	stale := actor
	stale.Metadata.Version = 99
	status, _ = doJSON(t, http.MethodPut, srv.URL+"/api/v1/clusters/local/actors/field/oriole", stale)
	if status != http.StatusConflict {
		t.Fatalf("stale update status = %d", status)
	}

	actor.WorkerSelector = &model.Selector{MatchLabels: map[string]string{"tier": "1"}}
	status, body = doJSON(t, http.MethodPut, srv.URL+"/api/v1/clusters/local/actors/field/oriole", actor)
	if status != http.StatusOK {
		t.Fatalf("update actor %d %s", status, body)
	}
	mustDecode(t, body, &actor)
	if actor.WorkerSelector.MatchLabels["tier"] != "1" || actor.Metadata.Version != 2 {
		t.Fatalf("actor update = %+v", actor)
	}

	var worker model.Worker
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/workers", map[string]any{
		"metadata":        map[string]string{"name": "worker-field"},
		"workerNamespace": "substrate-workers",
		"labels":          map[string]string{"pool": "field"},
		"status":          map[string]any{"state": model.WorkerActive},
	})
	if status != http.StatusCreated {
		t.Fatalf("create worker %d %s", status, body)
	}
	mustDecode(t, body, &worker)

	worker.Labels = map[string]string{"pool": "field", "sandbox": "gvisor"}
	status, body = doJSON(t, http.MethodPut, srv.URL+"/api/v1/clusters/local/workers/worker-field", worker)
	if status != http.StatusOK {
		t.Fatalf("update worker %d %s", status, body)
	}
	mustDecode(t, body, &worker)
	if worker.Labels["sandbox"] != "gvisor" {
		t.Fatalf("labels = %+v", worker.Labels)
	}

	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/atespaces/field?version=2&uid="+space.Metadata.UID, nil)
	if status != http.StatusConflict {
		t.Fatalf("delete busy atespace status = %d", status)
	}

	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/actors/field/oriole?version=2&uid="+actor.Metadata.UID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete actor status = %d", status)
	}
	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/actor-templates/field/scout?version=2&uid="+template.Metadata.UID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete template status = %d", status)
	}
	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/atespaces/field?version=2&uid="+space.Metadata.UID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete atespace status = %d", status)
	}
	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/workers/worker-field?version=2&uid="+worker.Metadata.UID, nil)
	if status != http.StatusOK {
		t.Fatalf("delete worker status = %d", status)
	}

	status, _ = doJSON(t, http.MethodGet, srv.URL+"/api/v1/clusters/local/actors/field/oriole", nil)
	if status != http.StatusNotFound {
		t.Fatalf("deleted actor status = %d", status)
	}
}

func TestActorRouteAndExecute(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/actors/prod/atlas/route", map[string]any{
		"remotePort": 80,
	})
	if status != http.StatusOK {
		t.Fatalf("route %d %s", status, body)
	}
	var session model.RouteSession
	mustDecode(t, body, &session)
	if session.LocalURL == "" || session.Status != "ready" {
		t.Fatalf("session = %+v", session)
	}

	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/actors/prod/atlas/execute", map[string]any{
		"method": "POST",
		"path":   "/",
	})
	if status != http.StatusOK {
		t.Fatalf("execute %d %s", status, body)
	}
	var result model.ExecuteResult
	mustDecode(t, body, &result)
	if result.Status != http.StatusOK || result.Body == "" {
		t.Fatalf("result = %+v", result)
	}

	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/actors/prod/atlas/command", map[string]any{
		"shell": "echo hello-from-command && printf world",
	})
	if status != http.StatusOK {
		t.Fatalf("command %d %s", status, body)
	}
	var cmd model.CommandResult
	mustDecode(t, body, &cmd)
	if cmd.ExitCode != 0 || !strings.Contains(cmd.Stdout, "hello-from-command") {
		t.Fatalf("command result = %+v", cmd)
	}

	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local/actors/prod/atlas/route", nil)
	if status != http.StatusNoContent {
		t.Fatalf("stop route status = %d", status)
	}
}

func TestClusters(t *testing.T) {
	srv := newTestServer(t)
	defer srv.Close()

	status, body := doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters", map[string]any{
		"id":       "edge",
		"name":     "Edge",
		"driver":   "grpc",
		"endpoint": "127.0.0.1:1",
		"token":    "secret-token",
		"tls":      false,
	})
	if status != http.StatusCreated {
		t.Fatalf("create cluster %d %s", status, body)
	}
	var pub model.PublicCluster
	mustDecode(t, body, &pub)
	if pub.HasToken != true || pub.Endpoint != "127.0.0.1:1" {
		t.Fatalf("public cluster = %+v", pub)
	}
	if bytes.Contains(body, []byte("secret-token")) {
		t.Fatal("token leaked in response")
	}

	status, _ = doJSON(t, http.MethodDelete, srv.URL+"/api/v1/clusters/local", nil)
	if status != http.StatusConflict {
		t.Fatalf("delete local status = %d", status)
	}

	var probe model.Probe
	status, body = doJSON(t, http.MethodPost, srv.URL+"/api/v1/clusters/local/probe", map[string]any{})
	if status != http.StatusOK {
		t.Fatalf("probe %d %s", status, body)
	}
	mustDecode(t, body, &probe)
	if !probe.OK {
		t.Fatalf("local probe = %+v", probe)
	}
}

func doJSON(t *testing.T, method, url string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, url, reader)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", "application/json")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer res.Body.Close()
	out, err := io.ReadAll(res.Body)
	if err != nil {
		t.Fatal(err)
	}
	return res.StatusCode, out
}

func getJSON(t *testing.T, url string, dst any) {
	t.Helper()
	status, body := doJSON(t, http.MethodGet, url, nil)
	if status != http.StatusOK {
		t.Fatalf("GET %s -> %d %s", url, status, body)
	}
	mustDecode(t, body, dst)
}

func mustDecode(t *testing.T, body []byte, dst any) {
	t.Helper()
	if err := json.Unmarshal(body, dst); err != nil {
		t.Fatalf("decode %s: %v", body, err)
	}
}
