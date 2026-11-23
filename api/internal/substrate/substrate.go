// Package substrate is the extension point for talking to a cluster.
//
// AgentSubstrateAPI does not hard-wire a single installation. Anything that
// implements Cluster can back the HTTP API: the in-memory demo, a gRPC
// control plane discovered at dial time, or a future driver (a recorded
// fixture, a port-forward helper, another substrate version).
package substrate

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
)

// Cluster is the control-plane surface the console needs.
// Implementations must be safe for concurrent use.
type Cluster interface {
	// Probe reports whether the control plane answered.
	Probe(ctx context.Context) (model.Probe, error)

	CreateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error)
	GetAtespace(ctx context.Context, name string) (model.Atespace, error)
	ListAtespaces(ctx context.Context) ([]model.Atespace, error)
	UpdateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error)
	DeleteAtespace(ctx context.Context, name string, pre model.Preconditions) (model.Atespace, error)

	CreateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error)
	GetActorTemplate(ctx context.Context, atespace, name string) (model.ActorTemplate, error)
	ListActorTemplates(ctx context.Context, atespace string) ([]model.ActorTemplate, error)
	UpdateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error)
	DeleteActorTemplate(ctx context.Context, atespace, name string, pre model.Preconditions) (model.ActorTemplate, error)

	CreateActor(ctx context.Context, item model.Actor) (model.Actor, error)
	GetActor(ctx context.Context, atespace, name string) (model.Actor, error)
	ListActors(ctx context.Context, atespace string) ([]model.Actor, error)
	UpdateActor(ctx context.Context, item model.Actor) (model.Actor, error)
	DeleteActor(ctx context.Context, atespace, name string, pre model.Preconditions) (model.Actor, error)

	CreateWorker(ctx context.Context, item model.Worker) (model.Worker, error)
	GetWorker(ctx context.Context, name string) (model.Worker, error)
	ListWorkers(ctx context.Context) ([]model.Worker, error)
	UpdateWorker(ctx context.Context, item model.Worker) (model.Worker, error)
	DeleteWorker(ctx context.Context, name string, pre model.Preconditions) (model.Worker, error)
}

// ActorActions opens a local route into an actor and executes HTTP / commands against it.
// Drivers that cannot reach actor dataplane return Unimplemented.
type ActorActions interface {
	RouteActor(ctx context.Context, atespace, name string, req model.RouteRequest) (model.RouteSession, error)
	GetActorRoute(ctx context.Context, atespace, name string) (model.RouteSession, error)
	StopActorRoute(ctx context.Context, atespace, name string) error
	ExecuteActor(ctx context.Context, atespace, name string, req model.ExecuteRequest) (model.ExecuteResult, error)
	CommandActor(ctx context.Context, atespace, name string, req model.CommandRequest) (model.CommandResult, error)
}

// Code is a stable error class the HTTP layer maps to a status.
type Code string

const (
	NotFoundCode           Code = "not_found"
	AlreadyExistsCode      Code = "already_exists"
	InvalidCode            Code = "invalid_argument"
	AbortedCode            Code = "aborted"
	FailedPreconditionCode Code = "failed_precondition"
	UnimplementedCode      Code = "unimplemented"
	UnavailableCode        Code = "unavailable"
	UnauthenticatedCode    Code = "unauthenticated"
	PermissionDeniedCode   Code = "permission_denied"
)

// Error is a classified failure from a cluster driver.
type Error struct {
	Code Code
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

// StatusCode is the HTTP status for this class.
func (e *Error) StatusCode() int {
	switch e.Code {
	case NotFoundCode:
		return http.StatusNotFound
	case AlreadyExistsCode, AbortedCode, FailedPreconditionCode:
		return http.StatusConflict
	case InvalidCode:
		return http.StatusBadRequest
	case UnimplementedCode:
		return http.StatusNotImplemented
	case UnavailableCode:
		return http.StatusServiceUnavailable
	case UnauthenticatedCode:
		return http.StatusUnauthorized
	case PermissionDeniedCode:
		return http.StatusForbidden
	default:
		return http.StatusInternalServerError
	}
}

func classify(code Code, format string, args ...any) error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...)}
}

func NotFound(format string, args ...any) error { return classify(NotFoundCode, format, args...) }
func AlreadyExists(format string, args ...any) error {
	return classify(AlreadyExistsCode, format, args...)
}
func Invalid(format string, args ...any) error { return classify(InvalidCode, format, args...) }
func Aborted(format string, args ...any) error { return classify(AbortedCode, format, args...) }
func FailedPrecondition(format string, args ...any) error {
	return classify(FailedPreconditionCode, format, args...)
}
func Unimplemented(format string, args ...any) error {
	return classify(UnimplementedCode, format, args...)
}
func Unavailable(format string, args ...any) error {
	return classify(UnavailableCode, format, args...)
}
func Unauthenticated(format string, args ...any) error {
	return classify(UnauthenticatedCode, format, args...)
}
func PermissionDenied(format string, args ...any) error {
	return classify(PermissionDeniedCode, format, args...)
}

// As reports whether err is a classified driver error.
func As(err error) (*Error, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target, true
	}
	return nil, false
}
