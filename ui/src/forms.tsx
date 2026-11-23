import { useEffect, useState, type FormEvent } from "react";
import {
  api,
  formatLabels,
  parseLabels,
  type Actor,
  type ActorTemplate,
  type Atespace,
  type Worker,
} from "./api";
import { Field } from "./widgets";

const DIGEST = "sha256:9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08";

export function AtespaceForm({
  clusterId,
  initial,
  onDone,
}: {
  clusterId: string;
  initial?: Atespace;
  onDone: () => void;
}) {
  const [name, setName] = useState(initial?.metadata.name ?? "");
  const [description, setDescription] = useState(initial?.description ?? "");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      if (initial) {
        await api.updateAtespace(clusterId, initial.metadata.name, {
          ...initial,
          description,
        });
      } else {
        await api.createAtespace(clusterId, { metadata: { name }, description });
      }
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the atespace");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      {error ? <div className="form-error">{error}</div> : null}
      <Field label="Name">
        <input value={name} required disabled={!!initial} pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" onChange={(e) => setName(e.target.value)} placeholder="field" />
      </Field>
      <Field label="Description">
        <textarea value={description} onChange={(e) => setDescription(e.target.value)} placeholder="What this boundary is for" />
      </Field>
      <button className="btn primary" disabled={busy}>{busy ? "Saving…" : initial ? "Save changes" : "Create atespace"}</button>
    </form>
  );
}

export function TemplateForm({
  clusterId,
  initial,
  onDone,
}: {
  clusterId: string;
  initial?: ActorTemplate;
  onDone: () => void;
}) {
  const [spaces, setSpaces] = useState<Atespace[]>([]);
  const [atespace, setAtespace] = useState(initial?.metadata.atespace ?? "");
  const [name, setName] = useState(initial?.metadata.name ?? "");
  const [container, setContainer] = useState(initial?.containers?.[0]?.name ?? "agent");
  const [image, setImage] = useState(initial?.containers?.[0]?.image ?? `registry.example.com/agents/app@${DIGEST}`);
  const [cpu, setCpu] = useState(String(initial?.resources?.limits.cpuMillicores ?? 1000));
  const [memory, setMemory] = useState(String(initial?.resources?.limits.memoryBytes ?? 1073741824));
  const [labels, setLabels] = useState(formatLabels(initial?.workerSelector?.matchLabels));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.atespaces(clusterId).then((res) => {
      setSpaces(res.items);
      if (!initial && !atespace && res.items[0]) setAtespace(res.items[0].metadata.name);
    }).catch((err: Error) => setError(err.message));
  }, [clusterId, initial, atespace]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    const matchLabels = parseLabels(labels);
    const body: ActorTemplate = {
      ...(initial ?? { metadata: { name, atespace } }),
      metadata: { ...(initial?.metadata ?? { name }), name, atespace },
      containers: [{ ...(initial?.containers?.[0] ?? { name: container, image }), name: container, image }],
      resources: { limits: { cpuMillicores: Number(cpu), memoryBytes: Number(memory) } },
      ...(Object.keys(matchLabels).length ? { workerSelector: { matchLabels } } : { workerSelector: undefined }),
    };
    try {
      if (initial) await api.updateTemplate(clusterId, body);
      else await api.createTemplate(clusterId, body);
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the template");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      {error ? <div className="form-error">{error}</div> : null}
      <Field label="Atespace">
        <select value={atespace} required disabled={!!initial} onChange={(e) => setAtespace(e.target.value)}>
          <option value="">Select an atespace</option>
          {spaces.map((space) => (
            <option key={space.metadata.name} value={space.metadata.name}>{space.metadata.name}</option>
          ))}
        </select>
      </Field>
      <Field label="Name">
        <input value={name} required disabled={!!initial} pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" onChange={(e) => setName(e.target.value)} placeholder="research-agent" />
      </Field>
      <Field label="Container">
        <input value={container} required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" onChange={(e) => setContainer(e.target.value)} />
      </Field>
      <Field label="Image">
        <input value={image} required onChange={(e) => setImage(e.target.value)} />
      </Field>
      <Field label="CPU millicores">
        <input value={cpu} required type="number" min={1} onChange={(e) => setCpu(e.target.value)} />
      </Field>
      <div className="chips">
        {[500, 1000, 2000, 4000].map((n) => (
          <button type="button" key={n} onClick={() => setCpu(String(n))}>{n}</button>
        ))}
      </div>
      <Field label="Memory bytes">
        <input value={memory} required type="number" min={1} onChange={(e) => setMemory(e.target.value)} />
      </Field>
      <div className="chips">
        <button type="button" onClick={() => setMemory(String(512 * 1024 * 1024))}>512 MiB</button>
        <button type="button" onClick={() => setMemory(String(1024 * 1024 * 1024))}>1 GiB</button>
        <button type="button" onClick={() => setMemory(String(2 * 1024 * 1024 * 1024))}>2 GiB</button>
        <button type="button" onClick={() => setMemory(String(4 * 1024 * 1024 * 1024))}>4 GiB</button>
      </div>
      <Field label="Worker selector">
        <textarea value={labels} onChange={(e) => setLabels(e.target.value)} placeholder={"sandbox=gvisor\ntier=1"} />
      </Field>
      <button className="btn primary" disabled={busy}>{busy ? "Saving…" : initial ? "Save template" : "Create template"}</button>
    </form>
  );
}

export function ActorForm({
  clusterId,
  initial,
  onDone,
}: {
  clusterId: string;
  initial?: Actor;
  onDone: () => void;
}) {
  const [spaces, setSpaces] = useState<Atespace[]>([]);
  const [templates, setTemplates] = useState<ActorTemplate[]>([]);
  const [atespace, setAtespace] = useState(initial?.metadata.atespace ?? "");
  const [name, setName] = useState(initial?.metadata.name ?? "");
  const [template, setTemplate] = useState(initial?.actorTemplate?.name ?? "");
  const [labels, setLabels] = useState(formatLabels(initial?.workerSelector?.matchLabels));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    api.atespaces(clusterId).then((res) => {
      setSpaces(res.items);
      if (!initial && !atespace && res.items[0]) setAtespace(res.items[0].metadata.name);
    }).catch((err: Error) => setError(err.message));
  }, [clusterId, initial, atespace]);

  useEffect(() => {
    if (!atespace) return;
    api.templates(clusterId, atespace).then((res) => {
      setTemplates(res.items);
      if (!initial && !res.items.some((item) => item.metadata.name === template) && res.items[0]) {
        setTemplate(res.items[0].metadata.name);
      }
    }).catch((err: Error) => setError(err.message));
  }, [clusterId, atespace, initial, template]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    const matchLabels = parseLabels(labels);
    const body: Actor = {
      ...(initial ?? { metadata: { name, atespace } }),
      metadata: { ...(initial?.metadata ?? { name }), name, atespace },
      actorTemplate: { atespace, name: template },
      workerSelector: Object.keys(matchLabels).length ? { matchLabels } : undefined,
    };
    try {
      if (initial) await api.updateActor(clusterId, body);
      else await api.createActor(clusterId, body);
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the actor");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      {error ? <div className="form-error">{error}</div> : null}
      <Field label="Atespace">
        <select value={atespace} required disabled={!!initial} onChange={(e) => { setAtespace(e.target.value); setTemplate(""); }}>
          <option value="">Select an atespace</option>
          {spaces.map((space) => (
            <option key={space.metadata.name} value={space.metadata.name}>{space.metadata.name}</option>
          ))}
        </select>
      </Field>
      <Field label="Name">
        <input value={name} required disabled={!!initial} pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" onChange={(e) => setName(e.target.value)} placeholder="oriole" />
      </Field>
      <Field label="Actor template">
        <select value={template} required disabled={!!initial} onChange={(e) => setTemplate(e.target.value)}>
          <option value="">Select a template</option>
          {templates.map((item) => (
            <option key={item.metadata.name} value={item.metadata.name}>{item.metadata.name}</option>
          ))}
        </select>
      </Field>
      <Field label="Worker selector">
        <textarea value={labels} onChange={(e) => setLabels(e.target.value)} placeholder={"tier=1"} />
      </Field>
      <button className="btn primary" disabled={busy}>{busy ? "Saving…" : initial ? "Save actor" : "Create actor"}</button>
    </form>
  );
}

export function WorkerForm({
  clusterId,
  initial,
  onDone,
}: {
  clusterId: string;
  initial?: Worker;
  onDone: () => void;
}) {
  const [name, setName] = useState(initial?.metadata.name ?? "");
  const [namespace, setNamespace] = useState(initial?.workerNamespace ?? "substrate-workers");
  const [sandbox, setSandbox] = useState(initial?.status?.sandboxClasses?.[0]?.sandboxClass ?? "gvisor");
  const [pool, setPool] = useState(initial?.labels?.pool ?? "general");
  const [labels, setLabels] = useState(formatLabels(initial?.labels));
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    const parsed = parseLabels(labels);
    if (pool) parsed.pool = pool;
    if (sandbox) parsed.sandbox = sandbox;
    try {
      if (initial) {
        await api.updateWorker(clusterId, { ...initial, labels: parsed });
      } else {
        await api.createWorker(clusterId, {
          metadata: { name },
          workerNamespace: namespace,
          labels: parsed,
          status: {
            state: "WORKER_STATE_ACTIVE",
            sandboxClasses: [{ sandboxClass: sandbox }],
            capacity: { cpuMillicores: 4000, memoryBytes: 8 * 1024 * 1024 * 1024, actors: 1000 },
          },
        });
      }
      onDone();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not save the worker");
    } finally {
      setBusy(false);
    }
  }

  return (
    <form onSubmit={submit}>
      {error ? <div className="form-error">{error}</div> : null}
      <Field label="Name">
        <input value={name} required disabled={!!initial} pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" onChange={(e) => setName(e.target.value)} placeholder="worker-a7f3" />
      </Field>
      <Field label="Kubernetes namespace">
        <input value={namespace} required disabled={!!initial} onChange={(e) => setNamespace(e.target.value)} />
      </Field>
      <Field label="Sandbox class">
        <select value={sandbox} disabled={!!initial} onChange={(e) => setSandbox(e.target.value)}>
          <option value="gvisor">gvisor</option>
          <option value="microvm">microvm</option>
        </select>
      </Field>
      <Field label="Pool label">
        <input value={pool} disabled={!!initial} onChange={(e) => setPool(e.target.value)} />
      </Field>
      <Field label="Labels">
        <textarea value={labels} onChange={(e) => setLabels(e.target.value)} placeholder={"tier=1"} />
      </Field>
      <p className="hint">{initial ? "On a live cluster, only labels can change." : "A new worker starts active in this console. A live cluster registers it unavailable until the worker process reports in."}</p>
      <button className="btn primary" disabled={busy}>{busy ? "Saving…" : initial ? "Save labels" : "Create worker"}</button>
    </form>
  );
}
