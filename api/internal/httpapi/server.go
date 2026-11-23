// Package httpapi is the JSON API in front of one or more Substrate clusters.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/registry"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
)

// Server serves AgentSubstrateAPI.
type Server struct {
	reg *registry.Registry
}

// New builds the HTTP handler.
func New(reg *registry.Registry) http.Handler {
	s := &Server{reg: reg}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"name":           "AgentSubstrateAPI",
			"version":        "0.1.0",
			"defaultCluster": s.reg.DefaultID(),
			"resources":      []string{"atespace", "actorTemplate", "actor", "worker"},
		})
	})

	mux.HandleFunc("GET /api/v1/clusters", s.listClusters)
	mux.HandleFunc("POST /api/v1/clusters", s.createCluster)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}", s.getCluster)
	mux.HandleFunc("PUT /api/v1/clusters/{cluster}", s.updateCluster)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}", s.deleteCluster)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/probe", s.probe)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/overview", s.overview)

	mux.HandleFunc("GET /api/v1/clusters/{cluster}/atespaces", s.listAtespaces)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/atespaces", s.createAtespace)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/atespaces/{name}", s.getAtespace)
	mux.HandleFunc("PUT /api/v1/clusters/{cluster}/atespaces/{name}", s.updateAtespace)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}/atespaces/{name}", s.deleteAtespace)

	mux.HandleFunc("GET /api/v1/clusters/{cluster}/actor-templates", s.listTemplates)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/actor-templates", s.createTemplate)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/actor-templates/{atespace}/{name}", s.getTemplate)
	mux.HandleFunc("PUT /api/v1/clusters/{cluster}/actor-templates/{atespace}/{name}", s.updateTemplate)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}/actor-templates/{atespace}/{name}", s.deleteTemplate)

	mux.HandleFunc("GET /api/v1/clusters/{cluster}/actors", s.listActors)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/actors", s.createActor)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/actors/{atespace}/{name}", s.getActor)
	mux.HandleFunc("PUT /api/v1/clusters/{cluster}/actors/{atespace}/{name}", s.updateActor)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}/actors/{atespace}/{name}", s.deleteActor)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/actors/{atespace}/{name}/route", s.routeActor)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/actors/{atespace}/{name}/route", s.getActorRoute)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}/actors/{atespace}/{name}/route", s.stopActorRoute)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/actors/{atespace}/{name}/execute", s.executeActor)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/actors/{atespace}/{name}/command", s.commandActor)

	mux.HandleFunc("GET /api/v1/clusters/{cluster}/workers", s.listWorkers)
	mux.HandleFunc("POST /api/v1/clusters/{cluster}/workers", s.createWorker)
	mux.HandleFunc("GET /api/v1/clusters/{cluster}/workers/{name}", s.getWorker)
	mux.HandleFunc("PUT /api/v1/clusters/{cluster}/workers/{name}", s.updateWorker)
	mux.HandleFunc("DELETE /api/v1/clusters/{cluster}/workers/{name}", s.deleteWorker)

	return withCORS(mux)
}

func (s *Server) cluster(r *http.Request) (substrate.Cluster, model.PublicCluster, error) {
	return s.reg.Cluster(r.PathValue("cluster"))
}

func (s *Server) listClusters(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"items":   s.reg.List(),
		"default": s.reg.DefaultID(),
	})
}

func (s *Server) createCluster(w http.ResponseWriter, r *http.Request) {
	var cfg model.Cluster
	if err := readJSON(w, r, &cfg); err != nil {
		writeErr(w, err)
		return
	}
	item, err := s.reg.Create(cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, item)
}

func (s *Server) getCluster(w http.ResponseWriter, r *http.Request) {
	item, err := s.reg.Get(r.PathValue("cluster"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateCluster(w http.ResponseWriter, r *http.Request) {
	var cfg model.Cluster
	if err := readJSON(w, r, &cfg); err != nil {
		writeErr(w, err)
		return
	}
	cfg.ID = r.PathValue("cluster")
	item, err := s.reg.Update(cfg)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) deleteCluster(w http.ResponseWriter, r *http.Request) {
	if err := s.reg.Delete(r.PathValue("cluster")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) probe(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	result, err := c.Probe(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request) {
	c, pub, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	atespaces, err := c.ListAtespaces(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	templates, err := c.ListActorTemplates(r.Context(), "")
	if err != nil {
		writeErr(w, err)
		return
	}
	actors, err := c.ListActors(r.Context(), "")
	if err != nil {
		writeErr(w, err)
		return
	}
	workers, err := c.ListWorkers(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	byState := map[string]int{}
	for _, a := range actors {
		state := a.Status.State
		if state == "" {
			state = model.ActorUnspecified
		}
		byState[state]++
	}
	writeJSON(w, http.StatusOK, model.Overview{
		Cluster: pub,
		Counts: model.Counts{
			Atespaces:      len(atespaces),
			ActorTemplates: len(templates),
			Actors:         len(actors),
			Workers:        len(workers),
		},
		ActorsByState: byState,
		Workers:       workers,
		Atespaces:     atespaces,
	})
}

func (s *Server) listAtespaces(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, err := c.ListAtespaces(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createAtespace(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Atespace
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	created, err := c.CreateAtespace(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getAtespace(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.GetAtespace(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateAtespace(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Atespace
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	item.Metadata.Name = r.PathValue("name")
	item.Metadata.Atespace = ""
	updated, err := c.UpdateAtespace(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteAtespace(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.DeleteAtespace(r.Context(), r.PathValue("name"), preconditions(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listTemplates(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, err := c.ListActorTemplates(r.Context(), r.URL.Query().Get("atespace"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createTemplate(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.ActorTemplate
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	created, err := c.CreateActorTemplate(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getTemplate(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.GetActorTemplate(r.Context(), r.PathValue("atespace"), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateTemplate(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.ActorTemplate
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	item.Metadata.Atespace = r.PathValue("atespace")
	item.Metadata.Name = r.PathValue("name")
	updated, err := c.UpdateActorTemplate(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteTemplate(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.DeleteActorTemplate(r.Context(), r.PathValue("atespace"), r.PathValue("name"), preconditions(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) listActors(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, err := c.ListActors(r.Context(), r.URL.Query().Get("atespace"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createActor(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Actor
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	created, err := c.CreateActor(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getActor(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.GetActor(r.Context(), r.PathValue("atespace"), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateActor(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Actor
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	item.Metadata.Atespace = r.PathValue("atespace")
	item.Metadata.Name = r.PathValue("name")
	updated, err := c.UpdateActor(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteActor(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.DeleteActor(r.Context(), r.PathValue("atespace"), r.PathValue("name"), preconditions(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) actorActions(r *http.Request) (substrate.ActorActions, error) {
	c, _, err := s.cluster(r)
	if err != nil {
		return nil, err
	}
	actions, ok := c.(substrate.ActorActions)
	if !ok {
		return nil, substrate.Unimplemented("this cluster driver cannot route or execute against actors")
	}
	return actions, nil
}

func (s *Server) routeActor(w http.ResponseWriter, r *http.Request) {
	actions, err := s.actorActions(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req model.RouteRequest
	if r.ContentLength != 0 {
		if err := readJSON(w, r, &req); err != nil {
			writeErr(w, err)
			return
		}
	}
	session, err := actions.RouteActor(r.Context(), r.PathValue("atespace"), r.PathValue("name"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) getActorRoute(w http.ResponseWriter, r *http.Request) {
	actions, err := s.actorActions(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	session, err := actions.GetActorRoute(r.Context(), r.PathValue("atespace"), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, session)
}

func (s *Server) stopActorRoute(w http.ResponseWriter, r *http.Request) {
	actions, err := s.actorActions(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	if err := actions.StopActorRoute(r.Context(), r.PathValue("atespace"), r.PathValue("name")); err != nil {
		writeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) executeActor(w http.ResponseWriter, r *http.Request) {
	actions, err := s.actorActions(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req model.ExecuteRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	result, err := actions.ExecuteActor(r.Context(), r.PathValue("atespace"), r.PathValue("name"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) commandActor(w http.ResponseWriter, r *http.Request) {
	actions, err := s.actorActions(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var req model.CommandRequest
	if err := readJSON(w, r, &req); err != nil {
		writeErr(w, err)
		return
	}
	result, err := actions.CommandActor(r.Context(), r.PathValue("atespace"), r.PathValue("name"), req)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (s *Server) listWorkers(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	items, err := c.ListWorkers(r.Context())
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": items})
}

func (s *Server) createWorker(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Worker
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	created, err := c.CreateWorker(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) getWorker(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.GetWorker(r.Context(), r.PathValue("name"))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func (s *Server) updateWorker(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	var item model.Worker
	if err := readJSON(w, r, &item); err != nil {
		writeErr(w, err)
		return
	}
	item.Metadata.Name = r.PathValue("name")
	item.Metadata.Atespace = ""
	updated, err := c.UpdateWorker(r.Context(), item)
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}

func (s *Server) deleteWorker(w http.ResponseWriter, r *http.Request) {
	c, _, err := s.cluster(r)
	if err != nil {
		writeErr(w, err)
		return
	}
	item, err := c.DeleteWorker(r.Context(), r.PathValue("name"), preconditions(r))
	if err != nil {
		writeErr(w, err)
		return
	}
	writeJSON(w, http.StatusOK, item)
}

func preconditions(r *http.Request) model.Preconditions {
	pre := model.Preconditions{UID: r.URL.Query().Get("uid")}
	if raw := r.URL.Query().Get("version"); raw != "" {
		if n, err := strconv.ParseInt(raw, 10, 64); err == nil {
			pre.Version = n
		}
	}
	return pre
}

func readJSON(w http.ResponseWriter, r *http.Request, dst any) error {
	defer r.Body.Close()
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20))
	if err := dec.Decode(dst); err != nil {
		return substrate.Invalid("invalid JSON: %s", err.Error())
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(body)
}

func writeErr(w http.ResponseWriter, err error) {
	if err == nil {
		return
	}
	var max *http.MaxBytesError
	if errors.As(err, &max) {
		err = substrate.Invalid("request body is too large")
	}
	status := http.StatusInternalServerError
	code := "internal"
	if se, ok := substrate.As(err); ok {
		status = se.StatusCode()
		code = string(se.Code)
	} else if errors.Is(err, io.EOF) {
		status = http.StatusBadRequest
		code = string(substrate.InvalidCode)
		err = substrate.Invalid("request body is required")
	}
	writeJSON(w, status, map[string]any{
		"error": map[string]string{"code": code, "message": err.Error()},
	})
}

func withCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		next.ServeHTTP(w, r)
	})
}
