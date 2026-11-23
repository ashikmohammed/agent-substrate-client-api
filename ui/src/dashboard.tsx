import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { api, formatBytes, formatCPU, refOf, type Overview } from "./api";
import { ActorForm, AtespaceForm, TemplateForm, WorkerForm } from "./forms";
import { useCluster } from "./shell";
import { Banner, Drawer, Pill } from "./widgets";

type CreateKind = "atespace" | "template" | "actor" | "worker" | null;

export function Dashboard() {
  const { clusterId } = useCluster();
  const navigate = useNavigate();
  const [data, setData] = useState<Overview | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [create, setCreate] = useState<CreateKind>(null);

  function load() {
    setError(null);
    api.overview(clusterId).then(setData).catch((err: Error) => setError(err.message));
  }

  useEffect(load, [clusterId]);

  const counts = data?.counts;

  return (
    <>
      <header className="hero">
        <p className="eyebrow">{data ? data.cluster.name : "Cluster"}</p>
        <h1>The cluster, <em>at a glance.</em></h1>
        <p className="lede">Atespaces, actor templates, actors, and workers. Create one, or open it to see how it is placed.</p>
      </header>
      <Banner message={error} />
      <section className="stats">
        <Stat kicker="Atespace" value={counts?.atespaces} copy="Isolation boundaries" onView={() => navigate("/atespaces")} onCreate={() => setCreate("atespace")} />
        <Stat kicker="Actor template" value={counts?.actorTemplates} copy="Workload blueprints" onView={() => navigate("/templates")} onCreate={() => setCreate("template")} />
        <Stat kicker="Actor" value={counts?.actors} copy="Live and suspended agents" onView={() => navigate("/actors")} onCreate={() => setCreate("actor")} />
        <Stat kicker="Worker" value={counts?.workers} copy="Warm sandbox hosts" onView={() => navigate("/workers")} onCreate={() => setCreate("worker")} />
      </section>
      <div className="states">
        {Object.entries(data?.actorsByState ?? {}).map(([state, count]) => (
          <span key={state} className="state-count"><Pill value={state} /> {count}</span>
        ))}
      </div>
      <section className="layout">
        <div className="panel">
          <div className="panel-head">
            <h2>Actors</h2>
            <Link className="text-btn" to="/actors">View all</Link>
          </div>
          <ActorTable onOpen={(space, name) => navigate(`/actors/${space}/${name}`)} />
        </div>
        <div className="panel">
          <div className="panel-head">
            <h2>Workers</h2>
            <Link className="text-btn" to="/workers">View all</Link>
          </div>
          {(data?.workers ?? []).map((worker) => {
            const cap = worker.status?.capacity?.cpuMillicores || 0;
            const used = worker.status?.allocated?.cpuMillicores || 0;
            const pct = cap ? Math.min(100, (used / cap) * 100) : 0;
            return (
              <button key={worker.metadata.name} className="bar-row" onClick={() => navigate(`/workers/${worker.metadata.name}`)}>
                <div className="bar-meta">
                  <strong>{worker.metadata.name}</strong>
                  <Pill value={worker.status?.state} />
                </div>
                <div className="track" aria-hidden><span style={{ width: `${pct}%` }} /></div>
                <div className="sub">{formatCPU(used)} of {formatCPU(cap)} · {formatBytes(worker.status?.allocated?.memoryBytes)} memory</div>
              </button>
            );
          })}
          {data && data.workers.length === 0 ? <p className="empty">No workers registered.</p> : null}
        </div>
      </section>
      {create === "atespace" ? (
        <Drawer title="New atespace" lede="A global isolation boundary. Actors and templates live inside it." onClose={() => setCreate(null)}>
          <AtespaceForm clusterId={clusterId} onDone={() => { setCreate(null); load(); }} />
        </Drawer>
      ) : null}
      {create === "template" ? (
        <Drawer title="New actor template" lede="The image, size, and worker selector a new actor inherits." onClose={() => setCreate(null)}>
          <TemplateForm clusterId={clusterId} onDone={() => { setCreate(null); load(); }} />
        </Drawer>
      ) : null}
      {create === "actor" ? (
        <Drawer title="New actor" lede="Created suspended, ready to resume on a matching worker." onClose={() => setCreate(null)}>
          <ActorForm clusterId={clusterId} onDone={() => { setCreate(null); load(); }} />
        </Drawer>
      ) : null}
      {create === "worker" ? (
        <Drawer title="New worker" lede="A global host. Pool selection is by label, not by atespace." onClose={() => setCreate(null)}>
          <WorkerForm clusterId={clusterId} onDone={() => { setCreate(null); load(); }} />
        </Drawer>
      ) : null}
    </>
  );
}

function Stat({ kicker, value, copy, onView, onCreate }: { kicker: string; value?: number; copy: string; onView: () => void; onCreate: () => void }) {
  return (
    <article className="stat">
      <p className="eyebrow">{kicker}</p>
      <strong>{value ?? "—"}</strong>
      <p>{copy}</p>
      <div className="stat-actions">
        <button className="text-btn" onClick={onView}>View</button>
        <button className="btn primary" onClick={onCreate}>Create</button>
      </div>
    </article>
  );
}

function ActorTable({ onOpen }: { onOpen: (atespace: string, name: string) => void }) {
  const { clusterId } = useCluster();
  const [rows, setRows] = useState<{ atespace: string; name: string; template: string; state?: string }[]>([]);
  useEffect(() => {
    api.actors(clusterId).then((res) => {
      setRows(res.items.slice(0, 6).map((actor) => ({
        atespace: actor.metadata.atespace || "",
        name: actor.metadata.name,
        template: actor.actorTemplate?.name || "—",
        state: actor.status?.state,
      })));
    }).catch(() => setRows([]));
  }, [clusterId]);
  if (rows.length === 0) return <p className="empty">No actors yet.</p>;
  return (
    <table>
      <thead><tr><th>Actor</th><th>Template</th><th>State</th></tr></thead>
      <tbody>
        {rows.map((row) => (
          <tr key={refOf({ metadata: row })} className="clickable" onClick={() => onOpen(row.atespace, row.name)}>
            <td><div className="name">{row.name}</div><div className="sub">{row.atespace}</div></td>
            <td>{row.template}</td>
            <td><Pill value={row.state} /></td>
          </tr>
        ))}
      </tbody>
    </table>
  );
}
