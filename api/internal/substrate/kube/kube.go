// Package kube connects AgentSubstrateAPI to a live Substrate install
// through kubeconfig: port-forward to ate-api-server, mint an ate-client
// token, and trust the cluster's API server certificates.
package kube

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	authv1 "k8s.io/api/authentication/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate/remote"
)

const (
	defaultSystemNS = "ate-system"
	defaultSA       = "ate-client"
	defaultAudience = "api.ate-system.svc"
	defaultServer   = "api.ate-system.svc"
	apiPortName     = "grpc"
	apiPort         = 443
)

// Driver is a substrate.Cluster backed by kubeconfig + ate-api-server.
type Driver struct {
	cfg model.Cluster

	mu          sync.Mutex
	remote      *remote.Driver
	restConfig  *rest.Config
	client      kubernetes.Interface
	dyn         dynamic.Interface
	localPort   int
	stopCh      chan struct{}
	readyCh     chan struct{}
	pfDone      chan struct{}
	caPEM       string
	token       string
	tokenExpiry time.Time
	routes      map[string]*liveRoute
}

// New builds a kube driver. Dialing and port-forward happen on first use.
func New(cfg model.Cluster) (*Driver, error) {
	cfg = defaults(cfg)
	restConfig, err := loadREST(cfg.Kubeconfig, cfg.KubeContext)
	if err != nil {
		return nil, err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return nil, substrate.Unavailable("kubernetes client: %s", err.Error())
	}
	dyn, err := dynamic.NewForConfig(restConfig)
	if err != nil {
		return nil, substrate.Unavailable("dynamic kubernetes client: %s", err.Error())
	}
	return &Driver{
		cfg:        cfg,
		restConfig: restConfig,
		client:     client,
		dyn:        dyn,
	}, nil
}

// Detect reports whether the current kubeconfig can reach an ate-api-server.
func Detect(kubeconfig, contextName string) (model.Cluster, error) {
	cfg := defaults(model.Cluster{
		Driver:      "kube",
		Kubeconfig:  kubeconfig,
		KubeContext: contextName,
	})
	resolvedContext, err := resolveContext(cfg.Kubeconfig, cfg.KubeContext)
	if err != nil {
		return model.Cluster{}, err
	}
	cfg.KubeContext = resolvedContext
	restConfig, err := loadREST(cfg.Kubeconfig, cfg.KubeContext)
	if err != nil {
		return model.Cluster{}, err
	}
	client, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return model.Cluster{}, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	if _, err := client.CoreV1().Namespaces().Get(ctx, cfg.SystemNamespace, metav1.GetOptions{}); err != nil {
		return model.Cluster{}, substrate.Unavailable("namespace %s: %s", cfg.SystemNamespace, err.Error())
	}
	pods, err := client.CoreV1().Pods(cfg.SystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=ate-api-server",
	})
	if err != nil {
		return model.Cluster{}, substrate.Unavailable("listing ate-api-server pods: %s", err.Error())
	}
	if len(pods.Items) == 0 {
		return model.Cluster{}, substrate.Unavailable("no ate-api-server pods in %s", cfg.SystemNamespace)
	}
	id := sanitizeID(cfg.KubeContext)
	if id == "" || id == "local" {
		id = "kube"
	}
	cfg.ID = id
	cfg.Name = cfg.KubeContext
	return cfg, nil
}

func resolveContext(kubeconfig, contextName string) (string, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loading.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	raw, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).RawConfig()
	if err != nil {
		return "", substrate.Unavailable("kubeconfig: %s", err.Error())
	}
	if contextName != "" {
		if _, ok := raw.Contexts[contextName]; !ok {
			return "", substrate.Unavailable("kube context %q not found", contextName)
		}
		return contextName, nil
	}
	if raw.CurrentContext == "" {
		return "", substrate.Unavailable("kubeconfig has no current context")
	}
	return raw.CurrentContext, nil
}

func defaults(cfg model.Cluster) model.Cluster {
	if cfg.SystemNamespace == "" {
		cfg.SystemNamespace = defaultSystemNS
	}
	if cfg.ServiceAccount == "" {
		cfg.ServiceAccount = defaultSA
	}
	if cfg.Audience == "" {
		cfg.Audience = defaultAudience
	}
	if cfg.ServerName == "" {
		cfg.ServerName = defaultServer
	}
	cfg.TLS = true
	cfg.Driver = "kube"
	return cfg
}

func loadREST(kubeconfig, contextName string) (*rest.Config, error) {
	loading := clientcmd.NewDefaultClientConfigLoadingRules()
	if kubeconfig != "" {
		loading.ExplicitPath = kubeconfig
	}
	overrides := &clientcmd.ConfigOverrides{}
	if contextName != "" {
		overrides.CurrentContext = contextName
	}
	cfg, err := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(loading, overrides).ClientConfig()
	if err != nil {
		return nil, substrate.Unavailable("kubeconfig: %s", err.Error())
	}
	cfg.QPS = 50
	cfg.Burst = 100
	return cfg, nil
}

func sanitizeID(contextName string) string {
	contextName = strings.ToLower(contextName)
	var b strings.Builder
	for _, r := range contextName {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
		} else if r == '-' || r == '_' || r == '.' {
			b.WriteByte('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if len(out) > 63 {
		out = out[:63]
		out = strings.Trim(out, "-")
	}
	return out
}

// Close stops actor routes, the port-forward, and the gRPC client.
func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopAllRoutesLocked()
	d.stopForwardLocked()
	if d.remote != nil {
		_ = d.remote.Close()
		d.remote = nil
	}
	return nil
}

func (d *Driver) ready(ctx context.Context) (*remote.Driver, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := d.ensureForwardLocked(ctx); err != nil {
		return nil, err
	}
	if _, err := d.ensureTokenLocked(ctx); err != nil {
		return nil, err
	}
	if d.caPEM == "" {
		pem, err := d.loadCAPEM(ctx)
		if err != nil {
			return nil, err
		}
		d.caPEM = pem
	}
	cfg := d.cfg
	cfg.Endpoint = fmt.Sprintf("127.0.0.1:%d", d.localPort)
	cfg.TLS = true
	cfg.ServerName = d.cfg.ServerName
	cfg.CACertPEM = d.caPEM
	cfg.Token = d.token
	if d.remote == nil {
		d.remote = remote.NewWithTokenSource(cfg, d.tokenSource)
	}
	return d.remote, nil
}

func (d *Driver) tokenSource(ctx context.Context) (string, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.ensureTokenLocked(ctx)
}

func (d *Driver) ensureTokenLocked(ctx context.Context) (string, error) {
	if d.token != "" && time.Now().Before(d.tokenExpiry.Add(-2*time.Minute)) {
		return d.token, nil
	}
	exp := int64((12 * time.Hour) / time.Second)
	req := &authv1.TokenRequest{
		Spec: authv1.TokenRequestSpec{
			Audiences:         []string{d.cfg.Audience},
			ExpirationSeconds: &exp,
		},
	}
	out, err := d.client.CoreV1().ServiceAccounts(d.cfg.SystemNamespace).CreateToken(ctx, d.cfg.ServiceAccount, req, metav1.CreateOptions{})
	if err != nil {
		return "", substrate.Unauthenticated("minting %s/%s token: %s", d.cfg.SystemNamespace, d.cfg.ServiceAccount, err.Error())
	}
	d.token = out.Status.Token
	if out.Status.ExpirationTimestamp.IsZero() {
		d.tokenExpiry = time.Now().Add(12 * time.Hour)
	} else {
		d.tokenExpiry = out.Status.ExpirationTimestamp.Time
	}
	return d.token, nil
}

func (d *Driver) loadCAPEM(ctx context.Context) (string, error) {
	gvr := schema.GroupVersionResource{
		Group:    "certificates.k8s.io",
		Version:  "v1",
		Resource: "clustertrustbundles",
	}
	list, err := d.dyn.Resource(gvr).List(ctx, metav1.ListOptions{})
	if err != nil {
		return "", substrate.Unavailable("listing ClusterTrustBundles: %s", err.Error())
	}
	var pems []string
	for _, item := range list.Items {
		signer, _, _ := unstructured.NestedString(item.Object, "spec", "signerName")
		bundle, _, _ := unstructured.NestedString(item.Object, "spec", "trustBundle")
		if bundle == "" {
			continue
		}
		if strings.HasPrefix(signer, "servicedns.podcert.ate.dev") ||
			strings.HasPrefix(signer, "podidentity.podcert.ate.dev") {
			pems = append(pems, strings.TrimSpace(bundle))
		}
	}
	if len(pems) == 0 {
		return "", substrate.Unavailable("no ClusterTrustBundle for ate-api-server TLS")
	}
	return strings.Join(pems, "\n") + "\n", nil
}

func (d *Driver) ensureForwardLocked(ctx context.Context) error {
	if d.stopCh != nil {
		select {
		case <-d.pfDone:
			d.stopForwardLocked()
		default:
			return nil
		}
	}
	pod, err := d.pickAPIServerPod(ctx)
	if err != nil {
		return err
	}
	local, err := freePort()
	if err != nil {
		return err
	}
	stopCh := make(chan struct{})
	readyCh := make(chan struct{})
	doneCh := make(chan struct{})
	url := d.client.CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(d.cfg.SystemNamespace).
		Name(pod.Name).
		SubResource("portforward").
		URL()
	transport, upgrader, err := spdy.RoundTripperFor(d.restConfig)
	if err != nil {
		return substrate.Unavailable("port-forward transport: %s", err.Error())
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, url)
	ports := []string{fmt.Sprintf("%d:%d", local, apiPort)}
	pf, err := portforward.New(dialer, ports, stopCh, readyCh, io.Discard, io.Discard)
	if err != nil {
		return substrate.Unavailable("port-forward: %s", err.Error())
	}
	go func() {
		defer close(doneCh)
		_ = pf.ForwardPorts()
	}()
	select {
	case <-readyCh:
	case <-ctx.Done():
		close(stopCh)
		return ctx.Err()
	case <-time.After(15 * time.Second):
		close(stopCh)
		return substrate.Unavailable("timed out waiting for port-forward to ate-api-server")
	case <-doneCh:
		return substrate.Unavailable("port-forward exited before becoming ready")
	}
	d.localPort = local
	d.stopCh = stopCh
	d.readyCh = readyCh
	d.pfDone = doneCh
	return nil
}

func (d *Driver) stopForwardLocked() {
	if d.stopCh != nil {
		select {
		case <-d.stopCh:
		default:
			close(d.stopCh)
		}
	}
	if d.pfDone != nil {
		<-d.pfDone
	}
	d.stopCh = nil
	d.readyCh = nil
	d.pfDone = nil
	d.localPort = 0
	if d.remote != nil {
		_ = d.remote.Close()
		d.remote = nil
	}
}

func (d *Driver) pickAPIServerPod(ctx context.Context) (*corev1.Pod, error) {
	pods, err := d.client.CoreV1().Pods(d.cfg.SystemNamespace).List(ctx, metav1.ListOptions{
		LabelSelector: "app=ate-api-server",
	})
	if err != nil {
		return nil, substrate.Unavailable("listing ate-api-server pods: %s", err.Error())
	}
	for i := range pods.Items {
		pod := &pods.Items[i]
		if pod.Status.Phase != corev1.PodRunning {
			continue
		}
		for _, c := range pod.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				return pod, nil
			}
		}
	}
	if len(pods.Items) == 0 {
		return nil, substrate.Unavailable("no ate-api-server pods in %s", d.cfg.SystemNamespace)
	}
	return &pods.Items[0], nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, substrate.Unavailable("allocating local port: %s", err.Error())
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

func (d *Driver) withRemote(ctx context.Context, fn func(*remote.Driver) error) error {
	r, err := d.ready(ctx)
	if err != nil {
		return err
	}
	return fn(r)
}

func (d *Driver) Probe(ctx context.Context) (model.Probe, error) {
	var out model.Probe
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.Probe(ctx)
		return e
	})
	if out.Detail == "" && err == nil {
		out.Detail = "kubeconfig port-forward to ate-api-server"
	}
	return out, err
}

func (d *Driver) CreateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error) {
	var out model.Atespace
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.CreateAtespace(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) GetAtespace(ctx context.Context, name string) (model.Atespace, error) {
	var out model.Atespace
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.GetAtespace(ctx, name)
		return e
	})
	return out, err
}
func (d *Driver) ListAtespaces(ctx context.Context) ([]model.Atespace, error) {
	var out []model.Atespace
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.ListAtespaces(ctx)
		return e
	})
	return out, err
}
func (d *Driver) UpdateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error) {
	var out model.Atespace
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.UpdateAtespace(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) DeleteAtespace(ctx context.Context, name string, pre model.Preconditions) (model.Atespace, error) {
	var out model.Atespace
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.DeleteAtespace(ctx, name, pre)
		return e
	})
	return out, err
}

func (d *Driver) CreateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	var out model.ActorTemplate
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.CreateActorTemplate(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) GetActorTemplate(ctx context.Context, atespace, name string) (model.ActorTemplate, error) {
	var out model.ActorTemplate
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.GetActorTemplate(ctx, atespace, name)
		return e
	})
	return out, err
}
func (d *Driver) ListActorTemplates(ctx context.Context, atespace string) ([]model.ActorTemplate, error) {
	var out []model.ActorTemplate
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.ListActorTemplates(ctx, atespace)
		return e
	})
	return out, err
}
func (d *Driver) UpdateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	var out model.ActorTemplate
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.UpdateActorTemplate(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) DeleteActorTemplate(ctx context.Context, atespace, name string, pre model.Preconditions) (model.ActorTemplate, error) {
	var out model.ActorTemplate
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.DeleteActorTemplate(ctx, atespace, name, pre)
		return e
	})
	return out, err
}

func (d *Driver) CreateActor(ctx context.Context, item model.Actor) (model.Actor, error) {
	var out model.Actor
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.CreateActor(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) GetActor(ctx context.Context, atespace, name string) (model.Actor, error) {
	var out model.Actor
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.GetActor(ctx, atespace, name)
		return e
	})
	return out, err
}
func (d *Driver) ListActors(ctx context.Context, atespace string) ([]model.Actor, error) {
	var out []model.Actor
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.ListActors(ctx, atespace)
		return e
	})
	return out, err
}
func (d *Driver) UpdateActor(ctx context.Context, item model.Actor) (model.Actor, error) {
	var out model.Actor
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.UpdateActor(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) DeleteActor(ctx context.Context, atespace, name string, pre model.Preconditions) (model.Actor, error) {
	var out model.Actor
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.DeleteActor(ctx, atespace, name, pre)
		return e
	})
	return out, err
}

func (d *Driver) CreateWorker(ctx context.Context, item model.Worker) (model.Worker, error) {
	var out model.Worker
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.CreateWorker(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) GetWorker(ctx context.Context, name string) (model.Worker, error) {
	var out model.Worker
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.GetWorker(ctx, name)
		return e
	})
	return out, err
}
func (d *Driver) ListWorkers(ctx context.Context) ([]model.Worker, error) {
	var out []model.Worker
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.ListWorkers(ctx)
		return e
	})
	return out, err
}
func (d *Driver) UpdateWorker(ctx context.Context, item model.Worker) (model.Worker, error) {
	var out model.Worker
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.UpdateWorker(ctx, item)
		return e
	})
	return out, err
}
func (d *Driver) DeleteWorker(ctx context.Context, name string, pre model.Preconditions) (model.Worker, error) {
	var out model.Worker
	err := d.withRemote(ctx, func(r *remote.Driver) error {
		var e error
		out, e = r.DeleteWorker(ctx, name, pre)
		return e
	})
	return out, err
}

// HomeKubeconfig is the default kubeconfig path.
func HomeKubeconfig() string {
	if v := os.Getenv("KUBECONFIG"); v != "" {
		return strings.Split(v, string(os.PathListSeparator))[0]
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".kube", "config")
}
