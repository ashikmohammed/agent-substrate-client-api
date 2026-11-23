# Agent Substrate client

Two apps for the [Agent Substrate](https://ate.dev) control plane:

- **AgentSubstrateAPI** (`api/`) is a Go HTTP API. It creates, reads, updates, and deletes atespaces, actor templates, actors, and workers. A cluster driver interface lets it talk to the in-memory demo or to any live substrate control plane.
- **Agent Substrate UI** (`ui/`) is the console: a landing dashboard plus create and view flows for those four resources.

## Run

In one terminal:

```bash
cd api
go run ./cmd/agentsubstrate-api
```

The API listens on `:8080`. On first start it:

- creates a `local` in-memory demo cluster
- auto-detects a live Substrate install from your kubeconfig (looks for `ate-api-server` in `ate-system`) and registers it as a `kube` cluster

The UI prefers the live kube cluster. That driver port-forwards to `ate-api-server`, mints a token for `ate-system/ate-client` with audience `api.ate-system.svc`, and trusts the cluster’s `ClusterTrustBundle` CAs.

In another terminal:

```bash
cd ui
npm install
npm run dev
```

Open http://localhost:5173. The dashboard should show real atespaces, actor templates, actors, and workers from the live cluster.

## Connect another cluster

`POST /api/v1/clusters` registers a control plane. Prefer the kubeconfig driver:

```bash
curl -s -X POST localhost:8080/api/v1/clusters \
  -H 'content-type: application/json' \
  -d '{
    "id": "kind",
    "name": "kind-kind",
    "driver": "kube",
    "kubeContext": "kind-kind",
    "systemNamespace": "ate-system"
  }'
```

A direct gRPC driver is also available when you already have an endpoint and token:

```bash
curl -s -X POST localhost:8080/api/v1/clusters \
  -H 'content-type: application/json' \
  -d '{
    "id": "lab",
    "name": "Lab cluster",
    "driver": "grpc",
    "endpoint": "127.0.0.1:50051",
    "token": "YOUR_TOKEN",
    "tls": false
  }'
```

The control plane must serve gRPC reflection. The same form is on the Clusters page in the UI.

Actor templates are immutable on a current Substrate cluster. The API still exposes update: the demo cluster applies it, and a live cluster returns a clear error when `UpdateActorTemplate` is not part of its schema. Actor updates change `workerSelector`. Worker updates change labels. Both send the uid and version you last read.

## HTTP surface

All resource routes sit under `/api/v1/clusters/{cluster}`.

| Resource | List and create | Get, update, delete |
| --- | --- | --- |
| Atespace | `GET/POST /atespaces` | `GET/PUT/DELETE /atespaces/{name}` |
| Actor template | `GET/POST /actor-templates` | `GET/PUT/DELETE /actor-templates/{atespace}/{name}` |
| Actor | `GET/POST /actors` | `GET/PUT/DELETE /actors/{atespace}/{name}` |
| Worker | `GET/POST /workers` | `GET/PUT/DELETE /workers/{name}` |

`GET /atespaces` style lists accept `?atespace=` on templates and actors. Deletes accept `?uid=&version=` preconditions. `GET /overview` is the dashboard summary. `POST /clusters/{cluster}/probe` checks the control plane.

A new backend implements `substrate.Cluster` and is selected from `registry.build`.
