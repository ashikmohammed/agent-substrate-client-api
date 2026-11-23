package kube

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/actions"
)

type liveRoute struct {
	session model.RouteSession
	cmd     *exec.Cmd
	cancel  context.CancelFunc
}

func (d *Driver) RouteActor(ctx context.Context, atespace, name string, req model.RouteRequest) (model.RouteSession, error) {
	if _, err := d.GetActor(ctx, atespace, name); err != nil {
		return model.RouteSession{}, err
	}
	req = actions.NormalizeRoute(req)
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.routes == nil {
		d.routes = map[string]*liveRoute{}
	}
	id := actions.SessionID(atespace, name)
	if existing, ok := d.routes[id]; ok && existing.cmd.Process != nil {
		return existing.session, nil
	}
	localPort := req.LocalPort
	if localPort <= 0 {
		port, err := actions.FreePort()
		if err != nil {
			return model.RouteSession{}, err
		}
		localPort = port
	}
	session, cmd, cancel, err := d.startProxyLocked(ctx, atespace, name, localPort, req.RemotePort)
	if err != nil {
		return model.RouteSession{}, err
	}
	d.routes[id] = &liveRoute{session: session, cmd: cmd, cancel: cancel}
	return session, nil
}

func (d *Driver) GetActorRoute(_ context.Context, atespace, name string) (model.RouteSession, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	route, ok := d.routes[actions.SessionID(atespace, name)]
	if !ok {
		return model.RouteSession{}, substrate.NotFound("no active route for actor %s/%s", atespace, name)
	}
	return route.session, nil
}

func (d *Driver) StopActorRoute(_ context.Context, atespace, name string) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	id := actions.SessionID(atespace, name)
	route, ok := d.routes[id]
	if !ok {
		return substrate.NotFound("no active route for actor %s/%s", atespace, name)
	}
	d.stopRouteLocked(id, route)
	return nil
}

func (d *Driver) ExecuteActor(ctx context.Context, atespace, name string, req model.ExecuteRequest) (model.ExecuteResult, error) {
	req = actions.NormalizeExecute(req)
	session, err := d.ensureRoute(ctx, atespace, name, req.RemotePort)
	if err != nil {
		return model.ExecuteResult{}, err
	}
	result, err := actions.DoHTTP(ctx, session.LocalURL, req)
	if err != nil {
		return model.ExecuteResult{}, err
	}
	result.RouteID = session.ID
	return result, nil
}

func (d *Driver) CommandActor(ctx context.Context, atespace, name string, req model.CommandRequest) (model.CommandResult, error) {
	req, err := actions.NormalizeCommand(req)
	if err != nil {
		return model.CommandResult{}, err
	}
	session, err := d.ensureRoute(ctx, atespace, name, req.RemotePort)
	if err != nil {
		return model.CommandResult{}, err
	}
	result, err := actions.DoCommand(ctx, session.LocalURL, req)
	if err != nil {
		return model.CommandResult{}, err
	}
	result.RouteID = session.ID
	return result, nil
}

func (d *Driver) ensureRoute(ctx context.Context, atespace, name string, remotePort int) (model.RouteSession, error) {
	d.mu.Lock()
	if d.routes != nil {
		if existing, ok := d.routes[actions.SessionID(atespace, name)]; ok && existing.cmd.Process != nil {
			session := existing.session
			d.mu.Unlock()
			return session, nil
		}
	}
	d.mu.Unlock()
	return d.RouteActor(ctx, atespace, name, model.RouteRequest{RemotePort: remotePort})
}

func (d *Driver) startProxyLocked(ctx context.Context, atespace, name string, localPort, remotePort int) (model.RouteSession, *exec.Cmd, context.CancelFunc, error) {
	bin, err := exec.LookPath("kubectl-ate")
	if err != nil {
		return model.RouteSession{}, nil, nil, substrate.Unavailable("kubectl-ate not found on PATH; install it to route to actors")
	}
	proxyCtx, cancel := context.WithCancel(context.Background())
	args := []string{
		"proxy", "actor", name,
		fmt.Sprintf("%d:%d", localPort, remotePort),
		"-a", atespace,
		"--address", "127.0.0.1",
	}
	if d.cfg.KubeContext != "" {
		args = append(args, "--context", d.cfg.KubeContext)
	}
	if d.cfg.Kubeconfig != "" {
		args = append(args, "--kubeconfig", d.cfg.Kubeconfig)
	}
	cmd := exec.CommandContext(proxyCtx, bin, args...)
	cmd.Env = os.Environ()
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return model.RouteSession{}, nil, nil, err
	}
	cmd.Stderr = cmd.Stdout
	if err := cmd.Start(); err != nil {
		cancel()
		return model.RouteSession{}, nil, nil, substrate.Unavailable("starting actor proxy: %s", err.Error())
	}
	addr := fmt.Sprintf("127.0.0.1:%d", localPort)
	waitCtx, waitCancel := context.WithTimeout(ctx, 25*time.Second)
	defer waitCancel()
	errCh := make(chan error, 1)
	go func() {
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			if strings.Contains(strings.ToLower(line), "error") || strings.Contains(line, "Error:") {
				select {
				case errCh <- substrate.Unavailable("actor proxy: %s", line):
				default:
				}
			}
		}
		if err := cmd.Wait(); err != nil {
			select {
			case errCh <- substrate.Unavailable("actor proxy exited: %s", err.Error()):
			default:
			}
		}
	}()
	if err := actions.WaitListening(waitCtx, addr); err != nil {
		cancel()
		_ = cmd.Process.Kill()
		select {
		case proxyErr := <-errCh:
			return model.RouteSession{}, nil, nil, proxyErr
		default:
			return model.RouteSession{}, nil, nil, err
		}
	}
	session := model.RouteSession{
		ID:         actions.SessionID(atespace, name),
		Atespace:   atespace,
		Actor:      name,
		LocalPort:  localPort,
		RemotePort: remotePort,
		LocalURL:   "http://" + addr,
		Status:     "ready",
		Detail:     "kubectl-ate proxy actor (resumes on demand)",
		CreatedAt:  time.Now().UTC(),
	}
	return session, cmd, cancel, nil
}

func (d *Driver) stopRouteLocked(id string, route *liveRoute) {
	if route.cancel != nil {
		route.cancel()
	}
	if route.cmd != nil && route.cmd.Process != nil {
		_ = route.cmd.Process.Kill()
		_, _ = route.cmd.Process.Wait()
	}
	delete(d.routes, id)
}

func (d *Driver) stopAllRoutesLocked() {
	for id, route := range d.routes {
		d.stopRouteLocked(id, route)
	}
}
