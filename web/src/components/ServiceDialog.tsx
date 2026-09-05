import { motion } from "framer-motion";
import { CircleAlert, Save, X } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { AllowType, ExposureMode, Service, ServiceForm } from "../api/types";
import { Spinner } from "./Spinner";

const emptyServiceForm: ServiceForm = {
  mode: "quick",
  name: "",
  hostname: "",
  origin_url: "http://",
  allow_type: "",
  allow_value: "",
};

const modeOptions: Array<{ value: ExposureMode; label: string; short: string }> = [
  { value: "quick", label: "临时公开", short: "Quick" },
  { value: "private", label: "私网受控", short: "Private" },
  { value: "public", label: "自有域名", short: "Public" },
];

type ServiceDialogProps = {
  value: Service | null;
  onClose: () => void;
  onSaved: (service: Service) => void;
};

export function ServiceDialog({ value, onClose, onSaved }: ServiceDialogProps) {
  const [form, setForm] = useState<ServiceForm>(() => {
    if (!value) return { ...emptyServiceForm };
    const mode = value.mode;
    return {
      mode,
      name: value.name,
      hostname: mode === "quick" ? "" : value.hostname || "",
      origin_url: value.origin_url,
      allow_type: mode === "quick" ? "" : value.allow_type || "email",
      allow_value: mode === "quick" ? "" : value.allow_value || "",
    };
  });
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const update = <K extends keyof ServiceForm>(key: K, next: ServiceForm[K]) => setForm((current) => ({ ...current, [key]: next }));

  function chooseMode(mode: ExposureMode) {
    setForm((current) => ({
      ...current,
      mode,
      hostname: mode === "quick" || current.mode !== mode ? "" : current.hostname,
      allow_type: mode === "quick" ? "" : current.allow_type || "email",
      allow_value: mode === "quick" ? "" : current.allow_value,
    }));
    setError("");
  }

  async function submit(event: FormEvent) {
    event.preventDefault();
    setSaving(true);
    setError("");
    try {
      const saved = await request<Service>(value ? `/api/v1/services/${value.id}` : "/api/v1/services", {
        method: value ? "PATCH" : "POST",
        body: JSON.stringify(form),
      });
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
          <div className="mode-picker" role="radiogroup" aria-label="发布方式">
            {modeOptions.map((option) => <button key={option.value} type="button" role="radio" aria-checked={form.mode === option.value} className={`mode-option ${form.mode === option.value ? "selected" : ""}`} onClick={() => chooseMode(option.value)}><strong>{option.label}</strong><span>{option.short}</span></button>)}
          </div>
          <div className={`mode-help ${form.mode}`}>
            {form.mode === "quick" && <><strong>没有域名也能马上分享</strong><span>cloudflared 会生成随机的 <code>trycloudflare.com</code> 地址。该地址是临时的，不带标准 Cloudflare Access 邮箱策略。</span></>}
            {form.mode === "private" && <><strong>不需要公网域名</strong><span>访问者需要加入同一个 Zero Trust 组织并使用 Cloudflare One Client/WARP；还要在 Split Tunnels 中让这个私网 IP 经过 WARP，Access 才会按下面的条件控制访问。</span></>}
            {form.mode === "public" && <><strong>普通浏览器 + Access</strong><span>需要你拥有的 Cloudflare Zone。TunnelBox 会创建 DNS CNAME，并在最后启用公网入口。</span></>}
          </div>
          <label>名称<input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder="例如 内网文档" required maxLength={120} /></label>
          {form.mode !== "quick" && <label>{form.mode === "private" ? "私网 IP" : "公网域名"}<input value={form.hostname} onChange={(event) => update("hostname", event.target.value)} placeholder={form.mode === "private" ? "192.168.1.20" : "docs.example.com"} required /></label>}
          <label>Origin URL <span className="label-hint">Connector 所在环境必须能访问</span><input type="url" value={form.origin_url} onChange={(event) => update("origin_url", event.target.value)} placeholder={form.mode === "private" ? "http://192.168.1.20:8080" : "http://127.0.0.1:3000"} required /></label>
          {form.mode !== "quick" && <div className="form-grid"><label>允许条件<select value={form.allow_type || "email"} onChange={(event) => update("allow_type", event.target.value as AllowType)}><option value="email">指定邮箱</option><option value="email_domain">邮箱域名</option></select></label><label>条件值<input type={form.allow_type === "email" ? "email" : "text"} value={form.allow_value} onChange={(event) => update("allow_value", event.target.value)} placeholder={form.allow_type === "email" ? "you@example.com" : "example.com"} required /></label></div>}
          {form.mode === "quick" && <p className="form-note">部署完成后，随机公网地址会出现在服务列表中。停止 TunnelBox 后该地址失效。</p>}
          {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
          <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>取消</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? "保存中" : "保存服务"}</button></div>
        </form>
      </motion.aside>
    </motion.div>
  );
}
