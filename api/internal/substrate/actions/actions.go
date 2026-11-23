// Package actions is shared glue for actor Route / Execute / Command sessions.
package actions

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
)

// NormalizeRoute fills defaults for a route request.
func NormalizeRoute(req model.RouteRequest) model.RouteRequest {
	if req.RemotePort <= 0 {
		req.RemotePort = 80
	}
	return req
}

// NormalizeExecute fills defaults for an execute request.
func NormalizeExecute(req model.ExecuteRequest) model.ExecuteRequest {
	if req.Method == "" {
		req.Method = http.MethodPost
	}
	if req.Path == "" {
		req.Path = "/"
	}
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	if req.RemotePort <= 0 {
		req.RemotePort = 80
	}
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 15000
	}
	return req
}

// FreePort binds an ephemeral localhost port and returns it.
func FreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, substrate.Unavailable("allocating local port: %s", err.Error())
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// WaitListening waits until address accepts TCP connections.
func WaitListening(ctx context.Context, addr string) error {
	deadline := time.Now().Add(20 * time.Second)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 250*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(150 * time.Millisecond):
		}
	}
	return substrate.Unavailable("timed out waiting for %s to listen", addr)
}

// DoHTTP executes an HTTP request against a local actor route.
func DoHTTP(ctx context.Context, baseURL string, req model.ExecuteRequest) (model.ExecuteResult, error) {
	req = NormalizeExecute(req)
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	client := &http.Client{Timeout: timeout}
	url := strings.TrimRight(baseURL, "/") + req.Path
	var bodyReader io.Reader
	if req.Body != "" {
		bodyReader = strings.NewReader(req.Body)
	}
	httpReq, err := http.NewRequestWithContext(ctx, strings.ToUpper(req.Method), url, bodyReader)
	if err != nil {
		return model.ExecuteResult{}, substrate.Invalid("building request: %s", err.Error())
	}
	for k, v := range req.Headers {
		httpReq.Header.Set(k, v)
	}
	if req.Body != "" && httpReq.Header.Get("Content-Type") == "" {
		httpReq.Header.Set("Content-Type", "application/json")
	}
	start := time.Now()
	res, err := client.Do(httpReq)
	if err != nil {
		return model.ExecuteResult{}, substrate.Unavailable("actor request failed: %s", err.Error())
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if err != nil {
		return model.ExecuteResult{}, substrate.Unavailable("reading actor response: %s", err.Error())
	}
	headers := map[string]string{}
	for k, vals := range res.Header {
		if len(vals) > 0 {
			headers[k] = vals[0]
		}
	}
	return model.ExecuteResult{
		Status:    res.StatusCode,
		Headers:   headers,
		Body:      string(raw),
		LatencyMs: time.Since(start).Milliseconds(),
		LocalURL:  url,
	}, nil
}

// SessionID builds a stable route id for an actor.
func SessionID(atespace, name string) string {
	return fmt.Sprintf("%s/%s", atespace, name)
}

// NormalizeCommand fills defaults and resolves shell vs argv.
func NormalizeCommand(req model.CommandRequest) (model.CommandRequest, error) {
	req.Shell = strings.TrimSpace(req.Shell)
	if req.Path == "" {
		req.Path = "/process"
	}
	if !strings.HasPrefix(req.Path, "/") {
		req.Path = "/" + req.Path
	}
	if req.RemotePort <= 0 {
		req.RemotePort = 80
	}
	if req.TimeoutMs <= 0 {
		req.TimeoutMs = 30000
	}
	if req.ShellBin == "" {
		req.ShellBin = "sh"
	}
	if req.Shell != "" {
		req.Command = []string{req.ShellBin, "-c", req.Shell}
	}
	if len(req.Command) == 0 {
		return req, substrate.Invalid("shell or command is required")
	}
	if req.Command[0] == "" {
		return req, substrate.Invalid("command[0] must be a program name")
	}
	return req, nil
}

type processPayload struct {
	Command []string          `json:"command"`
	EnvVars map[string]string `json:"envvars,omitempty"`
	Cwd     string            `json:"cwd,omitempty"`
	Timeout string            `json:"timeout,omitempty"`
}

type processResponse struct {
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
	ExitCode int    `json:"exitCode"`
	Error    string `json:"error,omitempty"`
}

// DoCommand POSTs a process request through an actor route (/process by default).
func DoCommand(ctx context.Context, baseURL string, req model.CommandRequest) (model.CommandResult, error) {
	req, err := NormalizeCommand(req)
	if err != nil {
		return model.CommandResult{}, err
	}
	payload, err := json.Marshal(processPayload{
		Command: req.Command,
		EnvVars: req.EnvVars,
		Cwd:     req.Cwd,
		Timeout: req.Timeout,
	})
	if err != nil {
		return model.CommandResult{}, substrate.Invalid("encoding command: %s", err.Error())
	}
	httpResult, err := DoHTTP(ctx, baseURL, model.ExecuteRequest{
		Method:     http.MethodPost,
		Path:       req.Path,
		Headers:    map[string]string{"Content-Type": "application/json"},
		Body:       string(payload),
		RemotePort: req.RemotePort,
		TimeoutMs:  req.TimeoutMs,
	})
	if err != nil {
		return model.CommandResult{}, err
	}
	out := model.CommandResult{
		LatencyMs: httpResult.LatencyMs,
		LocalURL:  httpResult.LocalURL,
		RawBody:   httpResult.Body,
	}
	if httpResult.Status < 200 || httpResult.Status >= 300 {
		out.ExitCode = -1
		out.Error = fmt.Sprintf("actor returned HTTP %d", httpResult.Status)
		out.Stderr = httpResult.Body
		return out, nil
	}
	var parsed processResponse
	if err := json.Unmarshal([]byte(httpResult.Body), &parsed); err != nil {
		out.ExitCode = 0
		out.Stdout = httpResult.Body
		return out, nil
	}
	out.Stdout = parsed.Stdout
	out.Stderr = parsed.Stderr
	out.ExitCode = parsed.ExitCode
	out.Error = parsed.Error
	return out, nil
}

// RunLocalCommand runs argv on the API host (memory/demo driver).
func RunLocalCommand(ctx context.Context, req model.CommandRequest) (model.CommandResult, error) {
	req, err := NormalizeCommand(req)
	if err != nil {
		return model.CommandResult{}, err
	}
	timeout := time.Duration(req.TimeoutMs) * time.Millisecond
	if req.Timeout != "" {
		if d, err := time.ParseDuration(req.Timeout); err == nil {
			timeout = d
		}
	}
	runCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(runCtx, req.Command[0], req.Command[1:]...)
	if req.Cwd != "" {
		cmd.Dir = req.Cwd
	}
	if len(req.EnvVars) > 0 {
		cmd.Env = append(cmd.Environ(), flattenEnv(req.EnvVars)...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	start := time.Now()
	err = cmd.Run()
	out := model.CommandResult{
		Stdout:    stdout.String(),
		Stderr:    stderr.String(),
		LatencyMs: time.Since(start).Milliseconds(),
	}
	if err != nil {
		out.Error = err.Error()
		if exitErr, ok := err.(*exec.ExitError); ok {
			out.ExitCode = exitErr.ExitCode()
		} else {
			out.ExitCode = -1
		}
		return out, nil
	}
	out.ExitCode = 0
	return out, nil
}

func flattenEnv(vars map[string]string) []string {
	out := make([]string, 0, len(vars))
	for k, v := range vars {
		out = append(out, k+"="+v)
	}
	return out
}
