import { AnimatePresence, motion } from "framer-motion";
import { ExternalLink, Plus, Power, TerminalSquare, Trash2 } from "lucide-react";
import type { Connector, Operation, Service } from "../api/types";
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
  return (
    <div className="service-table-wrap">
      {loading ? (
        <div className="loading-state"><Spinner size={22} /><span>正在读取服务...</span></div>
      ) : services.length === 0 ? (
        <div className="empty-state">
          <div className="empty-icon"><TerminalSquare size={21} /></div>
          <strong>还没有发布服务</strong>
          <span>没有公网域名也可以从「临时公开」开始；需要权限控制时选择「私网受控」。</span>
          <button type="button" className="button button-primary" onClick={onCreate}><Plus size={17} />新建服务</button>
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
                const mode = item.mode;
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
                      <button type="button" className="text-button" onClick={() => onEdit(item)} disabled={item.state === "deploying" || item.state === "stopping"}>编辑</button>
                      {canStop ? <button type="button" className="text-button stop-button" onClick={() => onStop(item)} disabled={operationActive} title="停止 Connector，保留 Cloudflare 资源"><Power size={14} />{operationActive && operation?.kind === "stop" ? "停止中" : "停止"}</button> : <button type="button" className="text-button deploy-button" onClick={() => onDeploy(item)} disabled={item.state === "deploying" || item.state === "stopping" || operationActive}>{item.state === "deploying" ? "部署中" : item.state === "stopping" ? "停止中" : "部署"}</button>}
                      <button type="button" className="text-button delete-button" onClick={() => onDelete(item)} disabled={deleteBlocked || operationActive} title={deleteBlocked ? "请先停止服务" : serviceHasRemoteResources(item) ? "删除服务及其绑定的 Cloudflare 资源" : "删除本地服务记录"}><Trash2 size={14} />删除</button>
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
