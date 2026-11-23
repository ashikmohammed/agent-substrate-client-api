// Package remote dials any Agent Substrate control plane.
//
// The driver does not vendor a generated client. On first use it asks the
// cluster for its gRPC schema (server reflection) and calls the Control
// service by method name. Field numbers can change between substrate
// versions; the JSON names in the live schema are what get sent.
package remote

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protodesc"
	"google.golang.org/protobuf/reflect/protoreflect"
	"google.golang.org/protobuf/reflect/protoregistry"
	"google.golang.org/protobuf/types/descriptorpb"
	"google.golang.org/protobuf/types/dynamicpb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"google.golang.org/protobuf/types/known/timestamppb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	reflection "google.golang.org/grpc/reflection/grpc_reflection_v1"

	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/model"
	"github.com/ashikmohammed/agent-substrate-client-api/api/internal/substrate"
)

// TokenSource supplies a bearer token for each RPC.
type TokenSource func(ctx context.Context) (string, error)

// Keep the well-known descriptors in the global registry so reflection
// responses can resolve timestamp and empty imports.
var (
	_ = timestamppb.Now
	_ = durationpb.New
	_ = emptypb.Empty{}
	_ = structpb.NullValue_NULL_VALUE
	_ = wrapperspb.Int64(0)
)

// Driver implements substrate.Cluster against a live ate-api-server.
type Driver struct {
	cfg         model.Cluster
	tokenSource TokenSource

	mu   sync.Mutex
	conn *grpc.ClientConn
	svc  protoreflect.ServiceDescriptor
}

// New returns a driver that connects lazily.
func New(cfg model.Cluster) *Driver {
	return &Driver{cfg: cfg}
}

// NewWithTokenSource returns a driver that refreshes credentials per RPC.
func NewWithTokenSource(cfg model.Cluster, source TokenSource) *Driver {
	return &Driver{cfg: cfg, tokenSource: source}
}

// Close drops the gRPC connection.
func (d *Driver) Close() error {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		return nil
	}
	err := d.conn.Close()
	d.conn = nil
	d.svc = nil
	return err
}

func (d *Driver) Probe(ctx context.Context) (model.Probe, error) {
	start := time.Now()
	_, err := d.invoke(ctx, "ListAtespaces", map[string]any{
		"options": map[string]any{"pageSize": 1},
	})
	p := model.Probe{LatencyMS: time.Since(start).Milliseconds(), Service: d.serviceName()}
	if err != nil {
		p.Detail = err.Error()
		return p, nil
	}
	p.OK = true
	p.Detail = "control plane responded"
	return p, nil
}

func (d *Driver) CreateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error) {
	return call[model.Atespace](d, ctx, "CreateAtespace", "atespace", item, true)
}

func (d *Driver) GetAtespace(ctx context.Context, name string) (model.Atespace, error) {
	return callRef[model.Atespace](d, ctx, "GetAtespace", "atespace", "", name)
}

func (d *Driver) ListAtespaces(ctx context.Context) ([]model.Atespace, error) {
	return list[model.Atespace](d, ctx, "ListAtespaces", "atespaces", "", false)
}

func (d *Driver) UpdateAtespace(ctx context.Context, item model.Atespace) (model.Atespace, error) {
	if err := d.requireMethod(ctx, "UpdateAtespace"); err != nil {
		if se, ok := substrate.As(err); ok && se.Code == substrate.UnimplementedCode {
			return model.Atespace{}, substrate.Unimplemented("this cluster has no UpdateAtespace method; create, view, and delete are the supported operations")
		}
		return model.Atespace{}, err
	}
	cur, err := d.fetch(ctx, "GetAtespace", "atespace", "", item.Metadata.Name)
	if err != nil {
		return model.Atespace{}, err
	}
	cur["description"] = item.Description
	setPreconditions(cur, item.Metadata)
	out, err := d.invoke(ctx, "UpdateAtespace", map[string]any{"atespace": cur})
	if err != nil {
		return model.Atespace{}, err
	}
	return decode[model.Atespace](out)
}

func (d *Driver) DeleteAtespace(ctx context.Context, name string, pre model.Preconditions) (model.Atespace, error) {
	return deleteRef[model.Atespace](d, ctx, "DeleteAtespace", "atespace", "", name, pre)
}

func (d *Driver) CreateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	return call[model.ActorTemplate](d, ctx, "CreateActorTemplate", "actorTemplate", item, true)
}

func (d *Driver) GetActorTemplate(ctx context.Context, atespace, name string) (model.ActorTemplate, error) {
	return callRef[model.ActorTemplate](d, ctx, "GetActorTemplate", "actorTemplate", atespace, name)
}

func (d *Driver) ListActorTemplates(ctx context.Context, atespace string) ([]model.ActorTemplate, error) {
	return list[model.ActorTemplate](d, ctx, "ListActorTemplates", "actorTemplates", atespace, true)
}

func (d *Driver) UpdateActorTemplate(ctx context.Context, item model.ActorTemplate) (model.ActorTemplate, error) {
	if err := d.requireMethod(ctx, "UpdateActorTemplate"); err != nil {
		if se, ok := substrate.As(err); ok && se.Code == substrate.UnimplementedCode {
			return model.ActorTemplate{}, substrate.Unimplemented("this cluster treats actor templates as immutable; delete the template and create a new one")
		}
		return model.ActorTemplate{}, err
	}
	cur, err := d.fetch(ctx, "GetActorTemplate", "actorTemplate", item.Metadata.Atespace, item.Metadata.Name)
	if err != nil {
		return model.ActorTemplate{}, err
	}
	overlay, err := toMap(item)
	if err != nil {
		return model.ActorTemplate{}, err
	}
	for _, key := range []string{"containers", "workerSelector", "resources", "volumes", "snapshotConfig"} {
		if v, ok := overlay[key]; ok {
			cur[key] = v
		} else {
			delete(cur, key)
		}
	}
	setPreconditions(cur, item.Metadata)
	out, err := d.invoke(ctx, "UpdateActorTemplate", map[string]any{"actorTemplate": cur})
	if err != nil {
		return model.ActorTemplate{}, err
	}
	return decode[model.ActorTemplate](out)
}

func (d *Driver) DeleteActorTemplate(ctx context.Context, atespace, name string, pre model.Preconditions) (model.ActorTemplate, error) {
	return deleteRef[model.ActorTemplate](d, ctx, "DeleteActorTemplate", "actorTemplate", atespace, name, pre)
}

func (d *Driver) CreateActor(ctx context.Context, item model.Actor) (model.Actor, error) {
	return call[model.Actor](d, ctx, "CreateActor", "actor", item, true)
}

func (d *Driver) GetActor(ctx context.Context, atespace, name string) (model.Actor, error) {
	return callRef[model.Actor](d, ctx, "GetActor", "actor", atespace, name)
}

func (d *Driver) ListActors(ctx context.Context, atespace string) ([]model.Actor, error) {
	return list[model.Actor](d, ctx, "ListActors", "actors", atespace, true)
}

func (d *Driver) UpdateActor(ctx context.Context, item model.Actor) (model.Actor, error) {
	cur, err := d.fetch(ctx, "GetActor", "actor", item.Metadata.Atespace, item.Metadata.Name)
	if err != nil {
		return model.Actor{}, err
	}
	if item.WorkerSelector == nil {
		cur["workerSelector"] = nil
	} else {
		raw, err := toMap(item.WorkerSelector)
		if err != nil {
			return model.Actor{}, err
		}
		cur["workerSelector"] = raw
	}
	setPreconditions(cur, item.Metadata)
	out, err := d.invoke(ctx, "UpdateActor", map[string]any{"actor": cur})
	if err != nil {
		return model.Actor{}, err
	}
	return decode[model.Actor](out)
}

func (d *Driver) DeleteActor(ctx context.Context, atespace, name string, pre model.Preconditions) (model.Actor, error) {
	return deleteRef[model.Actor](d, ctx, "DeleteActor", "actor", atespace, name, pre)
}

func (d *Driver) CreateWorker(ctx context.Context, item model.Worker) (model.Worker, error) {
	return call[model.Worker](d, ctx, "CreateWorker", "worker", item, true)
}

func (d *Driver) GetWorker(ctx context.Context, name string) (model.Worker, error) {
	return callRef[model.Worker](d, ctx, "GetWorker", "worker", "", name)
}

func (d *Driver) ListWorkers(ctx context.Context) ([]model.Worker, error) {
	return list[model.Worker](d, ctx, "ListWorkers", "workers", "", false)
}

func (d *Driver) UpdateWorker(ctx context.Context, item model.Worker) (model.Worker, error) {
	cur, err := d.fetch(ctx, "GetWorker", "worker", "", item.Metadata.Name)
	if err != nil {
		return model.Worker{}, err
	}
	if item.Labels == nil {
		cur["labels"] = map[string]any{}
	} else {
		labels := map[string]any{}
		for k, v := range item.Labels {
			labels[k] = v
		}
		cur["labels"] = labels
	}
	setPreconditions(cur, item.Metadata)
	out, err := d.invoke(ctx, "UpdateWorker", map[string]any{"worker": cur})
	if err != nil {
		return model.Worker{}, err
	}
	return decode[model.Worker](out)
}

func (d *Driver) DeleteWorker(ctx context.Context, name string, pre model.Preconditions) (model.Worker, error) {
	return deleteRef[model.Worker](d, ctx, "DeleteWorker", "worker", "", name, pre)
}

func call[T any](d *Driver, ctx context.Context, method, field string, item any, strip bool) (T, error) {
	var zero T
	body, err := toMap(item)
	if err != nil {
		return zero, err
	}
	if strip {
		stripServerFields(body)
	}
	out, err := d.invoke(ctx, method, map[string]any{field: body})
	if err != nil {
		return zero, err
	}
	return decode[T](out)
}

func callRef[T any](d *Driver, ctx context.Context, method, field, atespace, name string) (T, error) {
	var zero T
	out, err := d.fetch(ctx, method, field, atespace, name)
	if err != nil {
		return zero, err
	}
	return decode[T](out)
}

func deleteRef[T any](d *Driver, ctx context.Context, method, field, atespace, name string, pre model.Preconditions) (T, error) {
	var zero T
	req := map[string]any{field: objectRef(atespace, name)}
	if opts := deleteOptions(pre); opts != nil {
		req["options"] = opts
	}
	out, err := d.invoke(ctx, method, req)
	if err != nil {
		return zero, err
	}
	return decode[T](out)
}

func list[T any](d *Driver, ctx context.Context, method, key, atespace string, scoped bool) ([]T, error) {
	raw, err := d.listMaps(ctx, method, key, atespace, scoped)
	if err != nil {
		return nil, err
	}
	out := make([]T, 0, len(raw))
	for _, item := range raw {
		decoded, err := decode[T](item)
		if err != nil {
			return nil, err
		}
		out = append(out, decoded)
	}
	if !scoped || atespace == "" {
		return out, nil
	}
	filtered := out[:0]
	for _, item := range out {
		body, err := toMap(item)
		if err != nil {
			return nil, err
		}
		meta, _ := body["metadata"].(map[string]any)
		space, _ := meta["atespace"].(string)
		if space == atespace {
			filtered = append(filtered, item)
		}
	}
	return filtered, nil
}

func (d *Driver) fetch(ctx context.Context, method, field, atespace, name string) (map[string]any, error) {
	return d.invoke(ctx, method, map[string]any{field: objectRef(atespace, name)})
}

func (d *Driver) listMaps(ctx context.Context, method, key, atespace string, scoped bool) ([]map[string]any, error) {
	var all []map[string]any
	token := ""
	seen := map[string]bool{}
	for page := 0; page < 25; page++ {
		opts := map[string]any{"pageSize": 200}
		if token != "" {
			if seen[token] {
				break
			}
			seen[token] = true
			opts["pageToken"] = token
		}
		req := map[string]any{"options": opts}
		if scoped {
			req["atespace"] = atespace
		}
		resp, err := d.invoke(ctx, method, req)
		if err != nil {
			return nil, err
		}
		all = append(all, mapsOf(resp, key)...)
		next, _ := resp["nextPageToken"].(string)
		if next == "" {
			break
		}
		token = next
	}
	return all, nil
}

func (d *Driver) requireMethod(ctx context.Context, name string) error {
	svc, _, err := d.session(ctx)
	if err != nil {
		return err
	}
	if svc.Methods().ByName(protoreflect.Name(name)) == nil {
		return substrate.Unimplemented("%s", name)
	}
	return nil
}

func (d *Driver) serviceName() string {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.svc == nil {
		return ""
	}
	return string(d.svc.FullName())
}

func (d *Driver) invoke(ctx context.Context, method string, req map[string]any) (map[string]any, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
	}
	svc, conn, err := d.session(ctx)
	if err != nil {
		return nil, err
	}
	md := svc.Methods().ByName(protoreflect.Name(method))
	if md == nil {
		return nil, substrate.Unimplemented("%s is not served by %s", method, svc.FullName())
	}
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	in := dynamicpb.NewMessage(md.Input())
	if err := (protojson.UnmarshalOptions{DiscardUnknown: true}).Unmarshal(payload, in); err != nil {
		return nil, substrate.Invalid("request does not match the cluster schema for %s: %s", method, err.Error())
	}
	out := dynamicpb.NewMessage(md.Output())
	full := "/" + string(svc.FullName()) + "/" + method
	if err := conn.Invoke(ctx, full, in, out); err != nil {
		return nil, fromGRPC(err)
	}
	raw, err := (protojson.MarshalOptions{EmitUnpopulated: false}).Marshal(out)
	if err != nil {
		return nil, err
	}
	if len(raw) == 0 || string(raw) == "{}" {
		return map[string]any{}, nil
	}
	var decoded map[string]any
	if err := json.Unmarshal(raw, &decoded); err != nil {
		return nil, err
	}
	return decoded, nil
}

func (d *Driver) session(ctx context.Context) (protoreflect.ServiceDescriptor, *grpc.ClientConn, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.conn == nil {
		conn, err := d.dial()
		if err != nil {
			return nil, nil, err
		}
		d.conn = conn
	}
	if d.svc == nil {
		svc, err := discover(ctx, d.conn)
		if err != nil {
			return nil, nil, err
		}
		d.svc = svc
	}
	return d.svc, d.conn, nil
}

func (d *Driver) dial() (*grpc.ClientConn, error) {
	cfg := d.cfg
	if strings.TrimSpace(cfg.Endpoint) == "" {
		return nil, substrate.Invalid("cluster endpoint is required")
	}
	creds, err := transportCreds(cfg)
	if err != nil {
		return nil, err
	}
	source := d.tokenSource
	if source == nil {
		token := cfg.Token
		source = func(context.Context) (string, error) { return token, nil }
	}
	conn, err := grpc.NewClient(cfg.Endpoint,
		grpc.WithTransportCredentials(creds),
		grpc.WithPerRPCCredentials(tokenCreds{source: source}),
	)
	if err != nil {
		return nil, substrate.Unavailable("dial %s: %s", cfg.Endpoint, err.Error())
	}
	return conn, nil
}

func transportCreds(cfg model.Cluster) (credentials.TransportCredentials, error) {
	if !cfg.TLS {
		return insecure.NewCredentials(), nil
	}
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if cfg.ServerName != "" {
		tlsCfg.ServerName = cfg.ServerName
	}
	if cfg.InsecureSkipVerify {
		tlsCfg.InsecureSkipVerify = true
	}
	if pem := strings.TrimSpace(cfg.CACertPEM); pem != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(pem)) {
			return nil, substrate.Invalid("caCertPem is not a valid PEM certificate")
		}
		tlsCfg.RootCAs = pool
	}
	return credentials.NewTLS(tlsCfg), nil
}

type tokenCreds struct {
	source TokenSource
}

func (t tokenCreds) GetRequestMetadata(ctx context.Context, _ ...string) (map[string]string, error) {
	if t.source == nil {
		return nil, nil
	}
	token, err := t.source(ctx)
	if err != nil {
		return nil, err
	}
	if token == "" {
		return nil, nil
	}
	value := token
	if !strings.HasPrefix(strings.ToLower(value), "bearer ") {
		value = "Bearer " + value
	}
	return map[string]string{"authorization": value}, nil
}

func (t tokenCreds) RequireTransportSecurity() bool { return false }

func discover(ctx context.Context, conn *grpc.ClientConn) (protoreflect.ServiceDescriptor, error) {
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, 20*time.Second)
		defer cancel()
	}
	client := reflection.NewServerReflectionClient(conn)
	stream, err := client.ServerReflectionInfo(ctx)
	if err != nil {
		return nil, substrate.Unavailable("reflection stream: %s", err.Error())
	}
	defer stream.CloseSend()

	if err := stream.Send(&reflection.ServerReflectionRequest{
		MessageRequest: &reflection.ServerReflectionRequest_ListServices{ListServices: "*"},
	}); err != nil {
		return nil, substrate.Unavailable("listing control services: %s", fromGRPC(err).Error())
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, substrate.Unavailable("cluster did not serve gRPC reflection: %s", err.Error())
	}
	if errResp := resp.GetErrorResponse(); errResp != nil {
		return nil, substrate.Unavailable("reflection: %s", errResp.GetErrorMessage())
	}
	var names []string
	for _, svc := range resp.GetListServicesResponse().GetService() {
		name := svc.GetName()
		if name == "" || strings.Contains(name, "ServerReflection") {
			continue
		}
		names = append(names, name)
	}
	var fallback protoreflect.ServiceDescriptor
	for _, name := range names {
		desc, err := loadService(stream, name)
		if err != nil {
			continue
		}
		if desc.Methods().ByName("CreateActor") != nil && desc.Methods().ByName("CreateAtespace") != nil {
			return desc, nil
		}
		if fallback == nil && desc.Methods().ByName("CreateActor") != nil {
			fallback = desc
		}
	}
	if fallback != nil {
		return fallback, nil
	}
	return nil, substrate.Unavailable("no Substrate control service among: %s", strings.Join(names, ", "))
}

func loadService(stream reflection.ServerReflection_ServerReflectionInfoClient, symbol string) (protoreflect.ServiceDescriptor, error) {
	if err := stream.Send(&reflection.ServerReflectionRequest{
		MessageRequest: &reflection.ServerReflectionRequest_FileContainingSymbol{FileContainingSymbol: symbol},
	}); err != nil {
		return nil, err
	}
	resp, err := stream.Recv()
	if err != nil {
		return nil, err
	}
	if errResp := resp.GetErrorResponse(); errResp != nil {
		return nil, substrate.Unavailable("%s", errResp.GetErrorMessage())
	}
	files, err := filesFromBytes(resp.GetFileDescriptorResponse().GetFileDescriptorProto())
	if err != nil {
		return nil, err
	}
	desc, err := files.FindDescriptorByName(protoreflect.FullName(symbol))
	if err != nil {
		return nil, err
	}
	svc, ok := desc.(protoreflect.ServiceDescriptor)
	if !ok {
		return nil, substrate.Unavailable("%s is not a service", symbol)
	}
	return svc, nil
}

func filesFromBytes(raw [][]byte) (*protoregistry.Files, error) {
	reg := &protoregistry.Files{}
	pending := make([]*descriptorpb.FileDescriptorProto, 0, len(raw))
	for _, b := range raw {
		fd := &descriptorpb.FileDescriptorProto{}
		if err := proto.Unmarshal(b, fd); err != nil {
			return nil, err
		}
		pending = append(pending, fd)
	}
	res := chainResolver{local: reg, global: protoregistry.GlobalFiles}
	for len(pending) > 0 {
		var next []*descriptorpb.FileDescriptorProto
		progress := false
		for _, fd := range pending {
			file, err := protodesc.NewFile(fd, res)
			if err != nil {
				next = append(next, fd)
				continue
			}
			if err := reg.RegisterFile(file); err != nil {
				if strings.Contains(err.Error(), "already") {
					progress = true
					continue
				}
				next = append(next, fd)
				continue
			}
			progress = true
		}
		if !progress {
			return nil, substrate.Unavailable("could not resolve the cluster's proto dependencies")
		}
		pending = next
	}
	return reg, nil
}

type chainResolver struct {
	local  *protoregistry.Files
	global *protoregistry.Files
}

func (c chainResolver) FindFileByPath(path string) (protoreflect.FileDescriptor, error) {
	if fd, err := c.local.FindFileByPath(path); err == nil {
		return fd, nil
	}
	return c.global.FindFileByPath(path)
}

func (c chainResolver) FindDescriptorByName(name protoreflect.FullName) (protoreflect.Descriptor, error) {
	if d, err := c.local.FindDescriptorByName(name); err == nil {
		return d, nil
	}
	return c.global.FindDescriptorByName(name)
}

func objectRef(atespace, name string) map[string]any {
	ref := map[string]any{"name": name}
	if atespace != "" {
		ref["atespace"] = atespace
	}
	return ref
}

func deleteOptions(pre model.Preconditions) map[string]any {
	opts := map[string]any{}
	if pre.Version != 0 {
		opts["version"] = pre.Version
	}
	if pre.UID != "" {
		opts["uid"] = pre.UID
	}
	if len(opts) == 0 {
		return nil
	}
	return opts
}

func setPreconditions(obj map[string]any, meta model.ResourceMetadata) {
	raw, _ := obj["metadata"].(map[string]any)
	if raw == nil {
		raw = map[string]any{}
		obj["metadata"] = raw
	}
	raw["uid"] = meta.UID
	raw["version"] = int64(meta.Version)
}

func stripServerFields(obj map[string]any) {
	if meta, ok := obj["metadata"].(map[string]any); ok {
		delete(meta, "uid")
		delete(meta, "version")
		delete(meta, "createTime")
		delete(meta, "updateTime")
		delete(meta, "deleteTime")
	}
	delete(obj, "status")
}

func toMap(v any) (map[string]any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	if out == nil {
		out = map[string]any{}
	}
	return out, nil
}

func decode[T any](m map[string]any) (T, error) {
	var zero T
	b, err := json.Marshal(m)
	if err != nil {
		return zero, err
	}
	var out T
	if err := json.Unmarshal(b, &out); err != nil {
		return zero, substrate.Unavailable("cluster response did not match the console model: %s", err.Error())
	}
	return out, nil
}

func mapsOf(m map[string]any, key string) []map[string]any {
	raw, ok := m[key].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		obj, ok := item.(map[string]any)
		if ok {
			out = append(out, obj)
		}
	}
	return out
}

func fromGRPC(err error) error {
	st, ok := status.FromError(err)
	if !ok {
		return substrate.Unavailable("%s", err.Error())
	}
	msg := st.Message()
	switch st.Code() {
	case codes.NotFound:
		return substrate.NotFound("%s", msg)
	case codes.AlreadyExists:
		return substrate.AlreadyExists("%s", msg)
	case codes.InvalidArgument:
		return substrate.Invalid("%s", msg)
	case codes.Aborted:
		return substrate.Aborted("%s", msg)
	case codes.FailedPrecondition:
		return substrate.FailedPrecondition("%s", msg)
	case codes.Unimplemented:
		return substrate.Unimplemented("%s", msg)
	case codes.Unauthenticated:
		return substrate.Unauthenticated("%s", msg)
	case codes.PermissionDenied:
		return substrate.PermissionDenied("%s", msg)
	default:
		return substrate.Unavailable("%s", msg)
	}
}
