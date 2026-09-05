import { motion } from "framer-motion";
import { CircleAlert, Save, X } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { IntegrationStatus, Zone } from "../api/types";
import { Spinner } from "./Spinner";

type IntegrationDialogProps = {
  initial: IntegrationStatus;
  zones: Zone[];
  onClose: () => void;
  onSaved: (status: IntegrationStatus) => void;
};

export function IntegrationDialog({ initial, zones, onClose, onSaved }: IntegrationDialogProps) {
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
      const status = await request<IntegrationStatus>("/api/v1/integrations/cloudflare", {
        method: "PUT",
        body: JSON.stringify({ account_id: accountID, zone_id: zoneID, token: secret }),
      });
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
