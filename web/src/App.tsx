import { AnimatePresence, MotionConfig, motion } from "framer-motion";
import {
  ArrowRight,
  Check,
  ChevronDown,
  CircleAlert,
  CircleHelp,
  Cloud,
  ExternalLink,
  LoaderCircle,
  LockKeyhole,
  Plus,
  Power,
  RefreshCw,
  Save,
  Settings2,
  TerminalSquare,
  Trash2,
  X,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";

type ServiceState = "draft" | "deploying" | "stopping" | "active" | "stopped" | "error";
type AllowType = "email" | "email_domain";
type ExposureMode = "quick" | "private" | "public";
type TokenState = "idle" | "checking" | "connected" | "invalid";

type Service = {
  id: string;
  name: string;
  mode?: ExposureMode;
  hostname?: string;
  origin_url: string;
  allow_type?: AllowType;
  allow_value?: string;
  state: ServiceState;
  tunnel_id?: string;
  private_route_id?: string;
  dns_record_id?: string;
  access_application_id?: string;
  access_policy_id?: string;
  public_url?: string;
  created_at: string;
  updated_at: string;
};

type Operation = {
  operation_id: string;
  service_id: string;
  kind: string;
  status: "pending" | "running" | "succeeded" | "failed" | "unknown";
  current_step?: string;
  attempts: number;
  error_code?: string;
  error_message?: string;
  created_at: string;
  updated_at: string;
};

type IntegrationStatus = {
  configured: boolean;
  account_id?: string;
  zone_id?: string;
  token_id?: string;
  token_state?: string;
  last_error?: string;
};

type Zone = { id: string; name: string };
type Connector = { service_id: string; mode?: string; running: boolean; healthy: boolean; url?: string; message?: string };

type ServiceForm = {
  mode: ExposureMode;
  name: string;
  hostname: string;
  origin_url: string;
  allow_type: AllowType | "";
  allow_value: string;
};

const emptyIntegration: IntegrationStatus = { configured: false };
const emptyServiceForm: ServiceForm = {
  mode: "quick",
  name: "",
  hostname: "",
  origin_url: "http://",
  allow_type: "",
  allow_value: "",
};

const modeOptions: Array<{ value: ExposureMode; label: string; short: string }> = [
  { value: "quick", label: "临时公开", short: "Quick" },
  { value: "private", label: "私网受控", short: "Private" },
  { value: "public", label: "自有域名", short: "Public" },
];

function serviceMode(item: Pick<Service, "mode" | "hostname">): ExposureMode {
  if (item.mode === "private" || item.mode === "public" || item.mode === "quick") return item.mode;
  return item.hostname ? "public" : "quick";
}

function modeMeta(mode: ExposureMode) {
  switch (mode) {
    case "private":
      return { label: "私网受控", tone: "private" };
    case "public":
      return { label: "自有域名", tone: "public" };
    default:
      return { label: "临时公开", tone: "quick" };
  }
}

class ApiError extends Error {
  readonly status: number;
  readonly code?: string;

  constructor(status: number, message: string, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

async function request<T>(path: string, token: string, init?: RequestInit): Promise<T> {
  const headers = new Headers(init?.headers);
  if (init?.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }
  if (token.trim()) {
    headers.set("Authorization", `Bearer ${token.trim()}`);
  }
  const response = await fetch(path, { ...init, headers });
  const payload = (await response.json().catch(() => null)) as { message?: string; code?: string } | null;
  if (!response.ok) {
    throw new ApiError(response.status, payload?.message || "请求未完成", payload?.code);
  }
  return payload as T;
}

function getStoredToken() {
  try {
    return sessionStorage.getItem("tunnelbox.adminToken") || "";
  } catch {
    return "";
  }
}

function storeToken(token: string) {
  try {
    if (token.trim()) sessionStorage.setItem("tunnelbox.adminToken", token.trim());
    else sessionStorage.removeItem("tunnelbox.adminToken");
  } catch {
    // Session storage can be unavailable in locked-down browsers.
  }
}

const onboardingStorageKey = "tunnelbox.onboarding.dismissed";

function hasDismissedOnboarding() {
  if (typeof window === "undefined") return false;
  try {
    return window.localStorage.getItem(onboardingStorageKey) === "1";
  } catch {
    return false;
  }
}

function rememberOnboardingDismissed() {
  try {
    window.localStorage.setItem(onboardingStorageKey, "1");
  } catch {
    // Local storage can be unavailable in locked-down browsers.
  }
}

function formatTime(value?: string) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  return date.toLocaleString("zh-CN", { month: "short", day: "numeric", hour: "2-digit", minute: "2-digit" });
}

function stateMeta(state: ServiceState) {
  switch (state) {
    case "active":
      return { label: "运行中", tone: "active" };
    case "deploying":
      return { label: "部署中", tone: "working" };
    case "stopping":
      return { label: "停止中", tone: "working" };
    case "stopped":
      return { label: "已停止", tone: "stopped" };
    case "error":
      return { label: "异常", tone: "error" };
    default:
      return { label: "草稿", tone: "draft" };
  }
}

function serviceHasRemoteResources(item: Service) {
  return Boolean(item.tunnel_id || item.private_route_id || item.dns_record_id || item.access_application_id || item.access_policy_id || item.public_url);
}

function operationMeta(status: Operation["status"]) {
  switch (status) {
    case "succeeded":
      return { label: "已完成", tone: "active" };
    case "failed":
      return { label: "失败", tone: "error" };
    case "unknown":
      return { label: "待确认", tone: "working" };
    case "running":
      return { label: "执行中", tone: "working" };
    default:
      return { label: "排队中", tone: "draft" };
  }
}

function operationLabel(kind: string) {
  if (kind === "deploy") return "部署操作";
  if (kind === "stop") return "停止操作";
  if (kind === "delete") return "删除操作";
  return "最近操作";
}

function Spinner({ size = 16 }: { size?: number }) {
  return (
    <motion.span className="spinner" animate={{ rotate: 360 }} transition={{ repeat: Infinity, duration: 0.9, ease: "linear" }}>
      <LoaderCircle size={size} />
    </motion.span>
  );
}

function App() {
  const [adminToken, setAdminToken] = useState(getStoredToken);
  const [tokenDraft, setTokenDraft] = useState(adminToken);
  const adminTokenRef = useRef(adminToken);
  const loadGenerationRef = useRef(0);
  const [integration, setIntegration] = useState<IntegrationStatus>(emptyIntegration);
  const [zones, setZones] = useState<Zone[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
  const [tokenState, setTokenState] = useState<TokenState>(adminToken ? "checking" : "idle");
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editor, setEditor] = useState<Service | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Service | null>(null);
  const [guideOpen, setGuideOpen] = useState(() => !hasDismissedOnboarding());

  const loadData = useCallback(
    async (showRefresh = false, tokenOverride?: string): Promise<boolean> => {
      const requestToken = tokenOverride ?? adminTokenRef.current;
      const generation = ++loadGenerationRef.current;
      const isCurrentLoad = () => generation === loadGenerationRef.current;
      if (showRefresh) setRefreshing(true);
      else setLoading(true);
      if (isCurrentLoad()) setError("");
      try {
        const [status, servicePayload, connectorPayload] = await Promise.all([
          request<IntegrationStatus>("/api/v1/integrations/cloudflare/status", requestToken),
          request<{ services: Service[] }>("/api/v1/services", requestToken),
          request<{ connectors: Connector[] }>("/api/v1/connectors", requestToken),
        ]);
        if (!isCurrentLoad()) return false;
        setIntegration(status);
        setServices(servicePayload.services || []);
        setConnectors(connectorPayload.connectors || []);
        if (status.configured && status.zone_id) {
          const zonePayload = await request<{ zones: Zone[] }>("/api/v1/zones", requestToken);
          if (!isCurrentLoad()) return false;
          setZones(zonePayload.zones || []);
        } else {
          setZones([]);
        }
        setAuthRequired(false);
        if (requestToken && requestToken === adminTokenRef.current) setTokenState("connected");
        return true;
      } catch (caught) {
        if (!isCurrentLoad()) return false;
        if (caught instanceof ApiError && caught.status === 401) {
          if (requestToken === adminTokenRef.current) {
            setAuthRequired(true);
            setError("需要管理员令牌才能访问控制面");
            if (requestToken) setTokenState("invalid");
          }
        } else {
          setError(caught instanceof Error ? caught.message : "无法加载控制面数据");
        }
        return false;
      } finally {
        if (isCurrentLoad()) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    []
  );

  useEffect(() => {
    void loadData();
  }, [loadData]);

  useEffect(() => {
    if (!operation || ["succeeded", "failed", "unknown"].includes(operation.status)) return;
    let stopped = false;
    const timer = window.setInterval(async () => {
      try {
        const next = await request<Operation>(`/api/v1/operations/${operation.operation_id}`, adminToken);
        if (!stopped) {
          setOperation(next);
          if (["succeeded", "failed", "unknown"].includes(next.status)) {
            await loadData(true);
          }
        }
      } catch (caught) {
        if (!stopped) setError(caught instanceof Error ? caught.message : "无法读取操作进度");
      }
    }, 1000);
    return () => {
      stopped = true;
      window.clearInterval(timer);
    };
  }, [adminToken, loadData, operation]);

  const activeConnectors = useMemo(() => connectors.filter((item) => item.running).length, [connectors]);

  async function deploy(item: Service) {
    setError("");
    setNotice("");
    try {
      const next = await request<Operation>(`/api/v1/services/${item.id}/deploy`, adminToken, { method: "POST" });
      setOperation(next);
      setNotice(`已开始部署 ${item.name}`);
      await loadData(true);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "无法开始部署");
    }
  }

  async function stop(item: Service) {
    setError("");
    setNotice("");
    try {
      const next = await request<Operation>(`/api/v1/services/${item.id}/stop`, adminToken, { method: "POST" });
      setOperation(next);
      setNotice(`已开始停止 ${item.name}`);
      await loadData(true);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "无法停止服务");
    }
  }

  async function remove(item: Service) {
    setDeleteTarget(null);
    setError("");
    setNotice("");
    try {
      const next = await request<Operation | null>(`/api/v1/services/${item.id}`, adminToken, { method: "DELETE" });
      if (next?.operation_id) {
        setOperation(next);
        setNotice(`已开始删除 ${item.name}`);
      } else {
        setServices((current) => current.filter((entry) => entry.id !== item.id));
        setNotice(`已删除 ${item.name}`);
      }
      await loadData(true);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "无法删除服务");
    }
  }

  async function submitToken(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const next = tokenDraft.trim();
    setError("");
    setNotice("");
    if (!next) {
      adminTokenRef.current = "";
      setAdminToken("");
      storeToken("");
      setAuthRequired(true);
      setTokenState("invalid");
      setError("请输入管理员令牌");
      return;
    }

    setTokenState("checking");
    try {
      await request<IntegrationStatus>("/api/v1/integrations/cloudflare/status", next);
    } catch (caught) {
      setTokenState("invalid");
      setError(caught instanceof ApiError && caught.status === 401 ? "管理员令牌无效，请检查后重试" : caught instanceof Error ? caught.message : "令牌验证失败");
      return;
    }

    adminTokenRef.current = next;
    setAdminToken(next);
    storeToken(next);
    setAuthRequired(false);
    setTokenState("connected");
    setNotice("管理员令牌已验证");
    await loadData(true, next);
  }

  function closeGuide() {
    rememberOnboardingDismissed();
    setGuideOpen(false);
  }

  function startQuick() {
    closeGuide();
    setEditor("new");
  }

  function startConfiguration() {
    closeGuide();
    setIntegrationOpen(true);
  }

  return (
    <MotionConfig reducedMotion="user">
      <div className="app-shell">
      <aside className="sidebar">
        <div className="brand-lockup">
          <div className="brand-mark" aria-hidden="true"><TerminalSquare size={18} /></div>
          <div>
            <strong>TunnelBox</strong>
            <span>控制面</span>
          </div>
        </div>
        <div className="workspace-switcher">
          <span className="eyebrow">工作区</span>
          <button type="button" className="workspace-button" title="当前工作区">
            <span>Default</span><ChevronDown size={15} />
          </button>
        </div>
        <nav className="side-nav" aria-label="主导航">
          <a className="nav-item active" href="#services">服务</a>
          <a className="nav-item" href="#integration">Cloudflare</a>
        </nav>
        <div className="sidebar-footnote">
          <LockKeyhole size={15} />
          <span>访问策略由 Cloudflare Access 执行</span>
        </div>
      </aside>

      <main className="main-content">
        <header className="topbar">
          <div>
            <p className="eyebrow">工作区 / Default</p>
            <h1>服务发布</h1>
          </div>
          <div className="topbar-actions">
            <button type="button" className="button button-secondary guide-trigger" onClick={() => setGuideOpen(true)} title="打开使用指南">
              <CircleHelp size={16} />使用指南
            </button>
            <form className="token-form" onSubmit={submitToken}>
              <label className="token-field">
                <LockKeyhole size={14} aria-hidden="true" />
                <span className="sr-only">管理员令牌</span>
                <input
                  type="password"
                  value={tokenDraft}
                  onChange={(event) => { setTokenDraft(event.target.value); setTokenState("idle"); setError(""); }}
                  placeholder="管理员令牌"
                  autoComplete="off"
                  aria-invalid={tokenState === "invalid"}
                  disabled={tokenState === "checking"}
                />
              </label>
              <button type="submit" className="button button-primary token-submit" disabled={tokenState === "checking"}>
                {tokenState === "checking" ? <Spinner size={15} /> : tokenState === "connected" ? <Check size={15} /> : <ArrowRight size={15} />}
                {tokenState === "checking" ? "验证中" : tokenState === "connected" ? "已连接" : "连接"}
              </button>
              {tokenState === "connected" && <span className="token-feedback success" role="status"><Check size={13} />令牌有效</span>}
              {tokenState === "invalid" && <span className="token-feedback error" role="alert"><CircleAlert size={13} />令牌无效</span>}
            </form>
            <button className="icon-button" type="button" onClick={() => void loadData(true)} title="刷新数据" aria-label="刷新数据">
              {refreshing ? <Spinner size={17} /> : <RefreshCw size={17} />}
            </button>
          </div>
        </header>

        <AnimatePresence initial={false}>
          {error && (
            <motion.div className="banner banner-error" role="alert" initial={{ opacity: 0, y: -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -6 }}>
              <CircleAlert size={17} />
              <span>{error}</span>
              {authRequired && <button type="button" className="banner-action" onClick={() => document.querySelector<HTMLInputElement>(".token-field input")?.focus()}>输入令牌</button>}
              <button type="button" className="banner-close" onClick={() => setError("")} aria-label="关闭提示" title="关闭提示"><X size={16} /></button>
            </motion.div>
          )}
          {notice && (
            <motion.div className="banner banner-success" role="status" initial={{ opacity: 0, y: -6 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: -6 }}>
              <Check size={17} /><span>{notice}</span>
              <button type="button" className="banner-close" onClick={() => setNotice("")} aria-label="关闭提示" title="关闭提示"><X size={16} /></button>
            </motion.div>
          )}
        </AnimatePresence>

        <motion.section id="integration" className="integration-strip" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.25 }}>
          <div className="integration-icon"><Cloud size={20} /></div>
          <div className="integration-copy">
            <span className="eyebrow">Cloudflare 集成</span>
            <strong>{integration.configured ? "已连接" : "尚未连接"}</strong>
            <span>{integration.configured
              ? integration.zone_id ? `${integration.account_id} · ${integration.zone_id}` : `${integration.account_id} · 仅账号权限`
              : "Quick 模式无需配置；Private / Public 模式需要账号权限"}</span>
          </div>
          <div className="integration-state">
            <span className={`status-dot ${integration.configured ? "ok" : "muted"}`} />
            <span>{integration.token_state || "未配置"}</span>
          </div>
          <button type="button" className="button button-secondary" onClick={() => setIntegrationOpen(true)}>
            <Settings2 size={16} />{integration.configured ? "管理连接" : "配置连接"}
          </button>
        </motion.section>

        <section id="services" className="services-section">
          <div className="section-heading">
            <div>
              <p className="eyebrow">发布目标</p>
              <h2>服务 <span>{services.length}</span></h2>
            </div>
            <button type="button" className="button button-primary" onClick={() => setEditor("new")}>
              <Plus size={17} />新建服务
            </button>
          </div>

          <div className="service-table-wrap">
            {loading ? (
              <div className="loading-state"><Spinner size={22} /><span>正在读取服务...</span></div>
            ) : services.length === 0 ? (
              <div className="empty-state">
                <div className="empty-icon"><TerminalSquare size={21} /></div>
                <strong>还没有发布服务</strong>
                <span>没有公网域名也可以从「临时公开」开始；需要权限控制时选择「私网受控」。</span>
                <button type="button" className="button button-primary" onClick={() => setEditor("new")}><Plus size={17} />新建服务</button>
              </div>
            ) : (
              <table className="service-table">
                <thead>
                  <tr><th>服务</th><th>模式</th><th>入口 / Origin</th><th>访问条件</th><th>状态</th><th><span className="sr-only">操作</span></th></tr>
                </thead>
                <tbody>
                  <AnimatePresence initial={false}>
                    {services.map((item) => {
                      const state = stateMeta(item.state);
                      const mode = serviceMode(item);
                      const modeInfo = modeMeta(mode);
                      const connector = connectors.find((entry) => entry.service_id === item.id);
                      const operationActive = operation?.service_id === item.id && ["pending", "running"].includes(operation.status);
                      const canStop = item.state === "active" || (item.state === "error" && connector?.running);
                      const deleteBlocked = item.state === "deploying" || item.state === "stopping" || item.state === "active";
                      const access = mode === "quick"
                        ? "无需 Access（临时地址）"
                        : item.allow_type === "email_domain" ? `域 · ${item.allow_value || "-"}` : item.allow_value || "-";
                      return (
                        <motion.tr key={item.id} layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                          <td data-label="服务"><div className="service-name"><strong>{item.name}</strong><span>{mode === "quick" ? "cloudflared 临时隧道" : mode === "private" ? "Cloudflare 私网路由" : item.hostname}</span></div></td>
                          <td data-label="模式"><span className={`mode-badge ${modeInfo.tone}`}>{modeInfo.label}</span></td>
                          <td data-label="入口 / Origin"><div className="endpoint-cell">
                            {item.public_url ? <a href={item.public_url} target="_blank" rel="noreferrer">{item.public_url}<ExternalLink size={13} /></a> : <strong>{mode === "public" ? item.hostname : mode === "private" ? item.hostname : "部署后生成"}</strong>}
                            <code>{item.origin_url}</code>
                          </div></td>
                          <td data-label="访问条件"><span className="access-value">{access}</span></td>
                          <td data-label="状态"><div className="state-cell"><span className={`state-badge ${state.tone}`}><span className="status-dot" />{state.label}</span>{connector?.running && <span className="connector-note">Connector 在线</span>}</div></td>
                          <td className="row-actions">
                            <button type="button" className="text-button" onClick={() => setEditor(item)} disabled={item.state === "deploying" || item.state === "stopping"}>编辑</button>
                            {canStop ? <button type="button" className="text-button stop-button" onClick={() => void stop(item)} disabled={operationActive} title="停止 Connector，保留 Cloudflare 资源"><Power size={14} />{operationActive && operation?.kind === "stop" ? "停止中" : "停止"}</button> : <button type="button" className="text-button deploy-button" onClick={() => void deploy(item)} disabled={item.state === "deploying" || item.state === "stopping" || operationActive}>{item.state === "deploying" ? "部署中" : item.state === "stopping" ? "停止中" : "部署"}</button>}
                            <button type="button" className="text-button delete-button" onClick={() => setDeleteTarget(item)} disabled={deleteBlocked || operationActive} title={deleteBlocked ? "请先停止服务" : serviceHasRemoteResources(item) ? "删除服务及其绑定的 Cloudflare 资源" : "删除本地服务记录"}><Trash2 size={14} />删除</button>
                          </td>
                        </motion.tr>
                      );
                    })}
                  </AnimatePresence>
                </tbody>
              </table>
            )}
          </div>
        </section>

        <AnimatePresence>
          {operation && (
            <motion.section className={`operation-panel ${operationMeta(operation.status).tone}`} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 12 }}>
              <div className="operation-heading"><div><p className="eyebrow">{operationLabel(operation.kind)}</p><strong>{services.find((item) => item.id === operation.service_id)?.name || operation.service_id}</strong></div><span className={`state-badge ${operationMeta(operation.status).tone}`}><span className="status-dot" />{operationMeta(operation.status).label}</span></div>
              <div className="operation-detail"><span>步骤：{operation.current_step || "等待开始"}</span><span>尝试 {operation.attempts}</span><code>{operation.operation_id}</code></div>
              {operation.error_message && <p className="operation-error">{operation.error_message}</p>}
            </motion.section>
          )}
        </AnimatePresence>

        <footer className="page-footer"><span>{activeConnectors} 个 Connector 在线</span><span>最后更新 {formatTime(services[0]?.updated_at)}</span></footer>
      </main>

      <AnimatePresence>
        {guideOpen && <GuideDialog onClose={closeGuide} onQuick={startQuick} onConfigure={startConfiguration} />}
        {integrationOpen && <IntegrationDialog initial={integration} zones={zones} token={adminToken} onClose={() => setIntegrationOpen(false)} onSaved={(next) => { setIntegration(next); setIntegrationOpen(false); void loadData(true); }} />}
        {editor && <ServiceDialog key={editor === "new" ? "new" : editor.id} value={editor === "new" ? null : editor} token={adminToken} onClose={() => setEditor(null)} onSaved={(saved) => { setEditor(null); setServices((current) => editor === "new" ? [...current, saved] : current.map((item) => item.id === saved.id ? saved : item)); setNotice(editor === "new" ? "服务已创建" : "服务已更新"); }} />}
        {deleteTarget && <DeleteDialog value={deleteTarget} onClose={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} />}
      </AnimatePresence>
      </div>
    </MotionConfig>
  );
}

function GuideDialog({ onClose, onQuick, onConfigure }: { onClose: () => void; onQuick: () => void; onConfigure: () => void }) {
  return (
    <motion.div
      className="dialog-layer"
      role="presentation"
      tabIndex={-1}
      initial={{ opacity: 0 }}
      animate={{ opacity: 1 }}
      exit={{ opacity: 0 }}
      onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }}
      onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}
    >
      <motion.aside
        className="dialog guide-dialog"
        role="dialog"
        aria-modal="true"
        aria-labelledby="guide-dialog-title"
        aria-describedby="guide-dialog-description"
        initial={{ x: 28, opacity: 0 }}
        animate={{ x: 0, opacity: 1 }}
        exit={{ x: 28, opacity: 0 }}
        transition={{ type: "spring", stiffness: 360, damping: 32 }}
      >
        <div className="dialog-header">
          <div className="guide-title-lockup">
            <div className="guide-icon" aria-hidden="true"><CircleHelp size={20} /></div>
            <div><p className="eyebrow">首次使用</p><h2 id="guide-dialog-title">快速开始</h2></div>
          </div>
          <button type="button" className="icon-button" onClick={onClose} aria-label="关闭使用指南" title="关闭使用指南"><X size={18} /></button>
        </div>
        <div className="guide-content">
          <p id="guide-dialog-description" className="guide-intro">先选适合你的出口方式。没有公网域名时也能马上开始，但不同方式的访问体验和权限边界不同。</p>
          <ol className="guide-steps">
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">1</span>
              <div>
                <h3>选择出口方式</h3>
                <p><strong>临时公开</strong>不需要域名、账号或 Token，会生成随机的 trycloudflare.com 地址；<strong>私网受控</strong>不需要公网域名，但访问者要加入同一个 Zero Trust 组织、安装并登录 WARP，还要让目标 IP 经过 Split Tunnel；<strong>自有域名</strong>适合普通浏览器访问和标准 Access 登录。</p>
                <div className="guide-links">
                  <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/" target="_blank" rel="noreferrer">Quick Tunnel 说明 <ExternalLink size={13} /></a>
                  <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/private-net/cloudflared/connect-cidr/" target="_blank" rel="noreferrer">私网路由说明 <ExternalLink size={13} /></a>
                  <a href="https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/" target="_blank" rel="noreferrer">Split Tunnels 配置 <ExternalLink size={13} /></a>
                </div>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">2</span>
              <div>
                <h3>按需连接 Cloudflare</h3>
                <p>Quick 模式可以跳过连接设置。Private 只需 Account ID 和 Token；Public 还需要目标 Zone ID。Token 只会写入本机的受限 Secret 文件，不会显示在列表或响应中。</p>
                <div className="guide-note">
                  <strong>Public / Private 的最小权限</strong>
                  <ul className="guide-permissions">
                    <li>Account / Cloudflare Tunnel / Edit</li>
                    <li>Account / Access: Apps and Policies / Edit</li>
                    <li>Account / Zero Trust / Write</li>
                    <li>Public 额外需要 Zone / DNS / Edit、Zone / Zone / Read</li>
                  </ul>
                </div>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">3</span>
              <div>
                <h3>填写 Origin</h3>
                <p>Origin URL 必须是运行 Connector 的机器可以访问的 HTTP/HTTPS 地址。Private 模式再填写同一台服务的私网 IP；Public 模式填写目标 Zone 下的主机名；Quick 模式不需要额外地址。</p>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">4</span>
              <div>
                <h3>部署并验证</h3>
                <p>点击部署后等待操作面板完成。Quick 完成后直接打开随机地址；Private 先确认设备已 enrollment 且目标 IP 已走 WARP，再用 WARP 访问私网 IP；Public 最后创建 DNS，再用普通浏览器访问并测试 Access 登录。</p>
              </div>
            </li>
          </ol>
          <p className="guide-footnote">不确定字段含义时，可以查看仓库 README 的中英文完整说明。</p>
        </div>
        <div className="dialog-actions guide-actions">
          <button type="button" className="button button-secondary" onClick={onConfigure}><Settings2 size={16} />配置 Cloudflare</button>
          <button type="button" className="button button-primary" onClick={onQuick}><ArrowRight size={16} />直接创建 Quick</button>
        </div>
      </motion.aside>
    </motion.div>
  );
}

function IntegrationDialog({ initial, zones, token, onClose, onSaved }: { initial: IntegrationStatus; zones: Zone[]; token: string; onClose: () => void; onSaved: (status: IntegrationStatus) => void }) {
  const [accountID, setAccountID] = useState(initial.account_id || "");
  const [zoneID, setZoneID] = useState(initial.zone_id || "");
  const [secret, setSecret] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      const status = await request<IntegrationStatus>("/api/v1/integrations/cloudflare", token, { method: "PUT", body: JSON.stringify({ account_id: accountID, zone_id: zoneID, token: secret }) });
      setSecret("");
      onSaved(status);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "Cloudflare 配置失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <motion.div className="dialog-layer" role="presentation" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }} onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}>
      <motion.aside className="dialog" role="dialog" aria-modal="true" aria-labelledby="cloudflare-dialog-title" initial={{ x: 28, opacity: 0 }} animate={{ x: 0, opacity: 1 }} exit={{ x: 28, opacity: 0 }} transition={{ type: "spring", stiffness: 360, damping: 32 }}>
        <div className="dialog-header"><div><p className="eyebrow">Cloudflare</p><h2 id="cloudflare-dialog-title">连接设置</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label="关闭" title="关闭"><X size={18} /></button></div>
        <form className="dialog-form" onSubmit={submit}>
          <label>Account ID<input value={accountID} onChange={(event) => setAccountID(event.target.value)} required /></label>
          <label>Zone ID <span className="label-hint">Public 模式需要，Quick / Private 可留空</span>{zones.length > 0 ? <select value={zoneID} onChange={(event) => setZoneID(event.target.value)}><option value="">不选择 Zone</option>{zones.map((zone) => <option key={zone.id} value={zone.id}>{zone.name} · {zone.id}</option>)}</select> : <input value={zoneID} onChange={(event) => setZoneID(event.target.value)} placeholder="没有 Zone 时留空" />}</label>
          <label>API Token<input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} placeholder="仅本次提交使用" autoComplete="new-password" required /></label>
          <p className="form-note">Token 至少需要 Cloudflare Tunnel Edit；Private 还需要 Access Apps and Policies Edit、Zero Trust Write，Public 另外需要 Zone DNS Edit 和 Zone Read。</p>
          {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
          <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? "验证中" : "保存并验证"}</button></div>
        </form>
      </motion.aside>
    </motion.div>
  );
}

function DeleteDialog({ value, onClose, onConfirm }: { value: Service; onClose: () => void; onConfirm: () => void }) {
  const hasRemote = serviceHasRemoteResources(value);
  return (
    <motion.div className="dialog-layer" role="presentation" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }} onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}>
      <motion.aside className="dialog delete-dialog" role="dialog" aria-modal="true" aria-labelledby="delete-dialog-title" aria-describedby="delete-dialog-description" initial={{ y: 16, opacity: 0 }} animate={{ y: 0, opacity: 1 }} exit={{ y: 16, opacity: 0 }} transition={{ type: "spring", stiffness: 360, damping: 32 }}>
        <div className="dialog-header">
          <div className="delete-title-lockup"><div className="delete-icon" aria-hidden="true"><Trash2 size={20} /></div><div><p className="eyebrow">删除服务</p><h2 id="delete-dialog-title">确认删除？</h2></div></div>
          <button type="button" className="icon-button" onClick={onClose} aria-label="关闭" title="关闭"><X size={18} /></button>
        </div>
        <div className="delete-content">
          <p id="delete-dialog-description">将删除「<strong>{value.name}</strong>」及其本地配置。</p>
          {hasRemote ? <p className="delete-warning">该服务绑定了 Cloudflare 资源。确认后会停止 Connector，并删除 TunnelBox 创建的 DNS、Access、私网路由和 Tunnel。删除后无法恢复。</p> : <p className="delete-note">当前没有已绑定的远端资源，只会删除本地服务记录。</p>}
        </div>
        <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="button" className="button button-danger" onClick={onConfirm}><Trash2 size={16} />确认删除</button></div>
      </motion.aside>
    </motion.div>
  );
}

function ServiceDialog({ value, token, onClose, onSaved }: { value: Service | null; token: string; onClose: () => void; onSaved: (service: Service) => void }) {
  const [form, setForm] = useState<ServiceForm>(() => {
    if (!value) return { ...emptyServiceForm };
    const mode = serviceMode(value);
    return {
      mode,
      name: value.name,
      hostname: mode === "quick" ? "" : value.hostname || "",
      origin_url: value.origin_url,
      allow_type: mode === "quick" ? "" : value.allow_type || "email",
      allow_value: mode === "quick" ? "" : value.allow_value || "",
    };
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const update = <K extends keyof ServiceForm>(key: K, next: ServiceForm[K]) => setForm((current) => ({ ...current, [key]: next }));

  function chooseMode(mode: ExposureMode) {
    setForm((current) => ({
      ...current,
      mode,
      hostname: mode === "quick" || current.mode !== mode ? "" : current.hostname,
      allow_type: mode === "quick" ? "" : current.allow_type || "email",
      allow_value: mode === "quick" ? "" : current.allow_value,
    }));
    setError("");
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      const payload = JSON.stringify(form);
      const saved = await request<Service>(value ? `/api/v1/services/${value.id}` : "/api/v1/services", token, { method: value ? "PATCH" : "POST", body: payload });
      onSaved(saved);
    } catch (caught) {
      setError(caught instanceof Error ? caught.message : "服务保存失败");
    } finally {
      setSaving(false);
    }
  }

  return (
    <motion.div className="dialog-layer" role="presentation" initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }} onMouseDown={(event) => { if (event.target === event.currentTarget) onClose(); }} onKeyDown={(event) => { if (event.key === "Escape") onClose(); }}>
      <motion.aside className="dialog" role="dialog" aria-modal="true" aria-labelledby="service-dialog-title" initial={{ x: 28, opacity: 0 }} animate={{ x: 0, opacity: 1 }} exit={{ x: 28, opacity: 0 }} transition={{ type: "spring", stiffness: 360, damping: 32 }}>
        <div className="dialog-header"><div><p className="eyebrow">Web 服务</p><h2 id="service-dialog-title">{value ? "编辑服务" : "新建服务"}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label="关闭" title="关闭"><X size={18} /></button></div>
        <form className="dialog-form" onSubmit={submit}>
          <div className="mode-picker" role="radiogroup" aria-label="发布方式">
            {modeOptions.map((option) => <button key={option.value} type="button" role="radio" aria-checked={form.mode === option.value} className={`mode-option ${form.mode === option.value ? "selected" : ""}`} onClick={() => chooseMode(option.value)}><strong>{option.label}</strong><span>{option.short}</span></button>)}
          </div>
          <div className={`mode-help ${form.mode}`}>
            {form.mode === "quick" && <><strong>没有域名也能马上分享</strong><span>cloudflared 会生成随机的 <code>trycloudflare.com</code> 地址。该地址是临时的，不带标准 Cloudflare Access 邮箱策略。</span></>}
            {form.mode === "private" && <><strong>不需要公网域名</strong><span>访问者需要加入同一个 Zero Trust 组织并使用 Cloudflare One Client/WARP；还要在 Split Tunnels 中让这个私网 IP 经过 WARP，Access 才会按下面的条件控制访问。</span></>}
            {form.mode === "public" && <><strong>普通浏览器 + Access</strong><span>需要你拥有的 Cloudflare Zone。TunnelBox 会创建 DNS CNAME，并在最后启用公网入口。</span></>}
          </div>
          <label>名称<input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="例如 内网文档" required maxLength={120} /></label>
          {form.mode !== "quick" && <label>{form.mode === "private" ? "私网 IP" : "公网域名"}<input value={form.hostname} onChange={(event) => update("hostname", event.target.value)} placeholder={form.mode === "private" ? "192.168.1.20" : "docs.example.com"} required /></label>}
          <label>Origin URL <span className="label-hint">Connector 所在环境必须能访问</span><input type="url" value={form.origin_url} onChange={(event) => update("origin_url", event.target.value)} placeholder={form.mode === "private" ? "http://192.168.1.20:8080" : "http://127.0.0.1:3000"} required /></label>
          {form.mode !== "quick" && <div className="form-grid"><label>允许条件<select value={form.allow_type || "email"} onChange={(event) => update("allow_type", event.target.value as AllowType)}><option value="email">指定邮箱</option><option value="email_domain">邮箱域名</option></select></label><label>条件值<input type={form.allow_type === "email" ? "email" : "text"} value={form.allow_value} onChange={(event) => update("allow_value", event.target.value)} placeholder={form.allow_type === "email" ? "you@example.com" : "example.com"} required /></label></div>}
          {form.mode === "quick" && <p className="form-note">部署完成后，随机公网地址会出现在服务列表中。停止 TunnelBox 后该地址失效。</p>}
          {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
          <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? "保存中" : "保存服务"}</button></div>
        </form>
      </motion.aside>
    </motion.div>
  );
}

export default App;
