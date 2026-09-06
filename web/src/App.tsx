import { AnimatePresence, MotionConfig, motion } from "framer-motion";
import {
  Boxes,
  Check,
  CircleAlert,
  CircleHelp,
  Cloud,
  Info,
  LayoutList,
  LockKeyhole,
  Plus,
  RefreshCw,
  Settings2,
  X,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { ApiError, request } from "./api/client";
import type { AuthState, Connector, IntegrationStatus, Operation, Service, Zone } from "./api/types";
import { AboutDialog } from "./components/AboutDialog";
import { AuthScreen } from "./components/AuthScreen";
import { DeleteDialog } from "./components/DeleteDialog";
import { GuideDialog } from "./components/GuideDialog";
import { IntegrationDialog } from "./components/IntegrationDialog";
import { LanguageSwitch } from "./components/LanguageSwitch";
import { OperationPanel } from "./components/OperationPanel";
import { ServiceDialog } from "./components/ServiceDialog";
import { ServiceTable } from "./components/ServiceTable";
import { Spinner } from "./components/Spinner";
import { useOperationPolling } from "./hooks/useOperationPolling";
import { useLocale } from "./i18n";
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
  const { locale, t, errorMessage } = useLocale();
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
  const [aboutOpen, setAboutOpen] = useState(false);

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
          setError(errorMessage(caught, "error.loadData"));
        }
        return false;
      } finally {
        if (isCurrentLoad()) {
          setLoading(false);
          setRefreshing(false);
        }
      }
    },
    [errorMessage]
  );

  useEffect(() => {
    void request<{ initialized: boolean }>("/api/v1/auth/status")
      .then((status) => setAuthState(status.initialized ? "login" : "setup"))
      .catch(() => setError(t("error.authStatus")));
  }, []);

  useEffect(() => {
    if (authState === "authenticated") void loadData();
  }, [authState, loadData]);

  useEffect(() => {
    setError("");
    setNotice("");
  }, [locale]);

  useEffect(() => {
    if (!notice) return;
    const timer = window.setTimeout(() => setNotice(""), 4000);
    return () => window.clearTimeout(timer);
  }, [notice]);

  useEffect(() => {
    if (!error) return;
    const timer = window.setTimeout(() => setError(""), 8000);
    return () => window.clearTimeout(timer);
  }, [error]);

  useEffect(() => {
    if (!operation || !["succeeded", "failed", "unknown"].includes(operation.status)) return;
    const operationID = operation.operation_id;
    const timer = window.setTimeout(() => {
      setOperation((current) => current?.operation_id === operationID ? null : current);
    }, 8000);
    return () => window.clearTimeout(timer);
  }, [operation]);

  useOperationPolling(operation, setOperation, loadData, setError);

  const activeConnectors = useMemo(() => connectors.filter((item) => item.running).length, [connectors]);

  async function deploy(item: Service) {
    setError("");
    setNotice("");
    try {
      const next = await request<Operation>(`/api/v1/services/${item.id}/deploy`, { method: "POST" });
      setOperation(next);
      setNotice(t("notice.deployStarted", { name: item.name }));
      await loadData(true);
    } catch (caught) {
      setError(errorMessage(caught, "error.deploy"));
    }
  }

  async function stop(item: Service) {
    setError("");
    setNotice("");
    try {
      const next = await request<Operation>(`/api/v1/services/${item.id}/stop`, { method: "POST" });
      setOperation(next);
      setNotice(t("notice.stopStarted", { name: item.name }));
      await loadData(true);
    } catch (caught) {
      setError(errorMessage(caught, "error.stop"));
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
        setNotice(t("notice.deleteStarted", { name: item.name }));
      } else {
        setServices((current) => current.filter((entry) => entry.id !== item.id));
        setNotice(t("notice.deleted", { name: item.name }));
      }
      await loadData(true);
    } catch (caught) {
      setError(errorMessage(caught, "error.delete"));
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

  function tokenStateLabel(value?: string) {
    if (value === "active") return t("integration.tokenActive");
    if (value === "inactive") return t("integration.tokenInactive");
    if (value === "expired") return t("integration.tokenExpired");
    return value || t("app.notConfigured");
  }

  if (authState !== "authenticated") {
    return (
      <MotionConfig reducedMotion="user">
        <AuthScreen state={authState} onAuthenticated={() => setAuthState("authenticated")} onAbout={() => setAboutOpen(true)} error={error} />
        <AnimatePresence>{aboutOpen && <AboutDialog onClose={() => setAboutOpen(false)} />}</AnimatePresence>
      </MotionConfig>
    );
  }

  return (
    <MotionConfig reducedMotion="user">
      <div className="app-shell">
        <aside className="sidebar">
          <div className="brand-lockup">
            <img className="brand-mark" src="/assets/logo.svg" width="40" height="40" alt="" />
            <div><strong>TunnelBox</strong><span>{t("app.controlPlane")}</span></div>
          </div>
          <div className="workspace-switcher">
            <span className="eyebrow">{t("app.workspace")}</span>
            <div className="workspace-current" title={t("app.currentWorkspace")}><Boxes size={16} /><span>Default</span></div>
          </div>
          <nav className="side-nav" aria-label={t("app.mainNavigation")}>
            <a className="nav-item active" href="#services" aria-label={t("app.services")} title={t("app.services")}><LayoutList size={16} /><span>{t("app.services")}</span></a>
            <a className="nav-item" href="#integration" aria-label={t("app.cloudflare")} title={t("app.cloudflare")}><Cloud size={16} /><span>{t("app.cloudflare")}</span></a>
            <button type="button" className="nav-item" onClick={() => setAboutOpen(true)} aria-label={t("app.about")} title={t("app.about")}><Info size={16} /><span>{t("app.about")}</span></button>
            <button type="button" className="nav-item" onClick={() => setGuideOpen(true)} aria-label={t("app.guide")} title={t("app.guide")}><CircleHelp size={16} /><span>{t("app.guide")}</span></button>
          </nav>
          <div className="sidebar-bottom">
            <LanguageSwitch />
            <div className="sidebar-footnote"><LockKeyhole size={15} /><span>{t("app.policyNote")}</span></div>
          </div>
        </aside>

        <main className="main-content">
          <header className="topbar">
            <div><p className="eyebrow">{t("app.workspacePath")}</p><h1>{t("app.publishTitle")}</h1></div>
            <div className="topbar-actions">
              <button type="button" className="button button-secondary" onClick={async () => { try { await request<void>("/api/v1/auth/logout", { method: "POST" }); setAuthState("login"); } catch (caught) { setError(errorMessage(caught, "error.generic")); } }}><LockKeyhole size={15} />{t("app.logout")}</button>
              <button className="icon-button" type="button" onClick={() => void loadData(true)} title={t("app.refresh")} aria-label={t("app.refresh")}>
                {refreshing ? <Spinner size={17} /> : <RefreshCw size={17} />}
              </button>
            </div>
          </header>

          <div className="toast-region" aria-live="polite" aria-atomic="true">
            <AnimatePresence initial={false}>
              {error && (
                <motion.div key="error" className="toast toast-error" role="alert" initial={{ opacity: 0, x: 18 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 18 }}>
                  <CircleAlert size={17} /><span>{error}</span><button type="button" className="toast-close" onClick={() => setError("")} aria-label={t("app.closeNotice")} title={t("app.closeNotice")}><X size={16} /></button>
                </motion.div>
              )}
              {notice && (
                <motion.div key="notice" className="toast toast-success" role="status" initial={{ opacity: 0, x: 18 }} animate={{ opacity: 1, x: 0 }} exit={{ opacity: 0, x: 18 }}>
                  <Check size={17} /><span>{notice}</span><button type="button" className="toast-close" onClick={() => setNotice("")} aria-label={t("app.closeNotice")} title={t("app.closeNotice")}><X size={16} /></button>
                </motion.div>
              )}
            </AnimatePresence>
          </div>

          <motion.section id="integration" className="integration-strip" initial={{ opacity: 0 }} animate={{ opacity: 1 }} transition={{ duration: 0.25 }}>
            <div className="integration-icon"><Cloud size={20} /></div>
            <div className="integration-copy">
              <span className="eyebrow">{t("app.integration")}</span>
              <strong>{t(integration.configured ? "app.connected" : "app.notConnected")}</strong>
              <span>{integration.configured
                ? integration.zone_id ? t("app.accountAndZone", { account: integration.account_id || "-", zone: integration.zone_id }) : t("app.accountOnly", { account: integration.account_id || "-" })
                : t("app.integrationHint")}</span>
            </div>
            <div className="integration-state"><span className={`status-dot ${integration.configured ? "ok" : "muted"}`} /><span>{tokenStateLabel(integration.token_state)}</span></div>
            <button type="button" className="button button-secondary" onClick={() => setIntegrationOpen(true)}><Settings2 size={16} />{t(integration.configured ? "app.manageConnection" : "app.configureConnection")}</button>
          </motion.section>

          <section id="services" className="services-section">
            <div className="section-heading">
              <div><p className="eyebrow">{t("app.publishTargets")}</p><h2>{t("app.services")} <span>{services.length}</span></h2></div>
              <button type="button" className="button button-primary" onClick={() => setEditor("new")}><Plus size={17} />{t("app.createService")}</button>
            </div>
            <ServiceTable services={services} connectors={connectors} operation={operation} loading={loading} onCreate={() => setEditor("new")} onEdit={setEditor} onDeploy={(item) => void deploy(item)} onStop={(item) => void stop(item)} onDelete={setDeleteTarget} />
          </section>

          <OperationPanel operation={operation} services={services} onClose={() => setOperation(null)} />
          <footer className="page-footer"><span>{t("app.connectorOnline", { count: activeConnectors })}</span><span>{t("app.lastUpdated", { time: formatTime(services[0]?.updated_at, locale) })}</span></footer>
        </main>

        <AnimatePresence>
          {aboutOpen && <AboutDialog onClose={() => setAboutOpen(false)} />}
          {guideOpen && <GuideDialog onClose={closeGuide} onQuick={startQuick} onConfigure={startConfiguration} />}
          {integrationOpen && <IntegrationDialog initial={integration} zones={zones} onClose={() => setIntegrationOpen(false)} onSaved={(next) => { setIntegration(next); setIntegrationOpen(false); void loadData(true); }} />}
          {editor && <ServiceDialog key={editor === "new" ? "new" : editor.id} value={editor === "new" ? null : editor} onClose={() => setEditor(null)} onSaved={(saved) => { setEditor(null); setServices((current) => editor === "new" ? [...current, saved] : current.map((item) => item.id === saved.id ? saved : item)); setNotice(t(editor === "new" ? "notice.serviceCreated" : "notice.serviceUpdated")); }} />}
          {deleteTarget && <DeleteDialog value={deleteTarget} onClose={() => setDeleteTarget(null)} onConfirm={() => void remove(deleteTarget)} />}
        </AnimatePresence>
      </div>
    </MotionConfig>
  );
}

export default App;
