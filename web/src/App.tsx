import { FormEvent, useCallback, useEffect, useMemo, useState } from "react";
import {
  Activity, Bot, BrainCircuit, ChevronRight, CircleStop, Cpu, ExternalLink,
  Image, Languages, LockKeyhole, LogOut, MemoryStick, Play, RefreshCw,
  Server, Sparkles, SquareTerminal, Thermometer, X, Zap
} from "lucide-react";

type ServiceState = "running" | "stopped" | "starting" | "error";
type Service = {
  id: string; name: string; description: string; state: ServiceState;
  managed: boolean; healthy: boolean; port?: number; open_url?: string;
  dependencies?: string[]; last_error?: string; can_start: boolean;
  can_stop: boolean; can_restart: boolean;
};
type Group = { id: string; name: string; description: string; services: string[] };
type Metrics = {
  cpu_percent: number; memory_used_gb: number; memory_total_gb: number;
  memory_percent: number; uptime_seconds: number; collected_at: string;
  gpu: { available: boolean; name?: string; utilization: number; memory_used_gb: number; memory_total_gb: number; temperature: number; power_watts: number };
};
type ActivityEvent = { id: number; timestamp: string; service_id?: string; action: string; level: string; message: string };

const iconByService = { ollama: BrainCircuit, comfyui: Image, mimic: Languages, craig: Bot } as const;
const iconByGroup = { "local-ai": BrainCircuit, creative: Sparkles, language: Languages } as const;

const demoServices: Service[] = [
  { id: "ollama", name: "Ollama", description: "Local models and inference API", state: "stopped", managed: false, healthy: false, port: 11434, can_start: true, can_stop: false, can_restart: false },
  { id: "comfyui", name: "ComfyUI", description: "Image, speech and audio workflows", state: "stopped", managed: false, healthy: false, port: 8188, can_start: true, can_stop: false, can_restart: false },
  { id: "mimic", name: "Mimic backend", description: "Speech recognition and language tutoring", state: "stopped", managed: false, healthy: false, port: 8000, dependencies: ["ollama"], can_start: true, can_stop: false, can_restart: false },
  { id: "craig", name: "Craig", description: "ComfyUI Telegram agent", state: "stopped", managed: false, healthy: false, dependencies: ["comfyui"], can_start: true, can_stop: false, can_restart: false }
];
const demoGroups: Group[] = [
  { id: "local-ai", name: "Local AI", description: "Ollama", services: ["ollama"] },
  { id: "creative", name: "Creative stack", description: "ComfyUI + Craig", services: ["comfyui", "craig"] },
  { id: "language", name: "Language stack", description: "Ollama + Mimic", services: ["ollama", "mimic"] }
];
const demoMetrics: Metrics = {
  cpu_percent: 8, memory_used_gb: 9.4, memory_total_gb: 32, memory_percent: 29,
  uptime_seconds: 0, collected_at: new Date().toISOString(),
  gpu: { available: true, name: "NVIDIA GeForce RTX 3080 Ti", utilization: 2, memory_used_gb: 0.8, memory_total_gb: 12, temperature: 38, power_watts: 28 }
};

async function request<T>(path: string, options?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    credentials: "same-origin",
    ...options,
    headers: {
      "Content-Type": "application/json",
      ...(options?.method && options.method !== "GET" ? { "X-Workbench-Request": "1" } : {}),
      ...options?.headers
    }
  });
  const body = await response.json().catch(() => ({}));
  if (!response.ok) throw new Error(body.error || `Request failed (${response.status})`);
  return body as T;
}

function Metric({ label, value, detail, fill, icon: Icon }: { label: string; value: string; detail: string; fill: number; icon: typeof Cpu }) {
  return <div className="metric">
    <div className="metric-heading"><span><Icon size={15} /> {label}</span><strong>{value}</strong></div>
    <div className="meter" aria-hidden="true"><i style={{ width: `${Math.max(1, Math.min(100, fill))}%` }} /></div>
    <small>{detail}</small>
  </div>;
}

function State({ service }: { service: Service }) {
  const label = service.state === "running" && !service.managed ? "external" : service.state;
  return <span className={`state state-${service.state}`} title={!service.managed && service.state === "running" ? "Detected, but not started by Workbench" : undefined}><i />{label}</span>;
}

function Login({ onLogin }: { onLogin: () => void }) {
  const [password, setPassword] = useState("");
  const [error, setError] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event: FormEvent) {
    event.preventDefault(); setBusy(true); setError("");
    try { await request("/api/login", { method: "POST", body: JSON.stringify({ password }) }); onLogin(); }
    catch (reason) { setError(reason instanceof Error ? reason.message : "Sign-in failed"); }
    finally { setBusy(false); }
  }
  return <main className="login-wrap">
    <form className="login-card" onSubmit={submit}>
      <span className="login-mark"><SquareTerminal size={22} /></span>
      <p className="eyebrow">Private control plane</p>
      <h1>Workbench</h1>
      <p>Sign in to control the AI services on your gaming PC.</p>
      <label htmlFor="password">Password</label>
      <div className="password-field"><LockKeyhole size={16} /><input id="password" type="password" value={password} onChange={event => setPassword(event.target.value)} autoComplete="current-password" autoFocus /></div>
      {error ? <p className="form-error" role="alert">{error}</p> : null}
      <button className="primary-button" disabled={busy || !password}>{busy ? "Signing in…" : "Sign in"}</button>
      <small>Only reachable through your private network.</small>
    </form>
  </main>;
}

function ServiceRow({ service, busy, onAction, onLogs }: { service: Service; busy: boolean; onAction: (id: string, action: string) => void; onLogs: (service: Service) => void }) {
  const Icon = iconByService[service.id as keyof typeof iconByService] ?? Server;
  return <article className="service-row">
    <button className="service-icon service-icon-button" onClick={() => onLogs(service)} aria-label={`View ${service.name} logs`}><Icon size={19} /></button>
    <div className="service-copy">
      <div className="service-name"><button onClick={() => onLogs(service)}>{service.name}</button><State service={service} /></div>
      <p>{service.description}{service.port ? <span> · :{service.port}</span> : null}{service.dependencies?.length ? <span> · needs {service.dependencies.join(", ")}</span> : null}</p>
      {service.last_error ? <p className="service-error">{service.last_error}</p> : null}
    </div>
    <div className="service-actions">
      {service.open_url && service.state === "running" ? <a className="icon-button muted" href={service.open_url} target="_blank" rel="noreferrer" title={`Open ${service.name}`} aria-label={`Open ${service.name}`}><ExternalLink size={16} /></a> : null}
      <button className="icon-button" disabled={busy || !service.can_start} onClick={() => onAction(service.id, "start")} title={`Start ${service.name}`} aria-label={`Start ${service.name}`}><Play size={16} fill="currentColor" /></button>
      <button className="icon-button muted" disabled={busy || !service.can_restart} onClick={() => onAction(service.id, "restart")} title={`Restart ${service.name}`} aria-label={`Restart ${service.name}`}><RefreshCw size={16} /></button>
      <button className="icon-button muted danger-hover" disabled={busy || !service.can_stop} onClick={() => onAction(service.id, "stop")} title={`Stop ${service.name}`} aria-label={`Stop ${service.name}`}><CircleStop size={16} /></button>
    </div>
  </article>;
}

function LogsDialog({ service, logs, onClose }: { service: Service; logs: string; onClose: () => void }) {
  return <div className="dialog-backdrop" role="presentation" onMouseDown={event => { if (event.currentTarget === event.target) onClose(); }}>
    <section className="logs-dialog" role="dialog" aria-modal="true" aria-labelledby="logs-title">
      <div className="logs-head"><div><p className="eyebrow">Process output</p><h2 id="logs-title">{service.name}</h2></div><button className="icon-button muted" onClick={onClose} aria-label="Close logs"><X size={17} /></button></div>
      <pre>{logs || "No output captured yet.\n\nLogs appear here after Workbench starts this service."}</pre>
    </section>
  </div>;
}

export function App() {
  const [authenticated, setAuthenticated] = useState<boolean | null>(null);
  const [demo, setDemo] = useState(false);
  const [services, setServices] = useState<Service[]>(demoServices);
  const [groups, setGroups] = useState<Group[]>(demoGroups);
  const [metrics, setMetrics] = useState<Metrics>(demoMetrics);
  const [events, setEvents] = useState<ActivityEvent[]>([]);
  const [busy, setBusy] = useState<Record<string, boolean>>({});
  const [notice, setNotice] = useState("");
  const [logs, setLogs] = useState<{ service: Service; text: string } | null>(null);

  const refresh = useCallback(async () => {
    if (demo) return;
    try {
      const [serviceData, systemData, activityData] = await Promise.all([
        request<{ services: Service[]; groups: Group[] }>("/api/services"),
        request<Metrics>("/api/system"),
        request<{ events: ActivityEvent[] }>("/api/activity?limit=8")
      ]);
      setServices(serviceData.services); setGroups(serviceData.groups); setMetrics(systemData); setEvents(activityData.events);
    } catch (reason) {
      if (reason instanceof Error && reason.message === "authentication required") setAuthenticated(false);
    }
  }, [demo]);

  useEffect(() => {
    request<{ authenticated: boolean }>("/api/session").then(result => setAuthenticated(result.authenticated)).catch(() => {
      if (import.meta.env.DEV) { setDemo(true); setAuthenticated(true); }
      else setAuthenticated(false);
    });
  }, []);
  useEffect(() => { if (authenticated) refresh(); }, [authenticated, refresh]);
  useEffect(() => {
    if (!authenticated || demo) return;
    const stream = new EventSource("/api/events");
    stream.addEventListener("refresh", refresh);
    return () => stream.close();
  }, [authenticated, demo, refresh]);
  useEffect(() => { if (!notice) return; const timer = window.setTimeout(() => setNotice(""), 3500); return () => window.clearTimeout(timer); }, [notice]);

  const running = useMemo(() => services.filter(service => service.state === "running").length, [services]);
  async function action(id: string, operation: string) {
    if (demo) { setNotice("The live controller will perform this action after installation."); return; }
    setBusy(value => ({ ...value, [id]: true }));
    try { await request(`/api/services/${id}/${operation}`, { method: "POST" }); setNotice(`${operation[0].toUpperCase() + operation.slice(1)} request accepted.`); await refresh(); }
    catch (reason) { setNotice(reason instanceof Error ? reason.message : "Action failed"); }
    finally { setBusy(value => ({ ...value, [id]: false })); }
  }
  async function startGroup(id: string) {
    if (demo) { setNotice("The complete stack will start here after installation."); return; }
    setBusy(value => ({ ...value, [`group-${id}`]: true }));
    try { await request(`/api/groups/${id}/start`, { method: "POST" }); setNotice("Stack is starting."); await refresh(); }
    catch (reason) { setNotice(reason instanceof Error ? reason.message : "Could not start stack"); }
    finally { setBusy(value => ({ ...value, [`group-${id}`]: false })); }
  }
  async function stopAll() {
    if (demo) { setNotice("All Workbench-managed services will stop here."); return; }
    setBusy(value => ({ ...value, all: true }));
    try { await request("/api/stop-all", { method: "POST" }); setNotice("All managed services stopped."); await refresh(); }
    catch (reason) { setNotice(reason instanceof Error ? reason.message : "Could not stop services"); }
    finally { setBusy(value => ({ ...value, all: false })); }
  }
  async function showLogs(service: Service) {
    if (demo) { setLogs({ service, text: "Preview mode — live process output will appear here after installation." }); return; }
    try { const response = await request<{ logs: string }>(`/api/services/${service.id}/logs`); setLogs({ service, text: response.logs }); }
    catch (reason) { setNotice(reason instanceof Error ? reason.message : "Could not load logs"); }
  }
  async function logout() { if (!demo) await request("/api/logout", { method: "POST" }); setAuthenticated(false); }

  if (authenticated === null) return <div className="loading-screen"><SquareTerminal size={22} /> Connecting to Workbench…</div>;
  if (!authenticated) return <Login onLogin={() => setAuthenticated(true)} />;

  const gpu = metrics.gpu;
  return <div className="shell">
    <header className="topbar">
      <a className="brand" href="/" aria-label="Workbench home"><span className="brand-mark"><SquareTerminal size={18} /></span><span>Workbench</span></a>
      <div className="topbar-right"><div className="host-status"><i /> Gaming PC <span>online</span></div><button className="logout" onClick={logout} title="Sign out"><LogOut size={15} /><span>Sign out</span></button></div>
    </header>
    <main>
      <section className="intro"><div><p className="eyebrow">Local control plane</p><h1>Your AI tools, ready when you need them.</h1><p className="intro-copy">Start only the services you need, keep an eye on the GPU, and shut everything down cleanly when you are done.</p></div><div className="intro-actions"><div className="intro-badge"><Zap size={17} /> {gpu.name?.replace("NVIDIA GeForce ", "") || "GPU"}</div>{running > 0 ? <button className="stop-all" disabled={busy.all} onClick={stopAll}><CircleStop size={15} /> Stop all</button> : null}</div></section>
      <section className="system-panel" aria-label="System resources"><div className="panel-title"><span><Activity size={17} /> System</span><small>{demo ? "preview data" : "live from this PC"}</small></div><div className="metric-grid">
        <Metric label="GPU" value={gpu.available ? `${gpu.utilization}%` : "—"} detail={gpu.available ? `${gpu.power_watts} W` : "not available"} fill={gpu.utilization} icon={Cpu} />
        <Metric label="VRAM" value={gpu.available ? `${gpu.memory_used_gb} GB` : "—"} detail={gpu.available ? `of ${gpu.memory_total_gb} GB` : "not available"} fill={gpu.memory_total_gb ? gpu.memory_used_gb / gpu.memory_total_gb * 100 : 0} icon={MemoryStick} />
        <Metric label="Memory" value={`${metrics.memory_used_gb} GB`} detail={`of ${metrics.memory_total_gb} GB · CPU ${metrics.cpu_percent}%`} fill={metrics.memory_percent} icon={Server} />
        <Metric label="Temperature" value={gpu.available ? `${gpu.temperature}°C` : "—"} detail="GPU core" fill={gpu.temperature} icon={Thermometer} />
      </div></section>
      <div className="content-grid">
        <section className="panel services-panel"><div className="panel-title"><span><Server size={17} /> Services</span><small>{running} of {services.length} running</small></div><div className="service-list">{services.map(service => <ServiceRow key={service.id} service={service} busy={!!busy[service.id]} onAction={action} onLogs={showLogs} />)}</div></section>
        <aside className="side-column">
          <section className="panel quick-panel"><div className="panel-title"><span><Zap size={17} /> Quick start</span></div><div className="quick-list">{groups.map(group => { const Icon = iconByGroup[group.id as keyof typeof iconByGroup] ?? Zap; return <button className="quick-row" key={group.id} disabled={busy[`group-${group.id}`]} onClick={() => startGroup(group.id)}><span className="quick-icon"><Icon size={17} /></span><span><strong>{group.name}</strong><small>{group.description}</small></span><ChevronRight size={17} /></button>; })}</div></section>
          <section className="panel activity-panel"><div className="panel-title"><span><Activity size={17} /> Recent activity</span><small>{events.length ? "latest changes" : "quiet"}</small></div>{events.length ? <div className="activity-list">{events.map(event => <div className={`activity-row level-${event.level}`} key={event.id}><i /><div><p>{event.message}</p><small>{new Date(event.timestamp).toLocaleString([], { dateStyle: "short", timeStyle: "short" })}</small></div></div>)}</div> : <div className="empty-state"><span><Activity size={18} /></span><p>No activity yet</p><small>Service changes will appear here.</small></div>}</section>
        </aside>
      </div>
    </main>
    <footer>Private · available through Tailscale</footer>
    {notice ? <div className="toast" role="status">{notice}</div> : null}
    {logs ? <LogsDialog service={logs.service} logs={logs.text} onClose={() => setLogs(null)} /> : null}
  </div>;
}
