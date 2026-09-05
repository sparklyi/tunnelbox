import { motion } from "framer-motion";
import { Trash2, X } from "lucide-react";
import type { Service } from "../api/types";
import { serviceHasRemoteResources } from "../presentation";

type DeleteDialogProps = {
  value: Service;
  onClose: () => void;
  onConfirm: () => void;
};

export function DeleteDialog({ value, onClose, onConfirm }: DeleteDialogProps) {
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
