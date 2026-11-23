import { BrowserRouter, Route, Routes } from "react-router-dom";
import { ClustersPage } from "./clusters";
import { Dashboard } from "./dashboard";
import { ActorDetail, ActorsPage, AtespaceDetail, AtespacesPage, TemplateDetail, TemplatesPage, WorkerDetail, WorkersPage } from "./resources";
import { AppShell } from "./shell";

export function App() {
  return (
    <BrowserRouter>
      <Routes>
        <Route element={<AppShell />}>
          <Route index element={<Dashboard />} />
          <Route path="atespaces" element={<AtespacesPage />} />
          <Route path="atespaces/:name" element={<AtespaceDetail />} />
          <Route path="templates" element={<TemplatesPage />} />
          <Route path="templates/:atespace/:name" element={<TemplateDetail />} />
          <Route path="actors" element={<ActorsPage />} />
          <Route path="actors/:atespace/:name" element={<ActorDetail />} />
          <Route path="workers" element={<WorkersPage />} />
          <Route path="workers/:name" element={<WorkerDetail />} />
          <Route path="clusters" element={<ClustersPage />} />
        </Route>
      </Routes>
    </BrowserRouter>
  );
}
