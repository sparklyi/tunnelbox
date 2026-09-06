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
  LogOut,
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

function GitHubMark() {
  return (
    <svg aria-hidden="true" viewBox="0 0 16 16" width="14" height="14" fill="currentColor">
      <path d="M6.766 11.328c-2.063-.25-3.516-1.734-3.516-3.656 0-.781.281-1.625.75-2.188-.203-.515-.172-1.609.063-2.062.625-.078 1.468.25 1.968.703.594-.187 1.219-.281 1.985-.281.765 0 1.39.094 1.953.265.484-.437 1.344-.765 1.969-.687.218.422.25 1.515.046 2.047.5.593.766 1.39.766 2.203 0 1.922-1.453 3.375-3.547 3.64.531.344.89 1.094.89 1.954v1.625c0 .468.391.734.86.547C13.781 14.359 16 11.53 16 8.03 16 3.61 12.406 0 7.984 0 3.563 0 0 3.61 0 8.031a7.88 7.88 0 0 0 5.172 7.422c.422.156.828-.125.828-.547v-1.25c-.219.094-.5.156-.75.156-1.031 0-1.64-.562-2.078-1.609-.172-.422-.36-.672-.719-.719-.187-.015-.25-.093-.25-.187 0-.188.313-.328.625-.328.453 0 .844.281 1.25.86.313.452.64.655 1.031.655s.641-.14 1-.5c.266-.265.47-.5.657-.656" />
    </svg>
  );
}

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

  async function logout() {
    try {
      await request<void>("/api/v1/auth/logout", { method: "POST" });
      setAuthState("login");
    } catch (caught) {
      setError(errorMessage(caught, "error.generic"));
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
            <button type="button" className="nav-item" onClick={() => setIntegrationOpen(true)} aria-haspopup="dialog" aria-expanded={integrationOpen} aria-label={t("app.cloudflare")} title={t("app.cloudflare")}><Cloud size={16} /><span>{t("app.cloudflare")}</span></button>
            <button type="button" className="nav-item" onClick={() => setAboutOpen(true)} aria-label={t("app.about")} title={t("app.about")}><Info size={16} /><span>{t("app.about")}</span></button>
            <button type="button" className="nav-item" onClick={() => setGuideOpen(true)} aria-label={t("app.guide")} title={t("app.guide")}><CircleHelp size={16} /><span>{t("app.guide")}</span></button>
          </nav>
          <div className="sidebar-bottom">
            <LanguageSwitch />
            <button type="button" className="sidebar-logout" onClick={() => void logout()} aria-label={t("app.logout")} title={t("app.logout")}><LogOut size={15} /><span>{t("app.logout")}</span></button>
            <div className="sidebar-footnote"><LockKeyhole size={15} /><span>{t("app.policyNote")}</span></div>
          </div>
        </aside>

        <main className="main-content">
          <header className="topbar">
            <div><p className="eyebrow">{t("app.workspacePath")}</p><h1>{t("app.publishTitle")}</h1></div>
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
              <div className="section-actions">
                <button className="icon-button" type="button" onClick={() => void loadData(true)} disabled={refreshing} title={t("app.refresh")} aria-label={t("app.refresh")}>
                  {refreshing ? <Spinner size={17} /> : <RefreshCw size={17} />}
                </button>
                <button type="button" className="button button-primary" onClick={() => setEditor("new")}><Plus size={17} />{t("app.createService")}</button>
              </div>
            </div>
            <ServiceTable services={services} connectors={connectors} operation={operation} loading={loading} onCreate={() => setEditor("new")} onEdit={setEditor} onDeploy={(item) => void deploy(item)} onStop={(item) => void stop(item)} onDelete={setDeleteTarget} />
          </section>

          <OperationPanel operation={operation} services={services} onClose={() => setOperation(null)} />
          <div className="page-status"><span>{t("app.connectorOnline", { count: activeConnectors })}</span><span>{t("app.lastUpdated", { time: formatTime(services[0]?.updated_at, locale) })}</span></div>
          <footer className="page-footer">
            <span>&copy; 2026 TunnelBox contributors</span><span aria-hidden="true">&middot;</span>
            <a href="https://github.com/sparklyi/tunnelbox" target="_blank" rel="noreferrer"><GitHubMark /><span>GitHub</span></a><span aria-hidden="true">&middot;</span>
            <a href="https://github.com/sparklyi/tunnelbox/blob/main/LICENSE" target="_blank" rel="noreferrer">MIT License</a>
          </footer>
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
