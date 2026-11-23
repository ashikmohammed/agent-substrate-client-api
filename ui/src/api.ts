export type Metadata = {
  atespace?: string;
  name: string;
  uid?: string;
  version?: number;
  createTime?: string;
  updateTime?: string;
};

export type ObjectRef = { atespace?: string; name: string };
export type Selector = { matchLabels?: Record<string, string> };

export type Atespace = {
  metadata: Metadata;
  description?: string;
};

export type Container = {
  name: string;
  image: string;
  command?: string[];
  args?: string[];
  env?: { name: string; value?: string }[];
};

export type Limits = { cpuMillicores?: number; memoryBytes?: number };

export type ActorTemplate = {
  metadata: Metadata;
  containers?: Container[];
  workerSelector?: Selector;
  resources?: { limits: Limits };
  status?: {
    goldenSnapshotStatus?: {
      goldenTag?: ObjectRef;
      snapshotTime?: string;
      errorMessage?: string;
    };
  };
};

export type Actor = {
  metadata: Metadata;
  actorTemplate?: ObjectRef;
  workerSelector?: Selector;
  status?: {
    state?: string;
    assignedNode?: string;
    workerAssignment?: {
      worker?: ObjectRef;
      workerNamespace?: string;
      workerPool?: string;
      workerPod?: string;
      nodeName?: string;
      workerPodIps?: string[];
      sandboxRuntime?: { sandboxClass?: string };
    };
    crash?: { message?: string; crashTime?: string };
  };
};

export type Worker = {
  metadata: Metadata;
  workerNamespace?: string;
  workerPool?: string;
  workerPod?: string;
  workerPodUid?: string;
  nodeName?: string;
  workerSpiffeId?: string;
  labels?: Record<string, string>;
  status?: {
    state?: string;
    capacity?: Limits & { actors?: number };
    allocated?: Limits & { actors?: number };
    sandboxClasses?: { sandboxClass?: string }[];
    ips?: string[];
  };
};

export type PublicCluster = {
  id: string;
  name: string;
  driver: string;
  endpoint?: string;
  tls: boolean;
  insecureSkipVerify: boolean;
  serverName?: string;
  kubeContext?: string;
  systemNamespace?: string;
  hasToken: boolean;
};

export type Overview = {
  cluster: PublicCluster;
  counts: { atespaces: number; actorTemplates: number; actors: number; workers: number };
  actorsByState: Record<string, number>;
  workers: Worker[];
  atespaces: Atespace[];
};

export type Probe = { ok: boolean; service?: string; latencyMs: number; detail?: string };

export type RouteSession = {
  id: string;
  atespace: string;
  actor: string;
  localUrl: string;
  localPort: number;
  remotePort: number;
  status: string;
  detail?: string;
  createdAt?: string;
};

export type ExecuteRequest = {
  method?: string;
  path?: string;
  headers?: Record<string, string>;
  body?: string;
  remotePort?: number;
  timeoutMs?: number;
};

export type ExecuteResult = {
  status: number;
  headers?: Record<string, string>;
  body: string;
  latencyMs: number;
  localUrl?: string;
  routeId?: string;
};

export type CommandRequest = {
  shell?: string;
  shellBin?: string;
  command?: string[];
  envvars?: Record<string, string>;
  cwd?: string;
  timeout?: string;
  path?: string;
  remotePort?: number;
  timeoutMs?: number;
};

export type CommandResult = {
  stdout: string;
  stderr: string;
  exitCode: number;
  error?: string;
  latencyMs: number;
  localUrl?: string;
  routeId?: string;
  rawBody?: string;
};

export class ApiError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.status = status;
  }
}

async function request<T>(path: string, init?: RequestInit): Promise<T> {
  const res = await fetch(path, {
    ...init,
    headers: {
      Accept: "application/json",
      ...(init?.body ? { "Content-Type": "application/json" } : {}),
      ...init?.headers,
    },
  });
  const text = await res.text();
  const data = text ? JSON.parse(text) : null;
  if (!res.ok) {
    const message = data?.error?.message || res.statusText || "request failed";
    throw new ApiError(res.status, message);
  }
  return data as T;
}

const j = (body: unknown) => JSON.stringify(body);

export const api = {
  clusters: () => request<{ items: PublicCluster[]; default?: string }>("/api/v1/clusters"),
  createCluster: (body: unknown) => request<PublicCluster>("/api/v1/clusters", { method: "POST", body: j(body) }),
  deleteCluster: (id: string) => request<void>(`/api/v1/clusters/${id}`, { method: "DELETE" }),
  probe: (id: string) => request<Probe>(`/api/v1/clusters/${id}/probe`, { method: "POST", body: "{}" }),
  overview: (id: string) => request<Overview>(`/api/v1/clusters/${id}/overview`),

  atespaces: (id: string) => request<{ items: Atespace[] }>(`/api/v1/clusters/${id}/atespaces`),
  atespace: (id: string, name: string) => request<Atespace>(`/api/v1/clusters/${id}/atespaces/${name}`),
  createAtespace: (id: string, body: unknown) =>
    request<Atespace>(`/api/v1/clusters/${id}/atespaces`, { method: "POST", body: j(body) }),
  updateAtespace: (id: string, name: string, body: unknown) =>
    request<Atespace>(`/api/v1/clusters/${id}/atespaces/${name}`, { method: "PUT", body: j(body) }),
  deleteAtespace: (id: string, item: Atespace) =>
    request<Atespace>(precondition(`/api/v1/clusters/${id}/atespaces/${item.metadata.name}`, item), { method: "DELETE" }),

  templates: (id: string, atespace = "") =>
    request<{ items: ActorTemplate[] }>(`/api/v1/clusters/${id}/actor-templates${atespace ? `?atespace=${encodeURIComponent(atespace)}` : ""}`),
  template: (id: string, atespace: string, name: string) =>
    request<ActorTemplate>(`/api/v1/clusters/${id}/actor-templates/${atespace}/${name}`),
  createTemplate: (id: string, body: unknown) =>
    request<ActorTemplate>(`/api/v1/clusters/${id}/actor-templates`, { method: "POST", body: j(body) }),
  updateTemplate: (id: string, item: ActorTemplate) =>
    request<ActorTemplate>(`/api/v1/clusters/${id}/actor-templates/${item.metadata.atespace}/${item.metadata.name}`, {
      method: "PUT",
      body: j(item),
    }),
  deleteTemplate: (id: string, item: ActorTemplate) =>
    request<ActorTemplate>(
      precondition(`/api/v1/clusters/${id}/actor-templates/${item.metadata.atespace}/${item.metadata.name}`, item),
      { method: "DELETE" },
    ),

  actors: (id: string, atespace = "") =>
    request<{ items: Actor[] }>(`/api/v1/clusters/${id}/actors${atespace ? `?atespace=${encodeURIComponent(atespace)}` : ""}`),
  actor: (id: string, atespace: string, name: string) =>
    request<Actor>(`/api/v1/clusters/${id}/actors/${atespace}/${name}`),
  createActor: (id: string, body: unknown) =>
    request<Actor>(`/api/v1/clusters/${id}/actors`, { method: "POST", body: j(body) }),
  updateActor: (id: string, item: Actor) =>
    request<Actor>(`/api/v1/clusters/${id}/actors/${item.metadata.atespace}/${item.metadata.name}`, {
      method: "PUT",
      body: j(item),
    }),
  deleteActor: (id: string, item: Actor) =>
    request<Actor>(precondition(`/api/v1/clusters/${id}/actors/${item.metadata.atespace}/${item.metadata.name}`, item), {
      method: "DELETE",
    }),
  routeActor: (id: string, atespace: string, name: string, body: { localPort?: number; remotePort?: number } = {}) =>
    request<RouteSession>(`/api/v1/clusters/${id}/actors/${atespace}/${name}/route`, { method: "POST", body: j(body) }),
  actorRoute: (id: string, atespace: string, name: string) =>
    request<RouteSession>(`/api/v1/clusters/${id}/actors/${atespace}/${name}/route`),
  stopActorRoute: (id: string, atespace: string, name: string) =>
    request<void>(`/api/v1/clusters/${id}/actors/${atespace}/${name}/route`, { method: "DELETE" }),
  executeActor: (id: string, atespace: string, name: string, body: ExecuteRequest) =>
    request<ExecuteResult>(`/api/v1/clusters/${id}/actors/${atespace}/${name}/execute`, { method: "POST", body: j(body) }),
  commandActor: (id: string, atespace: string, name: string, body: CommandRequest) =>
    request<CommandResult>(`/api/v1/clusters/${id}/actors/${atespace}/${name}/command`, { method: "POST", body: j(body) }),

  workers: (id: string) => request<{ items: Worker[] }>(`/api/v1/clusters/${id}/workers`),
  worker: (id: string, name: string) => request<Worker>(`/api/v1/clusters/${id}/workers/${name}`),
  createWorker: (id: string, body: unknown) =>
    request<Worker>(`/api/v1/clusters/${id}/workers`, { method: "POST", body: j(body) }),
  updateWorker: (id: string, item: Worker) =>
    request<Worker>(`/api/v1/clusters/${id}/workers/${item.metadata.name}`, { method: "PUT", body: j(item) }),
  deleteWorker: (id: string, item: Worker) =>
    request<Worker>(precondition(`/api/v1/clusters/${id}/workers/${item.metadata.name}`, item), { method: "DELETE" }),
};

function precondition(path: string, item: { metadata: Metadata }) {
  const q = new URLSearchParams();
  if (item.metadata.uid) q.set("uid", item.metadata.uid);
  if (item.metadata.version) q.set("version", String(item.metadata.version));
  const s = q.toString();
  return s ? `${path}?${s}` : path;
}

export function stateName(value?: string) {
  if (!value) return "Unknown";
  return value
    .replace(/^ACTOR_STATE_/, "")
    .replace(/^WORKER_STATE_/, "")
    .toLowerCase()
    .replace(/^\w/, (c) => c.toUpperCase());
}

export function stateClass(value?: string) {
  const name = (value || "").toLowerCase();
  if (name.includes("running") || name.includes("active")) return "run";
  if (name.includes("crash") || name.includes("delet")) return "bad";
  if (name.includes("drain") || name.includes("suspend") || name.includes("unavail")) return "wait";
  return "idle";
}

export function formatBytes(value?: number) {
  if (!value) return "0 B";
  const units = ["B", "KiB", "MiB", "GiB", "TiB"];
  let n = value;
  let i = 0;
  while (n >= 1024 && i < units.length - 1) {
    n /= 1024;
    i += 1;
  }
  const digits = n >= 10 || i === 0 ? 0 : 1;
  return `${n.toFixed(digits)} ${units[i]}`;
}

export function formatCPU(value?: number) {
  if (!value) return "0";
  if (value % 1000 === 0) return `${value / 1000} cores`;
  return `${value} mCPU`;
}

export function formatWhen(value?: string) {
  if (!value || value.startsWith("0001")) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "—";
  return date.toLocaleString(undefined, { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

export function parseLabels(text: string) {
  const out: Record<string, string> = {};
  for (const line of text.split("\n")) {
    const trimmed = line.trim();
    if (!trimmed || trimmed.startsWith("#")) continue;
    const cut = trimmed.indexOf("=");
    if (cut <= 0) continue;
    out[trimmed.slice(0, cut).trim()] = trimmed.slice(cut + 1).trim();
  }
  return out;
}

export function formatLabels(labels?: Record<string, string>) {
  if (!labels) return "";
  return Object.entries(labels)
    .map(([key, value]) => `${key}=${value}`)
    .join("\n");
}

export function refOf(item: { metadata: Metadata }) {
  return item.metadata.atespace ? `${item.metadata.atespace}/${item.metadata.name}` : item.metadata.name;
}
