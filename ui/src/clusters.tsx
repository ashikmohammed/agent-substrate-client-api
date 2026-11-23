import { useState, type FormEvent } from "react";
import { api, type Probe, type PublicCluster } from "./api";
import { useCluster } from "./shell";
import { Banner, Field } from "./widgets";

export function ClustersPage() {
  const { clusters, refreshClusters, setClusterId } = useCluster();
  const [error, setError] = useState<string | null>(null);
  const [probes, setProbes] = useState<Record<string, Probe>>({});
  const [busy, setBusy] = useState(false);
  const [form, setForm] = useState({
    id: "",
    name: "",
    driver: "kube",
    endpoint: "",
    token: "",
    kubeContext: "",
    systemNamespace: "ate-system",
    tls: true,
    insecureSkipVerify: false,
  });

  async function probe(id: string) {
    const result = await api.probe(id);
    setProbes((prev) => ({ ...prev, [id]: result }));
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setBusy(true);
    setError(null);
    try {
      const body =
        form.driver === "kube"
          ? {
              id: form.id,
              name: form.name || form.id,
              driver: "kube",
              kubeContext: form.kubeContext || undefined,
              systemNamespace: form.systemNamespace || "ate-system",
              tls: true,
            }
          : {
              id: form.id,
              name: form.name || form.id,
              driver: "grpc",
              endpoint: form.endpoint,
              token: form.token,
              tls: form.tls,
              insecureSkipVerify: form.insecureSkipVerify,
            };
      const created = await api.createCluster(body);
      await refreshClusters();
      setClusterId(created.id);
      setForm({
        id: "",
        name: "",
        driver: "kube",
        endpoint: "",
        token: "",
        kubeContext: "",
        systemNamespace: "ate-system",
        tls: true,
        insecureSkipVerify: false,
      });
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not add the cluster");
    } finally {
      setBusy(false);
    }
  }

  async function remove(item: PublicCluster) {
    setError(null);
    try {
      await api.deleteCluster(item.id);
      await refreshClusters();
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not delete the cluster");
    }
  }

  return (
    <>
      <div className="page-head">
        <div>
          <p className="eyebrow">Connections</p>
          <h1>Clusters</h1>
          <p>The console keeps a driver per cluster. gRPC clusters are discovered from the live control plane.</p>
        </div>
      </div>
      <Banner message={error} />
      <div className="layout">
        <div className="cluster-grid">
          {clusters.map((item) => (
            <article key={item.id} className="cluster-card">
              <header>
                <h3>{item.name}</h3>
                <span className="pill idle">{item.driver}</span>
              </header>
              <p className="sub">
                {item.driver === "kube"
                  ? `kubeconfig · ${item.kubeContext || "current"} · ${item.systemNamespace || "ate-system"}`
                  : item.endpoint || "In-process demo"}
              </p>
              <p className="sub">
                {item.driver === "kube" ? "ServiceAccount token" : item.hasToken ? "Token stored" : "No token"}
                {item.tls ? " · TLS" : ""}
              </p>
              {probes[item.id] ? (
                <p className={probes[item.id].ok ? "ok" : "bad-text"}>
                  {probes[item.id].ok ? "Reachable" : "Unreachable"} · {probes[item.id].latencyMs} ms
                  {probes[item.id].detail ? ` · ${probes[item.id].detail}` : ""}
                </p>
              ) : null}
              <div className="row-actions">
                <button className="text-btn" onClick={() => { setClusterId(item.id); }}>Use</button>
                <button className="text-btn" onClick={() => probe(item.id).catch((err: Error) => setError(err.message))}>Probe</button>
                {item.id !== "local" ? <button className="btn danger" onClick={() => remove(item)}>Delete</button> : null}
              </div>
            </article>
          ))}
        </div>
        <form className="detail" onSubmit={submit}>
          <h3>Connect a cluster</h3>
          <p className="hint">Prefer kubeconfig for a live Substrate install. The API port-forwards to ate-api-server and mints an ate-client token.</p>
          <Field label="Id"><input required pattern="[a-z0-9]([-a-z0-9]*[a-z0-9])?" value={form.id} onChange={(e) => setForm({ ...form, id: e.target.value })} placeholder="lab" /></Field>
          <Field label="Name"><input value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="Lab cluster" /></Field>
          <Field label="Driver">
            <select value={form.driver} onChange={(e) => setForm({ ...form, driver: e.target.value, tls: e.target.value === "kube" })}>
              <option value="kube">kubeconfig</option>
              <option value="grpc">direct gRPC</option>
            </select>
          </Field>
          {form.driver === "kube" ? (
            <>
              <Field label="Kube context"><input value={form.kubeContext} onChange={(e) => setForm({ ...form, kubeContext: e.target.value })} placeholder="kind-kind" /></Field>
              <Field label="System namespace"><input value={form.systemNamespace} onChange={(e) => setForm({ ...form, systemNamespace: e.target.value })} placeholder="ate-system" /></Field>
            </>
          ) : (
            <>
              <Field label="Endpoint"><input required value={form.endpoint} onChange={(e) => setForm({ ...form, endpoint: e.target.value })} placeholder="127.0.0.1:50051" /></Field>
              <Field label="Bearer token"><input type="password" value={form.token} onChange={(e) => setForm({ ...form, token: e.target.value })} placeholder="optional" /></Field>
              <div className="field">
                <span>Transport</span>
                <div className="checks">
                  <label><input type="checkbox" checked={form.tls} onChange={(e) => setForm({ ...form, tls: e.target.checked })} /> TLS</label>
                  <label><input type="checkbox" checked={form.insecureSkipVerify} onChange={(e) => setForm({ ...form, insecureSkipVerify: e.target.checked })} /> Skip verify</label>
                </div>
              </div>
            </>
          )}
          <button className="btn primary" disabled={busy}>{busy ? "Connecting…" : "Add cluster"}</button>
        </form>
      </div>
    </>
  );
}
