package memory

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/actions"
)

func (s *Store) RouteActor(_ context.Context, atespace, name string, req model.RouteRequest) (model.RouteSession, error) {
	req = actions.NormalizeRoute(req)
	s.mu.Lock()
	defer s.mu.Unlock()
	actor, ok := s.actors[ref(atespace, name)]
	if !ok {
		return model.RouteSession{}, substrate.NotFound("actor %s not found", ref(atespace, name))
	}
	if s.routes == nil {
		s.routes = map[string]model.RouteSession{}
	}
	id := actions.SessionID(atespace, name)
	if existing, ok := s.routes[id]; ok {
		return existing, nil
	}
	port := req.LocalPort
	if port <= 0 {
		port = 18000 + (len(s.routes) % 1000)
	}
	session := model.RouteSession{
		ID:         id,
		Atespace:   atespace,
		Actor:      name,
		LocalPort:  port,
		RemotePort: req.RemotePort,
		LocalURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
		Status:     "ready",
		Detail:     "demo route; Execute simulates the actor dataplane",
		CreatedAt:  now(),
	}
	_ = actor
	s.routes[id] = session
	return session, nil
}

func (s *Store) GetActorRoute(_ context.Context, atespace, name string) (model.RouteSession, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.routes[actions.SessionID(atespace, name)]
	if !ok {
		return model.RouteSession{}, substrate.NotFound("no active route for actor %s", ref(atespace, name))
	}
	return session, nil
}

func (s *Store) StopActorRoute(_ context.Context, atespace, name string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	id := actions.SessionID(atespace, name)
	if _, ok := s.routes[id]; !ok {
		return substrate.NotFound("no active route for actor %s", ref(atespace, name))
	}
	delete(s.routes, id)
	return nil
}

func (s *Store) ExecuteActor(ctx context.Context, atespace, name string, req model.ExecuteRequest) (model.ExecuteResult, error) {
	req = actions.NormalizeExecute(req)
	start := time.Now()
	s.mu.Lock()
	actor, ok := s.actors[ref(atespace, name)]
	if !ok {
		s.mu.Unlock()
		return model.ExecuteResult{}, substrate.NotFound("actor %s not found", ref(atespace, name))
	}
	if s.counters == nil {
		s.counters = map[string]int{}
	}
	id := actions.SessionID(atespace, name)
	if _, routed := s.routes[id]; !routed {
		port := 18000 + (len(s.routes) % 1000)
		if s.routes == nil {
			s.routes = map[string]model.RouteSession{}
		}
		s.routes[id] = model.RouteSession{
			ID:         id,
			Atespace:   atespace,
			Actor:      name,
			LocalPort:  port,
			RemotePort: req.RemotePort,
			LocalURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
			Status:     "ready",
			Detail:     "auto-opened for Execute",
			CreatedAt:  now(),
		}
	}
	session := s.routes[id]
	count := s.counters[id]
	method := req.Method
	if method == http.MethodPost || method == http.MethodPut || method == http.MethodPatch {
		count++
		s.counters[id] = count
	} else {
		count = s.counters[id]
	}
	if actor.Status.State != model.ActorRunning {
		actor.Status.State = model.ActorRunning
		s.actors[ref(atespace, name)] = actor
	}
	s.mu.Unlock()

	body := fmt.Sprintf(`{"actor":%q,"atespace":%q,"path":%q,"method":%q,"count":%d,"message":"demo execute completed"}`,
		name, atespace, req.Path, method, count)
	_ = ctx
	return model.ExecuteResult{
		Status:    http.StatusOK,
		Headers:   map[string]string{"Content-Type": "application/json", "X-Ate-Demo": "true"},
		Body:      body,
		LatencyMs: time.Since(start).Milliseconds(),
		LocalURL:  session.LocalURL + req.Path,
		RouteID:   session.ID,
	}, nil
}

func (s *Store) CommandActor(ctx context.Context, atespace, name string, req model.CommandRequest) (model.CommandResult, error) {
	s.mu.Lock()
	actor, ok := s.actors[ref(atespace, name)]
	if !ok {
		s.mu.Unlock()
		return model.CommandResult{}, substrate.NotFound("actor %s not found", ref(atespace, name))
	}
	id := actions.SessionID(atespace, name)
	if _, routed := s.routes[id]; !routed {
		port := 18000 + (len(s.routes) % 1000)
		if s.routes == nil {
			s.routes = map[string]model.RouteSession{}
		}
		s.routes[id] = model.RouteSession{
			ID:         id,
			Atespace:   atespace,
			Actor:      name,
			LocalPort:  port,
			RemotePort: req.RemotePort,
			LocalURL:   fmt.Sprintf("http://127.0.0.1:%d", port),
			Status:     "ready",
			Detail:     "auto-opened for Command",
			CreatedAt:  now(),
		}
	}
	session := s.routes[id]
	if actor.Status.State != model.ActorRunning {
		actor.Status.State = model.ActorRunning
		s.actors[ref(atespace, name)] = actor
	}
	s.mu.Unlock()

	result, err := actions.RunLocalCommand(ctx, req)
	if err != nil {
		return model.CommandResult{}, err
	}
	result.RouteID = session.ID
	result.LocalURL = session.LocalURL + "/process"
	return result, nil
}
