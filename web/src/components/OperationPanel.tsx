import { AnimatePresence, motion } from "framer-motion";
import type { Operation, Service } from "../api/types";
import { operationLabel, operationMeta } from "../presentation";

type OperationPanelProps = {
  operation: Operation | null;
  services: Service[];
};

export function OperationPanel({ operation, services }: OperationPanelProps) {
  return (
    <AnimatePresence>
      {operation && (
        <motion.section className={`operation-panel ${operationMeta(operation.status).tone}`} initial={{ opacity: 0, y: 12 }} animate={{ opacity: 1, y: 0 }} exit={{ opacity: 0, y: 12 }}>
          <div className="operation-heading"><div><p className="eyebrow">{operationLabel(operation.kind)}</p><strong>{services.find((item) => item.id === operation.service_id)?.name || operation.service_id}</strong></div><span className={`state-badge ${operationMeta(operation.status).tone}`}><span className="status-dot" />{operationMeta(operation.status).label}</span></div>
          <div className="operation-detail"><span>步骤：{operation.current_step || "等待开始"}</span><span>尝试 {operation.attempts}</span><code>{operation.operation_id}</code></div>
          {operation.error_message && <p className="operation-error">{operation.error_message}</p>}
        </motion.section>
      )}
    </AnimatePresence>
  );
}
