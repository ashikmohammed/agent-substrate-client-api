import { createContext, useContext, useEffect, useMemo, useState } from "react";
import { NavLink, Outlet, useNavigate } from "react-router-dom";
import { api, type PublicCluster } from "./api";

const STORAGE_KEY = "agentsubstrate.cluster";

type ClusterContextValue = {
  clusterId: string;
  setClusterId: (id: string) => void;
  clusters: PublicCluster[];
  refreshClusters: () => Promise<PublicCluster[]>;
};

const ClusterContext = createContext<ClusterContextValue | null>(null);

export function useCluster() {
  const value = useContext(ClusterContext);
  if (!value) throw new Error("cluster context missing");
  return value;
}

export function AppShell() {
  const [clusters, setClusters] = useState<PublicCluster[]>([]);
  const [clusterId, setClusterIdState] = useState(() => localStorage.getItem(STORAGE_KEY) || "local");
  const navigate = useNavigate();

  async function refreshClusters() {
    const res = await api.clusters();
    setClusters(res.items);
    const preferred = res.default || res.items.find((item) => item.driver === "kube")?.id || res.items[0]?.id;
    if (preferred) {
      if (!res.items.some((item) => item.id === clusterId)) {
        setClusterId(preferred);
      } else if (!localStorage.getItem(STORAGE_KEY) && preferred !== clusterId) {
        setClusterId(preferred);
      }
    }
    return res.items;
  }

  useEffect(() => {
    refreshClusters().catch(() => setClusters([]));
  }, [clusterId]);

  function setClusterId(id: string) {
    localStorage.setItem(STORAGE_KEY, id);
    setClusterIdState(id);
  }

  const current = clusters.find((item) => item.id === clusterId);
  const value = useMemo(
    () => ({ clusterId, setClusterId, clusters, refreshClusters }),
    [clusterId, clusters],
  );

  return (
    <ClusterContext.Provider value={value}>
      <div className="app">
        <aside className="rail">
          <a className="brand" href="/" onClick={(e) => { e.preventDefault(); navigate("/"); }}>
            <div className="mark">A</div>
            <div>
              <strong>Substrate</strong>
              <span>Control console</span>
            </div>
          </a>
          <nav className="nav">
            <NavLink to="/" end>Dashboard <small>01</small></NavLink>
            <NavLink to="/atespaces">Atespaces <small>02</small></NavLink>
            <NavLink to="/templates">Templates <small>03</small></NavLink>
            <NavLink to="/actors">Actors <small>04</small></NavLink>
            <NavLink to="/workers">Workers <small>05</small></NavLink>
            <NavLink to="/clusters">Clusters <small>06</small></NavLink>
          </nav>
          <p className="rail-foot">Point this console at any substrate control plane from Clusters.</p>
        </aside>
        <main className="stage">
          <div className="topbar">
            <div className="cluster-pick">
              <span>Cluster</span>
              <select value={clusterId} onChange={(e) => setClusterId(e.target.value)} aria-label="Cluster">
                {clusters.map((item) => (
                  <option key={item.id} value={item.id}>{item.name}</option>
                ))}
              </select>
              {current ? <span className={`pill ${current.driver === "grpc" ? "run" : "idle"}`}>{current.driver}</span> : null}
            </div>
          </div>
          <Outlet key={clusterId} />
        </main>
      </div>
    </ClusterContext.Provider>
  );
}
