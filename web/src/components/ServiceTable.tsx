import { AnimatePresence, motion } from "framer-motion";
import { ExternalLink, Plus, Power, TerminalSquare, Trash2 } from "lucide-react";
import type { Connector, Operation, Service } from "../api/types";
import { useLocale } from "../i18n";
import { modeMeta, serviceHasRemoteResources, stateMeta } from "../presentation";
import { Spinner } from "./Spinner";

type ServiceTableProps = {
  services: Service[];
  connectors: Connector[];
  operation: Operation | null;
  loading: boolean;
  onCreate: () => void;
  onEdit: (service: Service) => void;
  onDeploy: (service: Service) => void;
  onStop: (service: Service) => void;
  onDelete: (service: Service) => void;
};

export function ServiceTable({ services, connectors, operation, loading, onCreate, onEdit, onDeploy, onStop, onDelete }: ServiceTableProps) {
  const { t } = useLocale();
  return (
    <div className="service-table-wrap">
      {loading ? (
        <div className="loading-state"><Spinner size={22} /><span>{t("service.loading")}</span></div>
      ) : services.length === 0 ? (
        <div className="empty-state">
          <div className="empty-icon"><TerminalSquare size={21} /></div>
          <strong>{t("service.emptyTitle")}</strong>
          <span>{t("service.emptyBody")}</span>
          <button type="button" className="button button-primary" onClick={onCreate}><Plus size={17} />{t("app.createService")}</button>
        </div>
      ) : (
        <table className="service-table">
          <thead>
            <tr><th>{t("service.columnService")}</th><th>{t("service.columnMode")}</th><th>{t("service.columnEndpoint")}</th><th>{t("service.columnAccess")}</th><th>{t("service.columnStatus")}</th><th><span className="sr-only">{t("service.columnActions")}</span></th></tr>
          </thead>
          <tbody>
            <AnimatePresence initial={false}>
              {services.map((item) => {
                const state = stateMeta(item.state, t);
                const mode = item.mode;
                const modeInfo = modeMeta(mode, t);
                const connector = connectors.find((entry) => entry.service_id === item.id);
                const operationActive = operation?.service_id === item.id && ["pending", "running"].includes(operation.status);
                const canStop = item.state === "active" || (item.state === "error" && connector?.running);
                const deleteBlocked = item.state === "deploying" || item.state === "stopping" || item.state === "active";
                const access = mode === "quick"
                  ? t("service.noAccess")
                  : item.allow_type === "email_domain" ? t("service.domainAccess", { value: item.allow_value || "-" }) : item.allow_value || "-";
                return (
                  <motion.tr key={item.id} layout initial={{ opacity: 0 }} animate={{ opacity: 1 }} exit={{ opacity: 0 }}>
                    <td data-label={t("service.columnService")}><div className="service-name"><strong>{item.name}</strong><span>{mode === "quick" ? t("service.quickConnector") : mode === "private" ? t("service.privateRoute") : item.hostname}</span></div></td>
                    <td data-label={t("service.columnMode")}><span className={`mode-badge ${modeInfo.tone}`}>{modeInfo.label}</span></td>
                    <td data-label={t("service.columnEndpoint")}><div className="endpoint-cell">
                      {item.public_url ? <a href={item.public_url} target="_blank" rel="noreferrer">{item.public_url}<ExternalLink size={13} /></a> : <strong>{mode === "public" || mode === "private" ? item.hostname : t("service.generatedAfterDeploy")}</strong>}
                      <code>{item.origin_url}</code>
                    </div></td>
                    <td data-label={t("service.columnAccess")}><span className="access-value">{access}</span></td>
                    <td data-label={t("service.columnStatus")}><div className="state-cell"><span className={`state-badge ${state.tone}`}><span className="status-dot" />{state.label}</span>{connector?.running && <span className="connector-note">{t("service.connectorOnline")}</span>}</div></td>
                    <td className="row-actions">
                      <button type="button" className="text-button" onClick={() => onEdit(item)} disabled={item.state === "deploying" || item.state === "stopping"}>{t("service.edit")}</button>
                      {canStop ? <button type="button" className="text-button stop-button" onClick={() => onStop(item)} disabled={operationActive} title={t("service.stopHint")}><Power size={14} />{operationActive && operation?.kind === "stop" ? t("service.stopping") : t("service.stop")}</button> : <button type="button" className="text-button deploy-button" onClick={() => onDeploy(item)} disabled={item.state === "deploying" || item.state === "stopping" || operationActive}>{item.state === "deploying" ? t("service.deploying") : item.state === "stopping" ? t("service.stopping") : t("service.deploy")}</button>}
                      <button type="button" className="text-button delete-button" onClick={() => onDelete(item)} disabled={deleteBlocked || operationActive} title={deleteBlocked ? t("service.deleteBlocked") : serviceHasRemoteResources(item) ? t("service.deleteRemoteHint") : t("service.deleteLocalHint")}><Trash2 size={14} />{t("service.delete")}</button>
                    </td>
                  </motion.tr>
                );
              })}
            </AnimatePresence>
          </tbody>
        </table>
      )}
    </div>
  );
}
