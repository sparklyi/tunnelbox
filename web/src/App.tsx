import { AnimatePresence, MotionConfig, motion } from "framer-motion";
import {
  Check,
  ChevronDown,
  CircleAlert,
  CircleHelp,
  Cloud,
  LockKeyhole,
  Plus,
  RefreshCw,
  Settings2,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError, request } from "./api/client";
import type { AuthState, Connector, IntegrationStatus, Operation, Service, Zone } from "./api/types";
import { AuthScreen } from "./components/AuthScreen";
import { DeleteDialog } from "./components/DeleteDialog";
import { GuideDialog } from "./components/GuideDialog";
import { IntegrationDialog } from "./components/IntegrationDialog";
import { OperationPanel } from "./components/OperationPanel";
import { ServiceDialog } from "./components/ServiceDialog";
import { ServiceTable } from "./components/ServiceTable";
import { Spinner } from "./components/Spinner";
import { useOperationPolling } from "./hooks/useOperationPolling";
import { formatTime } from "./presentation";

const emptyIntegration: IntegrationStatus = { configured: false };

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

function App() {
  const [authState, setAuthState] = useState<AuthState>("loading");
  const loadGenerationRef = useRef(0);
  const [integration, setIntegration] = useState<IntegrationStatus>(emptyIntegration);
  const [zones, setZones] = useState<Zone[]>([]);
  const [services, setServices] = useState<Service[]>([]);
  const [connectors, setConnectors] = useState<Connector[]>([]);
  const [operation, setOperation] = useState<Operation | null>(null);
  const [loading, setLoading] = useState(true);
  const [refreshing, setRefreshing] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [integrationOpen, setIntegrationOpen] = useState(false);
  const [editor, setEditor] = useState<Service | "new" | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Service | null>(null);
  const [guideOpen, setGuideOpen] = useState(() => !hasDismissedOnboarding());

  const loadData = useCallback(
    async (showRefresh = false): Promise<boolean> => {
      const generation = ++loadGenerationRef.current;
      const isCurrentLoad = () => generation === loadGenerationRef.current;
      if (showRefresh) setRefreshing(true);
      else setLoading(true);
      if (isCurrentLoad()) setError("");
      try {
        const [status, servicePayload, connectorPayload] = await Promise.all([
          request<IntegrationStatus>("/api/v1/integrations/cloudflare/status"),
          request<{ services: Service[] }>("/api/v1/services"),
          request<{ connectors: Connector[] }>("/api/v1/connectors"),
        ]);
        if (!isCurrentLoad()) return false;
        setIntegration(status);
        setServices(servicePayload.services || []);
        setConnectors(connectorPayload.connectors || []);
        if (status.configured && status.zone_id) {
          const zonePayload = await request<{ zones: Zone[] }>("/api/v1/zones");
          if (!isCurrentLoad()) return false;
          setZones(zonePayload.zones || []);
        } else {
          setZones([]);
        }
        return true;
      } catch (caught) {
        if (!isCurrentLoad()) return false;
        if (caught instanceof ApiError && caught.status === 401) {
          setAuthState("login");
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
    void request<{ initialized: boolean }>("/api/v1/auth/status")
      .then((status) => setAuthState(status.initialized ? "login" : "setup"))
      .catch(() => setError("无法读取登录状态"));
  }, []);

  useEffect(() => {
    if (authState === "authenticated") void loadData();
  }, [authState, loadData]);

  useOperationPolling(operation, setOperation, loadData, setError);

  const activeConnectors = useMemo(() => connectors.filter((item) => item.running).length, [connectors]);

  async function deploy(item: Service) {
    setError("");
    setNotice("");
    try {
      const next = await request<Operation>(`/api/v1/services/${item.id}/deploy`, { method: "POST" });
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
      const next = await request<Operation>(`/api/v1/services/${item.id}/stop`, { method: "POST" });
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
      const next = await request<Operation | null>(`/api/v1/services/${item.id}`, { method: "DELETE" });
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

  if (authState !== "authenticated") {
    return <AuthScreen state={authState} onAuthenticated={() => setAuthState("authenticated")} error={error} />;
  }

  return (
    <MotionConfig reducedMotion="user">
      <div className="app-shell">
      <aside className="sidebar">
        <div className="brand-lockup">
          <img className="brand-mark" src="/assets/logo.svg" width="40" height="40" alt="" />
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
            <button type="button" className="button button-secondary" onClick={async () => { await request<void>("/api/v1/auth/logout", { method: "POST" }); setAuthState("login"); }}><LockKeyhole size={15} />退出登录</button>
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

          <ServiceTable
            services={services}
            connectors={connectors}
            operation={operation}
            loading={loading}
            onCreate={() => setEditor("new")}
            onEdit={setEditor}
            onDeploy={(item) => void deploy(item)}
            onStop={(item) => void stop(item)}
            onDelete={setDeleteTarget}
          />
        </section>

        <OperationPanel operation={operation} services={services} />

        <footer className="page-footer"><span>{activeConnectors} 个 Connector 在线</span><span>最后更新 {formatTime(services[0]?.updated_at)}</span></footer>
      </main>

      <AnimatePresence>
        {guideOpen && <GuideDialog onClose={closeGuide} onQuick={startQuick} onConfigure={startConfiguration} />}
        {integrationOpen && <IntegrationDialog initial={integration} zones={zones} onClose={() => setIntegrationOpen(false)} onSaved={(next) => { setIntegration(next); setIntegrationOpen(false); void loadData(true); }} />}
        {editor && <ServiceDialog key={editor === "new" ? "new" : editor.id} value={editor === "new" ? null : editor} onClose={() => setEditor(null)} onSaved={(saved) => { setEditor(null); setServices((current) => editor === "new" ? [...current, saved] : current.map((item) => item.id === saved.id ? saved : item)); setNotice(editor === "new" ? "服务已创建" : "服务已更新"); }} />}
        {deleteTarget && <DeleteDialog value={deleteTarget} onClose={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} />}
      </AnimatePresence>
      </div>
    </MotionConfig>
  );
}

export default App;
