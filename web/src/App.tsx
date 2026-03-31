import { useState, useEffect } from "react";
import Layout from "./components/Layout";
import Dashboard from "./components/Dashboard";
import RunDetail from "./components/RunDetail";
import CompareView from "./components/CompareView";
import TrendsPage from "./components/TrendsPage";
import LiveView from "./components/LiveView";
import Settings from "./components/Settings";
import NewRun from "./components/NewRun";
import RunStatus from "./components/RunStatus";
import RunQueue from "./components/RunQueue";

type Route =
  | { page: "home" }
  | { page: "run"; id: string }
  | { page: "compare" }
  | { page: "trends" }
  | { page: "live" }
  | { page: "settings" }
  | { page: "newRun" }
  | { page: "runStatus"; id: string }
  | { page: "runQueue" };

function parseHash(): Route {
  const hash = window.location.hash.slice(1);
  if (hash === "/compare") return { page: "compare" };
  if (hash === "/trends") return { page: "trends" };
  if (hash === "/live" || hash.startsWith("/live?")) return { page: "live" };
  if (hash === "/settings") return { page: "settings" };
  if (hash === "/runs/new") return { page: "newRun" };
  if (hash === "/runs/queue") return { page: "runQueue" };
  const statusMatch = hash.match(/^\/runs\/status\/(.+)$/);
  if (statusMatch?.[1]) return { page: "runStatus", id: statusMatch[1] };
  const runMatch = hash.match(/^\/runs\/(.+)$/);
  if (runMatch?.[1]) return { page: "run", id: runMatch[1] };
  return { page: "home" };
}

export default function App() {
  const [route, setRoute] = useState<Route>(parseHash);

  useEffect(() => {
    const onHashChange = () => setRoute(parseHash());
    window.addEventListener("hashchange", onHashChange);
    return () => window.removeEventListener("hashchange", onHashChange);
  }, []);

  return (
    <Layout>
      {route.page === "home" && <Dashboard />}
      {route.page === "run" && <RunDetail id={route.id} />}
      {route.page === "compare" && <CompareView />}
      {route.page === "trends" && <TrendsPage />}
      {route.page === "live" && <LiveView />}
      {route.page === "settings" && <Settings />}
      {route.page === "newRun" && <NewRun />}
      {route.page === "runStatus" && <RunStatus id={route.id} />}
      {route.page === "runQueue" && <RunQueue />}
    </Layout>
  );
}
