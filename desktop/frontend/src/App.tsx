import { useCallback, useEffect, useMemo, useState } from "react";
import { fetchRuntimeStatus, fetchServiceStatus, RuntimeStatus, ServiceStatus } from "./api";

const navItems = ["Chat", "Models", "Runtime", "Settings", "Doctor", "Logs"] as const;
type NavItem = (typeof navItems)[number];

export default function App() {
  const [active, setActive] = useState<NavItem>("Chat");
  const [service, setService] = useState<ServiceStatus>({
    running: false,
    base_url: "http://127.0.0.1:11435",
  });
  const [runtime, setRuntime] = useState<RuntimeStatus | null>(null);
  const [refreshing, setRefreshing] = useState(false);

  const refresh = useCallback(async () => {
    const controller = new AbortController();
    setRefreshing(true);
    try {
      const [nextService, nextRuntime] = await Promise.all([
        fetchServiceStatus(controller.signal),
        fetchRuntimeStatus(controller.signal),
      ]);
      setService(nextService);
      setRuntime(nextRuntime);
    } finally {
      setRefreshing(false);
    }
  }, []);

  useEffect(() => {
    void refresh();
    const timer = window.setInterval(() => void refresh(), 5000);
    return () => window.clearInterval(timer);
  }, [refresh]);

  const runtimeLabel = useMemo(() => {
    if (!service.running) {
      return "offline";
    }
    if (!runtime) {
      return "unknown";
    }
    return runtime.backend || "auto";
  }, [runtime, service.running]);

  return (
    <main className="app-shell">
      <aside className="sidebar" aria-label="Primary">
        <div className="brand">
          <span className="brand-mark">VL</span>
          <div>
            <strong>VinoLlama</strong>
            <span>local API {service.running ? "online" : "offline"}</span>
          </div>
        </div>
        <nav className="nav-list">
          {navItems.map((item) => (
            <button
              key={item}
              className={item === active ? "nav-item active" : "nav-item"}
              type="button"
              onClick={() => setActive(item)}
            >
              {item}
            </button>
          ))}
        </nav>
      </aside>

      <section className="workspace">
        <header className="topbar">
          <div>
            <span className={service.running ? "status-dot online" : "status-dot"} />
            <span>{service.base_url}</span>
          </div>
          <div className="topbar-actions">
            <span className="runtime-pill">backend {runtimeLabel}</span>
            <button type="button" className="icon-button" onClick={() => void refresh()} title="Refresh service status">
              {refreshing ? "..." : "↻"}
            </button>
          </div>
        </header>

        {active === "Chat" && <ChatPanel service={service} />}
        {active === "Models" && <PlaceholderPanel title="Models" service={service} />}
        {active === "Runtime" && <RuntimePanel service={service} runtime={runtime} />}
        {active === "Settings" && <PlaceholderPanel title="Settings" service={service} />}
        {active === "Doctor" && <PlaceholderPanel title="Doctor" service={service} />}
        {active === "Logs" && <PlaceholderPanel title="Logs" service={service} />}
      </section>
    </main>
  );
}

function ChatPanel({ service }: { service: ServiceStatus }) {
  return (
    <section className="panel">
      <div className="chat-toolbar">
        <select aria-label="Model" disabled={!service.running}>
          <option>{service.running ? "Select model" : "Service offline"}</option>
        </select>
        <span className="service-chip">{service.running ? `${service.name} ${service.version}` : "not connected"}</span>
      </div>
      <div className="message-surface">
        <div className="empty-state">
          <strong>{service.running ? "Ready" : "Local service not running"}</strong>
          <span>{service.running ? "Choose a model to begin." : service.error ?? "Start vinollama serve."}</span>
        </div>
      </div>
      <form className="composer">
        <textarea aria-label="Message" disabled={!service.running} placeholder="Message VinoLlama" rows={3} />
        <button type="button" disabled={!service.running}>
          Send
        </button>
      </form>
    </section>
  );
}

function RuntimePanel({ service, runtime }: { service: ServiceStatus; runtime: RuntimeStatus | null }) {
  return (
    <section className="panel">
      <div className="section-header">
        <h1>Runtime</h1>
        <span className="service-chip">{service.running ? "connected" : "offline"}</span>
      </div>
      <div className="process-table" role="table" aria-label="Runtime processes">
        <div className="table-row header" role="row">
          <span>Model</span>
          <span>Backend</span>
          <span>PID</span>
          <span>Port</span>
          <span>State</span>
        </div>
        {(runtime?.processes ?? []).length === 0 ? (
          <div className="table-empty">No running models.</div>
        ) : (
          runtime?.processes.map((process) => (
            <div className="table-row" role="row" key={`${process.model}-${process.backend}-${process.pid}`}>
              <span>{process.model}</span>
              <span>{process.backend}</span>
              <span>{process.pid}</span>
              <span>{process.port}</span>
              <span>{process.state}</span>
            </div>
          ))
        )}
      </div>
    </section>
  );
}

function PlaceholderPanel({ title, service }: { title: NavItem; service: ServiceStatus }) {
  return (
    <section className="panel">
      <div className="section-header">
        <h1>{title}</h1>
        <span className="service-chip">{service.running ? "connected" : "offline"}</span>
      </div>
      <div className="empty-state compact">
        <strong>{title}</strong>
        <span>{service.running ? "API connection is available." : "Waiting for local API."}</span>
      </div>
    </section>
  );
}
