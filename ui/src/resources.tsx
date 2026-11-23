import { useEffect, useState, type ReactNode } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import {
  api,
  formatBytes,
  formatCPU,
  formatLabels,
  formatWhen,
  type Actor,
  type ActorTemplate,
  type Atespace,
  type Worker,
} from "./api";
import { ActorActionsPanel } from "./actor-actions";
import { ActorForm, AtespaceForm, TemplateForm, WorkerForm } from "./forms";
import { useCluster } from "./shell";
import { Banner, Drawer, Pill } from "./widgets";

export function AtespacesPage() {
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [items, setItems] = useState<Atespace[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [query, setQuery] = useState("");
  const [open, setOpen] = useState(false);

  function load() {
    api.atespaces(clusterId).then((res) => setItems(res.items)).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId]);
  const shown = items.filter((item) => item.metadata.name.includes(query.toLowerCase()));

  return (
    <>
      <PageHead kicker="Global" title="Atespaces" copy="The isolation boundary an actor belongs to for its whole life." action="Create atespace" onAction={() => setOpen(true)} />
      <Banner message={error} />
      <input className="search" placeholder="Filter by name" value={query} onChange={(e) => setQuery(e.target.value)} />
      <div className="panel" style={{ marginTop: 14 }}>
        <table>
          <thead><tr><th>Name</th><th>Description</th><th>Updated</th></tr></thead>
          <tbody>
            {shown.map((item) => (
              <tr key={item.metadata.name} className="clickable" onClick={() => navigate(`/atespaces/${item.metadata.name}`)}>
                <td className="name">{item.metadata.name}</td>
                <td>{item.description || "—"}</td>
                <td className="sub">{formatWhen(item.metadata.updateTime)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {shown.length === 0 ? <p className="empty">No atespaces match.</p> : null}
      </div>
      {open ? (
        <Drawer title="New atespace" lede="Names are DNS labels and cannot be changed later." onClose={() => setOpen(false)}>
          <AtespaceForm clusterId={clusterId} onDone={() => { setOpen(false); load(); }} />
        </Drawer>
      ) : null}
    </>
  );
}

export function AtespaceDetail() {
  const { name = "" } = useParams();
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [item, setItem] = useState<Atespace | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);

  function load() {
    api.atespace(clusterId, name).then(setItem).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, name]);

  return (
    <DetailFrame
      back="/atespaces"
      kicker="Atespace"
      title={name}
      error={error}
      onEdit={() => setEditing(true)}
      confirming={confirming}
      onAskDelete={() => setConfirming(true)}
      onCancelDelete={() => setConfirming(false)}
      onDelete={async () => {
        if (!item) return;
        await api.deleteAtespace(clusterId, item);
        navigate("/atespaces");
      }}
    >
      {item ? (
        <div className="detail">
          <dl className="kv">
            <dt>Description</dt><dd>{item.description || "—"}</dd>
            <dt>UID</dt><dd className="mono">{item.metadata.uid}</dd>
            <dt>Version</dt><dd>{item.metadata.version}</dd>
            <dt>Created</dt><dd>{formatWhen(item.metadata.createTime)}</dd>
            <dt>Updated</dt><dd>{formatWhen(item.metadata.updateTime)}</dd>
          </dl>
          <div className="row-actions">
            <Link className="text-btn" to={`/templates?atespace=${item.metadata.name}`}>View templates</Link>
            <Link className="text-btn" to={`/actors?atespace=${item.metadata.name}`}>View actors</Link>
          </div>
        </div>
      ) : null}
      {editing && item ? (
        <Drawer title="Edit atespace" lede="Description is kept by this console. A live cluster without UpdateAtespace will refuse the save." onClose={() => setEditing(false)}>
          <AtespaceForm clusterId={clusterId} initial={item} onDone={() => { setEditing(false); load(); }} />
        </Drawer>
      ) : null}
    </DetailFrame>
  );
}

export function TemplatesPage() {
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [items, setItems] = useState<ActorTemplate[]>([]);
  const [spaces, setSpaces] = useState<Atespace[]>([]);
  const [space, setSpace] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);

  function load() {
    api.templates(clusterId, space).then((res) => setItems(res.items)).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, space]);
  useEffect(() => {
    api.atespaces(clusterId).then((res) => setSpaces(res.items)).catch(() => setSpaces([]));
  }, [clusterId]);

  return (
    <>
      <PageHead kicker="Atespace scoped" title="Actor templates" copy="Immutable on a live cluster. The demo cluster still accepts an update." action="Create template" onAction={() => setOpen(true)} />
      <Banner message={error} />
      <div className="toolbar">
        <select className="search" value={space} onChange={(e) => setSpace(e.target.value)} aria-label="Filter by atespace">
          <option value="">All atespaces</option>
          {spaces.map((item) => <option key={item.metadata.name} value={item.metadata.name}>{item.metadata.name}</option>)}
        </select>
      </div>
      <div className="panel">
        <table>
          <thead><tr><th>Template</th><th>Image</th><th>Size</th></tr></thead>
          <tbody>
            {items.map((item) => (
              <tr key={`${item.metadata.atespace}/${item.metadata.name}`} className="clickable" onClick={() => navigate(`/templates/${item.metadata.atespace}/${item.metadata.name}`)}>
                <td><div className="name">{item.metadata.name}</div><div className="sub">{item.metadata.atespace}</div></td>
                <td className="mono">{item.containers?.[0]?.image}</td>
                <td>{formatCPU(item.resources?.limits.cpuMillicores)} · {formatBytes(item.resources?.limits.memoryBytes)}</td>
              </tr>
            ))}
          </tbody>
        </table>
        {items.length === 0 ? <p className="empty">No templates in this view.</p> : null}
      </div>
      {open ? (
        <Drawer title="New actor template" lede="Pin the image by digest. Select the pools with labels." onClose={() => setOpen(false)}>
          <TemplateForm clusterId={clusterId} onDone={() => { setOpen(false); load(); }} />
        </Drawer>
      ) : null}
    </>
  );
}

export function TemplateDetail() {
  const { atespace = "", name = "" } = useParams();
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [item, setItem] = useState<ActorTemplate | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  function load() {
    api.template(clusterId, atespace, name).then(setItem).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, atespace, name]);
  const container = item?.containers?.[0];
  return (
    <DetailFrame back="/templates" kicker="Actor template" title={name} error={error} onEdit={() => setEditing(true)} confirming={confirming} onAskDelete={() => setConfirming(true)} onCancelDelete={() => setConfirming(false)} onDelete={async () => { if (!item) return; await api.deleteTemplate(clusterId, item); navigate("/templates"); }}>
      {item ? (
        <div className="detail-grid">
          <div className="detail">
            <h3>Spec</h3>
            <dl className="kv">
              <dt>Atespace</dt><dd>{item.metadata.atespace}</dd>
              <dt>Image</dt><dd className="mono">{container?.image}</dd>
              <dt>Container</dt><dd>{container?.name}</dd>
              <dt>CPU</dt><dd>{formatCPU(item.resources?.limits.cpuMillicores)}</dd>
              <dt>Memory</dt><dd>{formatBytes(item.resources?.limits.memoryBytes)}</dd>
              <dt>Selector</dt><dd className="mono">{formatLabels(item.workerSelector?.matchLabels) || "any pool"}</dd>
            </dl>
          </div>
          <div className="detail">
            <h3>Status</h3>
            <dl className="kv">
              <dt>Golden tag</dt><dd>{item.status?.goldenSnapshotStatus?.goldenTag?.name || "—"}</dd>
              <dt>Snapshot</dt><dd>{formatWhen(item.status?.goldenSnapshotStatus?.snapshotTime)}</dd>
              <dt>Version</dt><dd>{item.metadata.version}</dd>
              <dt>UID</dt><dd className="mono">{item.metadata.uid}</dd>
            </dl>
          </div>
        </div>
      ) : null}
      {editing && item ? (
        <Drawer title="Edit template" lede="A current Substrate cluster will reject this. The demo cluster replaces the spec." onClose={() => setEditing(false)}>
          <TemplateForm clusterId={clusterId} initial={item} onDone={() => { setEditing(false); load(); }} />
        </Drawer>
      ) : null}
    </DetailFrame>
  );
}

export function ActorsPage() {
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [items, setItems] = useState<Actor[]>([]);
  const [spaces, setSpaces] = useState<Atespace[]>([]);
  const [space, setSpace] = useState(new URLSearchParams(window.location.search).get("atespace") || "");
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  function load() {
    api.actors(clusterId, space).then((res) => setItems(res.items)).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, space]);
  useEffect(() => {
    api.atespaces(clusterId).then((res) => setSpaces(res.items)).catch(() => setSpaces([]));
  }, [clusterId]);
  return (
    <>
      <PageHead kicker="Atespace scoped" title="Actors" copy="An actor is addressed by atespace and name. New actors start suspended." action="Create actor" onAction={() => setOpen(true)} />
      <Banner message={error} />
      <div className="toolbar">
        <select className="search" value={space} onChange={(e) => setSpace(e.target.value)} aria-label="Filter by atespace">
          <option value="">All atespaces</option>
          {spaces.map((item) => <option key={item.metadata.name} value={item.metadata.name}>{item.metadata.name}</option>)}
        </select>
      </div>
      <div className="panel">
        <table>
          <thead><tr><th>Actor</th><th>Template</th><th>Worker</th><th>State</th><th>Actions</th></tr></thead>
          <tbody>
            {items.map((item) => (
              <tr key={`${item.metadata.atespace}/${item.metadata.name}`} className="clickable" onClick={() => navigate(`/actors/${item.metadata.atespace}/${item.metadata.name}`)}>
                <td><div className="name">{item.metadata.name}</div><div className="sub">{item.metadata.atespace}</div></td>
                <td>{item.actorTemplate?.name}</td>
                <td>{item.status?.workerAssignment?.worker?.name || "—"}</td>
                <td><Pill value={item.status?.state} /></td>
                <td>
                  <div className="row-actions" onClick={(e) => e.stopPropagation()}>
                    <button
                      className="text-btn"
                      type="button"
                      onClick={() => navigate(`/actors/${item.metadata.atespace}/${item.metadata.name}?action=route`)}
                    >
                      Route
                    </button>
                    <button
                      className="text-btn"
                      type="button"
                      onClick={() => navigate(`/actors/${item.metadata.atespace}/${item.metadata.name}?action=execute`)}
                    >
                      HTTP
                    </button>
                    <button
                      className="text-btn"
                      type="button"
                      onClick={() => navigate(`/actors/${item.metadata.atespace}/${item.metadata.name}?action=command`)}
                    >
                      Command
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {items.length === 0 ? <p className="empty">No actors in this view.</p> : null}
      </div>
      {open ? (
        <Drawer title="New actor" lede="The template must already exist in the same atespace." onClose={() => setOpen(false)}>
          <ActorForm clusterId={clusterId} onDone={() => { setOpen(false); load(); }} />
        </Drawer>
      ) : null}
    </>
  );
}

export function ActorDetail() {
  const { atespace = "", name = "" } = useParams();
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [item, setItem] = useState<Actor | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  function load() {
    api.actor(clusterId, atespace, name).then(setItem).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, atespace, name]);
  const assignment = item?.status?.workerAssignment;
  return (
    <DetailFrame back="/actors" kicker={`${atespace}`} title={name} error={error} extra={item ? <Pill value={item.status?.state} /> : null} onEdit={() => setEditing(true)} confirming={confirming} onAskDelete={() => setConfirming(true)} onCancelDelete={() => setConfirming(false)} onDelete={async () => { if (!item) return; await api.deleteActor(clusterId, item); navigate("/actors"); }}>
      {item ? (
        <>
          <div className="detail-grid">
            <div className="detail">
              <h3>Identity</h3>
              <dl className="kv">
                <dt>Template</dt><dd><Link to={`/templates/${item.actorTemplate?.atespace}/${item.actorTemplate?.name}`}>{item.actorTemplate?.name}</Link></dd>
                <dt>Selector</dt><dd className="mono">{formatLabels(item.workerSelector?.matchLabels) || "template default"}</dd>
                <dt>Version</dt><dd>{item.metadata.version}</dd>
                <dt>UID</dt><dd className="mono">{item.metadata.uid}</dd>
              </dl>
            </div>
            <div className="detail">
              <h3>Placement</h3>
              <dl className="kv">
                <dt>Worker</dt><dd>{assignment?.worker?.name ? <Link to={`/workers/${assignment.worker.name}`}>{assignment.worker.name}</Link> : "—"}</dd>
                <dt>Pool</dt><dd>{assignment?.workerPool || "—"}</dd>
                <dt>Node</dt><dd>{assignment?.nodeName || item.status?.assignedNode || "—"}</dd>
                <dt>Sandbox</dt><dd>{assignment?.sandboxRuntime?.sandboxClass || "—"}</dd>
                <dt>Crash</dt><dd>{item.status?.crash?.message || "—"}</dd>
              </dl>
            </div>
          </div>
          <ActorActionsPanel clusterId={clusterId} atespace={atespace} name={name} />
        </>
      ) : null}
      {editing && item ? (
        <Drawer title="Edit actor" lede="workerSelector is the mutable field. It takes effect the next time the actor resumes." onClose={() => setEditing(false)}>
          <ActorForm clusterId={clusterId} initial={item} onDone={() => { setEditing(false); load(); }} />
        </Drawer>
      ) : null}
    </DetailFrame>
  );
}

export function WorkersPage() {
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [items, setItems] = useState<Worker[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [open, setOpen] = useState(false);
  function load() {
    api.workers(clusterId).then((res) => setItems(res.items)).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId]);
  return (
    <>
      <PageHead kicker="Global" title="Workers" copy="Workers belong to no atespace. They host whichever actors the scheduler places." action="Create worker" onAction={() => setOpen(true)} />
      <Banner message={error} />
      <div className="panel">
        <table>
          <thead><tr><th>Worker</th><th>Sandbox</th><th>CPU</th><th>State</th></tr></thead>
          <tbody>
            {items.map((item) => (
              <tr key={item.metadata.name} className="clickable" onClick={() => navigate(`/workers/${item.metadata.name}`)}>
                <td><div className="name">{item.metadata.name}</div><div className="sub">{item.workerNamespace}</div></td>
                <td>{item.status?.sandboxClasses?.[0]?.sandboxClass || item.labels?.sandbox || "—"}</td>
                <td>{formatCPU(item.status?.allocated?.cpuMillicores)} / {formatCPU(item.status?.capacity?.cpuMillicores)}</td>
                <td><Pill value={item.status?.state} /></td>
              </tr>
            ))}
          </tbody>
        </table>
        {items.length === 0 ? <p className="empty">No workers registered.</p> : null}
      </div>
      {open ? (
        <Drawer title="New worker" lede="Labels are what actor templates select on." onClose={() => setOpen(false)}>
          <WorkerForm clusterId={clusterId} onDone={() => { setOpen(false); load(); }} />
        </Drawer>
      ) : null}
    </>
  );
}

export function WorkerDetail() {
  const { name = "" } = useParams();
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [item, setItem] = useState<Worker | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [editing, setEditing] = useState(false);
  const [confirming, setConfirming] = useState(false);
  function load() {
    api.worker(clusterId, name).then(setItem).catch((err: Error) => setError(err.message));
  }
  useEffect(load, [clusterId, name]);
  return (
    <DetailFrame back="/workers" kicker="Worker" title={name} error={error} extra={item ? <Pill value={item.status?.state} /> : null} onEdit={() => setEditing(true)} confirming={confirming} onAskDelete={() => setConfirming(true)} onCancelDelete={() => setConfirming(false)} onDelete={async () => { if (!item) return; await api.deleteWorker(clusterId, item); navigate("/workers"); }}>
      {item ? (
        <div className="detail-grid">
          <div className="detail">
            <h3>Host</h3>
            <dl className="kv">
              <dt>Namespace</dt><dd>{item.workerNamespace}</dd>
              <dt>Pool</dt><dd>{item.workerPool || "—"}</dd>
              <dt>Pod</dt><dd className="mono">{item.workerPod || "—"}</dd>
              <dt>Node</dt><dd>{item.nodeName || "—"}</dd>
              <dt>SPIFFE</dt><dd className="mono">{item.workerSpiffeId || "—"}</dd>
              <dt>IPs</dt><dd>{item.status?.ips?.join(", ") || "—"}</dd>
              <dt>Labels</dt><dd className="mono">{formatLabels(item.labels) || "—"}</dd>
            </dl>
          </div>
          <div className="detail">
            <h3>Capacity</h3>
            <dl className="kv">
              <dt>CPU</dt><dd>{formatCPU(item.status?.allocated?.cpuMillicores)} of {formatCPU(item.status?.capacity?.cpuMillicores)}</dd>
              <dt>Memory</dt><dd>{formatBytes(item.status?.allocated?.memoryBytes)} of {formatBytes(item.status?.capacity?.memoryBytes)}</dd>
              <dt>Actors</dt><dd>{item.status?.allocated?.actors || 0} of {item.status?.capacity?.actors || 0}</dd>
              <dt>Sandbox</dt><dd>{item.status?.sandboxClasses?.[0]?.sandboxClass || "—"}</dd>
            </dl>
          </div>
        </div>
      ) : null}
      {editing && item ? (
        <Drawer title="Edit worker" lede="Labels are the only field an update may change." onClose={() => setEditing(false)}>
          <WorkerForm clusterId={clusterId} initial={item} onDone={() => { setEditing(false); load(); }} />
        </Drawer>
      ) : null}
    </DetailFrame>
  );
}

function PageHead({ kicker, title, copy, action, onAction }: { kicker: string; title: string; copy: string; action: string; onAction: () => void }) {
  return (
    <div className="page-head">
      <div>
        <p className="eyebrow">{kicker}</p>
        <h1>{title}</h1>
        <p>{copy}</p>
      </div>
      <button className="btn primary" onClick={onAction}>{action}</button>
    </div>
  );
}

function DetailFrame({
  back, kicker, title, error, extra, children, onEdit, confirming, onAskDelete, onCancelDelete, onDelete,
}: {
  back: string;
  kicker: string;
  title: string;
  error: string | null;
  extra?: ReactNode;
  children: ReactNode;
  onEdit: () => void;
  confirming: boolean;
  onAskDelete: () => void;
  onCancelDelete: () => void;
  onDelete: () => Promise<void>;
}) {
  const [busy, setBusy] = useState(false);
  const [localError, setLocalError] = useState<string | null>(null);
  return (
    <>
      <Link className="back" to={back}>← Back</Link>
      <div className="page-head" style={{ marginTop: 10 }}>
        <div>
          <p className="eyebrow">{kicker}</p>
          <h1>{title}</h1>
          <div style={{ marginTop: 8 }}>{extra}</div>
        </div>
        <div className="row-actions">
          <button className="text-btn" onClick={onEdit}>Edit</button>
          {confirming ? (
            <>
              <button className="text-btn" onClick={onCancelDelete}>Cancel</button>
              <button className="btn danger" disabled={busy} onClick={() => {
                setBusy(true);
                onDelete().catch((err: Error) => { setLocalError(err.message); setBusy(false); });
              }}>Delete</button>
            </>
          ) : (
            <button className="btn danger" onClick={onAskDelete}>Delete</button>
          )}
        </div>
      </div>
      <Banner message={error || localError} />
      {children}
    </>
  );
}
