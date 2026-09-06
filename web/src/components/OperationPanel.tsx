import { AnimatePresence, motion } from "framer-motion";
import { X } from "lucide-react";
import type { Operation, Service } from "../api/types";
import { useLocale } from "../i18n";
import { operationLabel, operationMeta, operationStepLabel } from "../presentation";

type OperationPanelProps = {
  operation: Operation | null;
  services: Service[];
  onClose: () => void;
};

export function OperationPanel({ operation, services, onClose }: OperationPanelProps) {
  const { t, codeMessage } = useLocale();
  const terminal = operation && ["succeeded", "failed", "unknown"].includes(operation.status);
  return (
    <AnimatePresence>
      {operation && (
        <motion.section className={`operation-panel ${operationMeta(operation.status, t).tone}`} aria-live="polite" initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 8 }}>
          <div className="operation-heading">
            <div><p className="eyebrow">{operationLabel(operation.kind, t)}</p><strong>{services.find((item) => item.id === operation.service_id)?.name || operation.service_id}</strong></div>
            <div className="operation-heading-actions"><span className={`state-badge ${operationMeta(operation.status, t).tone}`}><span className="status-dot" />{operationMeta(operation.status, t).label}</span>{terminal && <button type="button" className="panel-close" onClick={onClose} aria-label={t("operation.close")} title={t("operation.close")}><X size={16} /></button>}</div>
          </div>
          <div className="operation-detail"><span>{t("operation.step", { step: operationStepLabel(operation.current_step, t) })}</span><span>{t("operation.attempt", { count: operation.attempts })}</span><code>{operation.operation_id}</code></div>
          {operation.error_message && <p className="operation-error">{codeMessage(operation.error_code, "operation.errorFallback")}</p>}
        </motion.section>
      )}
    </AnimatePresence>
  );
}
