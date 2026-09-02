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
  RefreshCw,
  Save,
  Settings2,
  TerminalSquare,
  X,
} from "lucide-react";
import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";

type ServiceState = "draft" | "deploying" | "active" | "error";
type AllowType = "email" | "email_domain";

type Service = {
  id: string;
  name: string;
  hostname: string;
  origin_url: string;
  allow_type: AllowType;
  allow_value: string;
  state: ServiceState;
  tunnel_id?: string;
  dns_record_id?: string;
  access_application_id?: string;
  access_policy_id?: string;
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
type Connector = { service_id: string; running: boolean; healthy: boolean; message?: string };

type ServiceForm = {
  name: string;
  hostname: string;
  origin_url: string;
  allow_type: AllowType;
  allow_value: string;
};

const emptyIntegration: IntegrationStatus = { configured: false };
const emptyServiceForm: ServiceForm = {
  name: "",
  hostname: "",
  origin_url: "http://",
  allow_type: "email",
  allow_value: "",
};

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
    case "error":
      return { label: "异常", tone: "error" };
    default:
      return { label: "草稿", tone: "draft" };
  }
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
  const [integration, setIntegration] = useState<IntegrationStatus>(emptyIntegration);
  const [zones, setZones] = useState<Zone[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [authRequired, setAuthRequired] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editor, setEditor] = useState<Service | "new" | null>(null);
  const [guideOpen, setGuideOpen] = useState(() => !hasDismissedOnboarding());

  const loadData = useCallback(
    async (showRefresh = false) => {
      if (showRefresh) setRefreshing(true);
      else setLoading(true);
      setError("");
      try {
        const [status, servicePayload, connectorPayload] = await Promise.all([
          request<IntegrationStatus>("/api/v1/integrations/cloudflare/status", adminTokenRef.current),
          request<{ services: Service[] }>("/api/v1/services", adminTokenRef.current),
          request<{ connectors: Connector[] }>("/api/v1/connectors", adminTokenRef.current),
        ]);
        setIntegration(status);
        setServices(servicePayload.services || []);
        setConnectors(connectorPayload.connectors || []);
        if (status.configured) {
          const zonePayload = await request<{ zones: Zone[] }>("/api/v1/zones", adminTokenRef.current);
          setZones(zonePayload.zones || []);
        } else {
          setZones([]);
        }
        setAuthRequired(false);
      } catch (caught) {
        if (caught instanceof ApiError && caught.status === 401) {
          setAuthRequired(true);
          setError("需要管理员令牌才能访问控制面");
        } else {
          setError(caught instanceof Error ? caught.message : "无法加载控制面数据");
        }
      } finally {
        setLoading(false);
        setRefreshing(false);
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

  function applyToken() {
    const next = tokenDraft.trim();
    if (next === adminToken) return;
    adminTokenRef.current = next;
    setAdminToken(next);
    storeToken(next);
    void loadData(true);
  }

  function closeGuide() {
    rememberOnboardingDismissed();
    setGuideOpen(false);
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
            <label className="token-field">
              <LockKeyhole size={14} aria-hidden="true" />
              <span className="sr-only">管理员令牌</span>
              <input
                type="password"
                value={tokenDraft}
                onChange={(event) => setTokenDraft(event.target.value)}
                onBlur={applyToken}
                onKeyDown={(event) => { if (event.key === "Enter") { event.preventDefault(); applyToken(); } }}
                placeholder="管理员令牌"
                autoComplete="off"
              />
            </label>
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
            <span>{integration.configured ? `${integration.account_id} · ${integration.zone_id}` : "配置后才能创建 Tunnel 和 Access 策略"}</span>
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
                <span>创建一个 Web Origin，随后由 Cloudflare Access 保护它。</span>
                <button type="button" className="button button-primary" onClick={() => setEditor("new")}><Plus size={17} />新建服务</button>
              </div>
            ) : (
              <table className="service-table">
                <thead>
                  <tr><th>服务</th><th>Origin</th><th>访问条件</th><th>状态</th><th><span className="sr-only">操作</span></th></tr>
                </thead>
                <tbody>
                  <AnimatePresence initial={false}>
                    {services.map((item) => {
                      const state = stateMeta(item.state);
                      const connector = connectors.find((entry) => entry.service_id === item.id);
                      return (
                        <motion.tr key={item.id} layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                          <td data-label="服务"><div className="service-name"><strong>{item.name}</strong><span>{item.hostname}</span></div></td>
                          <td data-label="Origin"><code>{item.origin_url}</code></td>
                          <td data-label="访问条件"><span className="access-value">{item.allow_type === "email_domain" ? `域 · ${item.allow_value}` : item.allow_value}</span></td>
                          <td data-label="状态"><div className="state-cell"><span className={`state-badge ${state.tone}`}><span className="status-dot" />{state.label}</span>{connector?.running && <span className="connector-note">Connector 在线</span>}</div></td>
                          <td className="row-actions"><button type="button" className="text-button" onClick={() => setEditor(item)}>编辑</button><button type="button" className="text-button deploy-button" onClick={() => void deploy(item)} disabled={item.state === "deploying"}>{item.state === "deploying" ? "部署中" : "部署"}</button></td>
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
              <div className="operation-heading"><div><p className="eyebrow">最近操作</p><strong>{services.find((item) => item.id === operation.service_id)?.name || operation.service_id}</strong></div><span className={`state-badge ${operationMeta(operation.status).tone}`}><span className="status-dot" />{operationMeta(operation.status).label}</span></div>
              <div className="operation-detail"><span>步骤：{operation.current_step || "等待开始"}</span><span>尝试 {operation.attempts}</span><code>{operation.operation_id}</code></div>
              {operation.error_message && <p className="operation-error">{operation.error_message}</p>}
            </motion.section>
          )}
        </AnimatePresence>

        <footer className="page-footer"><span>{activeConnectors} 个 Connector 在线</span><span>最后更新 {formatTime(services[0]?.updated_at)}</span></footer>
      </main>

      <AnimatePresence>
        {guideOpen && <GuideDialog onClose={closeGuide} onStart={startConfiguration} />}
        {integrationOpen && <IntegrationDialog initial={integration} zones={zones} token={adminToken} onClose={() => setIntegrationOpen(false)} onSaved={(next) => { setIntegration(next); setIntegrationOpen(false); void loadData(true); }} />}
        {editor && <ServiceDialog value={editor === "new" ? null : editor} token={adminToken} onClose={() => setEditor(null)} onSaved={(saved) => { setEditor(null); setServices((current) => editor === "new" ? [...current, saved] : current.map((item) => item.id === saved.id ? saved : item)); setNotice(editor === "new" ? "服务已创建" : "服务已更新"); }} />}
      </AnimatePresence>
      </div>
    </MotionConfig>
  );
}

function GuideDialog({ onClose, onStart }: { onClose: () => void; onStart: () => void }) {
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
          <p id="guide-dialog-description" className="guide-intro">准备好 Cloudflare 信息后，按下面顺序即可发布一个受保护的 Web 服务。</p>
          <ol className="guide-steps">
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">1</span>
              <div>
                <h3>准备 Cloudflare 信息</h3>
                <p>需要 Account ID、Zone ID 和自定义 API Token。它们都可以在 Cloudflare Dashboard 找到。</p>
                <div className="guide-links">
                  <a href="https://developers.cloudflare.com/fundamentals/account/find-account-and-zone-ids/" target="_blank" rel="noreferrer">查找 Account/Zone ID <ExternalLink size={13} /></a>
                  <a href="https://developers.cloudflare.com/fundamentals/api/get-started/create-token/" target="_blank" rel="noreferrer">Token 创建说明 <ExternalLink size={13} /></a>
                  <a href="https://dash.cloudflare.com/profile/api-tokens" target="_blank" rel="noreferrer">创建 API Token <ExternalLink size={13} /></a>
                </div>
                <div className="guide-note">
                  <strong>最小权限</strong>
                  <ul className="guide-permissions">
                    <li>Account / Cloudflare Tunnel / Edit</li>
                    <li>Account / Access: Apps and Policies / Edit</li>
                    <li>Zone / DNS / Edit</li>
                    <li>Zone / Zone / Read</li>
                  </ul>
                </div>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">2</span>
              <div>
                <h3>连接 Cloudflare</h3>
                <p>在连接设置中填入三个值并保存验证。API Token 只会写入本机的受限 Secret 文件，不会显示在列表或响应中。</p>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">3</span>
              <div>
                <h3>填写 Web 服务</h3>
                <p>公网域名填写目标 Zone 下的完整主机名；Origin URL 必须是运行 Connector 的机器可以访问的 HTTP/HTTPS 地址；Allow 条件填写允许登录的邮箱或邮箱域名。</p>
              </div>
            </li>
            <li className="guide-step">
              <span className="guide-step-number" aria-hidden="true">4</span>
              <div>
                <h3>部署并验证</h3>
                <p>点击部署后等待操作面板完成。TunnelBox 会先准备 Tunnel、Connector 和 Access，最后创建 DNS；完成后访问公网域名测试登录和授权。</p>
              </div>
            </li>
          </ol>
          <p className="guide-footnote">不确定字段含义时，可以查看仓库 README 的中英文完整说明。</p>
        </div>
        <div className="dialog-actions guide-actions">
          <button type="button" className="button button-secondary" onClick={onClose}>关闭指南</button>
          <button type="button" className="button button-primary" onClick={onStart}><ArrowRight size={16} />开始配置</button>
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
          <label>Zone ID{zones.length > 0 ? <select value={zoneID} onChange={(event) => setZoneID(event.target.value)} required><option value="">选择 Zone</option>{zones.map((zone) => <option key={zone.id} value={zone.id}>{zone.name} · {zone.id}</option>)}</select> : <input value={zoneID} onChange={(event) => setZoneID(event.target.value)} placeholder="例如 023e..." required />}</label>
          <label>API Token<input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} placeholder="仅本次提交使用" autoComplete="new-password" required /></label>
          {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
          <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? "验证中" : "保存并验证"}</button></div>
        </form>
      </motion.aside>
    </motion.div>
  );
}

function ServiceDialog({ value, token, onClose, onSaved }: { value: Service | null; token: string; onClose: () => void; onSaved: (service: Service) => void }) {
  const [form, setForm] = useState<ServiceForm>(value ? { name: value.name, hostname: value.hostname, origin_url: value.origin_url, allow_type: value.allow_type, allow_value: value.allow_value } : emptyServiceForm);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const update = <K extends keyof ServiceForm>(key: K, next: ServiceForm[K]) => setForm((current) => ({ ...current, [key]: next }));

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
          <label>名称<input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="例如 内网文档" required maxLength={120} /></label>
          <label>公网域名<input value={form.hostname} onChange={(event) => update("hostname", event.target.value)} placeholder="docs.example.com" required /></label>
          <label>Origin URL<input type="url" value={form.origin_url} onChange={(event) => update("origin_url", event.target.value)} placeholder="http://192.168.1.20:8080" required /></label>
          <div className="form-grid"><label>允许条件<select value={form.allow_type} onChange={(event) => update("allow_type", event.target.value as AllowType)}><option value="email">指定邮箱</option><option value="email_domain">邮箱域名</option></select></label><label>条件值<input type={form.allow_type === "email" ? "email" : "text"} value={form.allow_value} onChange={(event) => update("allow_value", event.target.value)} placeholder={form.allow_type === "email" ? "you@example.com" : "example.com"} required /></label></div>
          {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
          <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? "保存中" : "保存服务"}</button></div>
        </form>
      </motion.aside>
    </motion.div>
  );
}

export default App;
