import { useEffect, useRef, useState, type FormEvent } from "react";
import { useSearchParams } from "react-router-dom";
import { api, type CommandResult, type ExecuteResult, type RouteSession } from "./api";
import { Field } from "./widgets";

export function ActorActionsPanel({
  clusterId,
  atespace,
  name,
}: {
  clusterId: string;
  atespace: string;
  name: string;
}) {
  const [searchParams] = useSearchParams();
  const action = searchParams.get("action");
  const routeRef = useRef<HTMLDivElement>(null);
  const executeRef = useRef<HTMLDivElement>(null);
  const commandRef = useRef<HTMLDivElement>(null);
  const [route, setRoute] = useState<RouteSession | null>(null);
  const [remotePort, setRemotePort] = useState("80");
  const [method, setMethod] = useState("POST");
  const [path, setPath] = useState("/");
  const [body, setBody] = useState("");
  const [result, setResult] = useState<ExecuteResult | null>(null);
  const [commandMode, setCommandMode] = useState<"shell" | "argv">("shell");
  const [shell, setShell] = useState("uname -a && pwd && ls");
  const [shellBin, setShellBin] = useState("sh");
  const [argv, setArgv] = useState("uname,-a");
  const [commandPath, setCommandPath] = useState("/process");
  const [cwd, setCwd] = useState("");
  const [commandResult, setCommandResult] = useState<CommandResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState<"route" | "execute" | "command" | "stop" | null>(null);

  useEffect(() => {
    api.actorRoute(clusterId, atespace, name).then(setRoute).catch(() => setRoute(null));
  }, [clusterId, atespace, name]);

  useEffect(() => {
    const target =
      action === "execute"
        ? executeRef.current
        : action === "command"
          ? commandRef.current
          : action === "route"
            ? routeRef.current
            : null;
    target?.scrollIntoView({ behavior: "smooth", block: "start" });
  }, [action]);

  async function openRoute() {
    setBusy("route");
    setError(null);
    try {
      const session = await api.routeActor(clusterId, atespace, name, {
        remotePort: Number(remotePort) || 80,
      });
      setRoute(session);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not open route");
    } finally {
      setBusy(null);
    }
  }

  async function stopRoute() {
    setBusy("stop");
    setError(null);
    try {
      await api.stopActorRoute(clusterId, atespace, name);
      setRoute(null);
    } catch (err) {
      setError(err instanceof Error ? err.message : "Could not stop route");
    } finally {
      setBusy(null);
    }
  }

  async function execute(event: FormEvent) {
    event.preventDefault();
    setBusy("execute");
    setError(null);
    setResult(null);
    try {
      const out = await api.executeActor(clusterId, atespace, name, {
        method,
        path,
        body: body || undefined,
        remotePort: Number(remotePort) || 80,
      });
      setResult(out);
      if (out.routeId) {
        const session = await api.actorRoute(clusterId, atespace, name).catch(() => null);
        if (session) setRoute(session);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Execute failed");
    } finally {
      setBusy(null);
    }
  }

  async function runCommand(event: FormEvent) {
    event.preventDefault();
    setBusy("command");
    setError(null);
    setCommandResult(null);
    try {
      const payload =
        commandMode === "shell"
          ? {
              shell,
              shellBin: shellBin || "sh",
              path: commandPath || "/process",
              cwd: cwd || undefined,
              remotePort: Number(remotePort) || 80,
            }
          : {
              command: argv
                .split(",")
                .map((part) => part.trim())
                .filter(Boolean),
              path: commandPath || "/process",
              cwd: cwd || undefined,
              remotePort: Number(remotePort) || 80,
            };
      const out = await api.commandActor(clusterId, atespace, name, payload);
      setCommandResult(out);
      if (out.routeId) {
        const session = await api.actorRoute(clusterId, atespace, name).catch(() => null);
        if (session) setRoute(session);
      }
    } catch (err) {
      setError(err instanceof Error ? err.message : "Command failed");
    } finally {
      setBusy(null);
    }
  }

  return (
    <>
      <section className="detail-grid" style={{ marginTop: 14 }}>
        <div className="detail" ref={routeRef}>
          <h3>Routing</h3>
          <p className="hint">Opens a local ingress path through Substrate’s router. Suspended actors resume on the first request.</p>
          {error ? <div className="form-error">{error}</div> : null}
          <Field label="Actor port">
            <input value={remotePort} onChange={(e) => setRemotePort(e.target.value)} type="number" min={1} />
          </Field>
          {route ? (
            <dl className="kv">
              <dt>Status</dt><dd className="ok">{route.status}</dd>
              <dt>Local URL</dt><dd className="mono">{route.localUrl}</dd>
              <dt>Mapping</dt><dd className="mono">{route.localPort}:{route.remotePort}</dd>
              <dt>Detail</dt><dd>{route.detail || "—"}</dd>
            </dl>
          ) : (
            <p className="empty">No active route.</p>
          )}
          <div className="row-actions">
            <button className="btn primary" disabled={busy !== null} onClick={openRoute}>
              {busy === "route" ? "Opening…" : route ? "Refresh route" : "Route"}
            </button>
            {route ? (
              <button className="text-btn" disabled={busy !== null} onClick={stopRoute}>
                {busy === "stop" ? "Stopping…" : "Stop route"}
              </button>
            ) : null}
            {route ? (
              <button
                className="text-btn"
                type="button"
                onClick={() => navigator.clipboard.writeText(route.localUrl)}
              >
                Copy URL
              </button>
            ) : null}
          </div>
        </div>
        <div className="detail" ref={executeRef}>
          <h3>HTTP Execute</h3>
          <p className="hint">Sends an HTTP request to the actor. Opens a route automatically when needed.</p>
          <form onSubmit={execute}>
            <Field label="Method">
              <select value={method} onChange={(e) => setMethod(e.target.value)}>
                <option>GET</option>
                <option>POST</option>
                <option>PUT</option>
                <option>PATCH</option>
                <option>DELETE</option>
              </select>
            </Field>
            <Field label="Path">
              <input value={path} onChange={(e) => setPath(e.target.value)} placeholder="/" />
            </Field>
            <Field label="Body">
              <textarea value={body} onChange={(e) => setBody(e.target.value)} placeholder='{"optional":true}' />
            </Field>
            <button className="btn primary" disabled={busy !== null}>
              {busy === "execute" ? "Executing…" : "Execute"}
            </button>
          </form>
          {result ? (
            <div style={{ marginTop: 16 }}>
              <dl className="kv">
                <dt>Status</dt><dd>{result.status}</dd>
                <dt>Latency</dt><dd>{result.latencyMs} ms</dd>
                <dt>URL</dt><dd className="mono">{result.localUrl || "—"}</dd>
              </dl>
              <pre className="mono" style={{ whiteSpace: "pre-wrap", margin: 0, maxHeight: 240, overflow: "auto" }}>
                {result.body || "(empty body)"}
              </pre>
            </div>
          ) : null}
        </div>
      </section>
      <section className="detail-grid" style={{ marginTop: 14 }}>
        <div className="detail" ref={commandRef} style={{ gridColumn: "1 / -1" }}>
          <h3>Command Execute</h3>
          <p className="hint">
            Runs any program or shell line inside the actor via <span className="mono">/process</span> (sandbox protocol).
            On the local demo cluster, commands run on the API host.
          </p>
          <form onSubmit={runCommand}>
            <Field label="Mode">
              <select value={commandMode} onChange={(e) => setCommandMode(e.target.value as "shell" | "argv")}>
                <option value="shell">Shell (`sh -c`)</option>
                <option value="argv">Argv</option>
              </select>
            </Field>
            {commandMode === "shell" ? (
              <>
                <Field label="Shell binary">
                  <input value={shellBin} onChange={(e) => setShellBin(e.target.value)} placeholder="sh" />
                </Field>
                <Field label="Shell command">
                  <textarea
                    value={shell}
                    onChange={(e) => setShell(e.target.value)}
                    placeholder="ls -la && echo done"
                    rows={4}
                  />
                </Field>
              </>
            ) : (
              <Field label="Argv (comma-separated)">
                <input value={argv} onChange={(e) => setArgv(e.target.value)} placeholder="ls,-la,/tmp" />
              </Field>
            )}
            <div className="detail-grid" style={{ margin: 0 }}>
              <Field label="Process path">
                <input value={commandPath} onChange={(e) => setCommandPath(e.target.value)} placeholder="/process" />
              </Field>
              <Field label="Working directory">
                <input value={cwd} onChange={(e) => setCwd(e.target.value)} placeholder="optional" />
              </Field>
            </div>
            <button className="btn primary" disabled={busy !== null || (commandMode === "shell" ? !shell.trim() : !argv.trim())}>
              {busy === "command" ? "Running…" : "Run command"}
            </button>
          </form>
          {commandResult ? (
            <div style={{ marginTop: 16 }}>
              <dl className="kv">
                <dt>Exit</dt><dd className={commandResult.exitCode === 0 ? "ok" : undefined}>{commandResult.exitCode}</dd>
                <dt>Latency</dt><dd>{commandResult.latencyMs} ms</dd>
                <dt>URL</dt><dd className="mono">{commandResult.localUrl || "—"}</dd>
                {commandResult.error ? (
                  <>
                    <dt>Error</dt><dd>{commandResult.error}</dd>
                  </>
                ) : null}
              </dl>
              {commandResult.stdout ? (
                <>
                  <h4 className="sub" style={{ margin: "12px 0 6px" }}>stdout</h4>
                  <pre className="mono" style={{ whiteSpace: "pre-wrap", margin: 0, maxHeight: 240, overflow: "auto" }}>
                    {commandResult.stdout}
                  </pre>
                </>
              ) : null}
              {commandResult.stderr ? (
                <>
                  <h4 className="sub" style={{ margin: "12px 0 6px" }}>stderr</h4>
                  <pre className="mono" style={{ whiteSpace: "pre-wrap", margin: 0, maxHeight: 160, overflow: "auto" }}>
                    {commandResult.stderr}
                  </pre>
                </>
              ) : null}
            </div>
          ) : null}
        </div>
      </section>
    </>
  );
}
