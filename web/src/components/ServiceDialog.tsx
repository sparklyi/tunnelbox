import { CircleAlert, Save, X } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { AllowType, ExposureMode, Service, ServiceForm } from "../api/types";
import { useLocale } from "../i18n";
import { DialogFrame } from "./DialogFrame";
import { Spinner } from "./Spinner";

const emptyServiceForm: ServiceForm = {
  mode: "quick",
  name: "",
  hostname: "",
  origin_url: "http://",
  allow_type: "",
  allow_value: "",
};

type ServiceDialogProps = {
  value: Service | null;
  onClose: () => void;
  onSaved: (service: Service) => void;
};

export function ServiceDialog({ value, onClose, onSaved }: ServiceDialogProps) {
  const { t, errorMessage } = useLocale();
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
  const modeOptions: Array<{ value: ExposureMode; label: string; short: string }> = [
    { value: "quick", label: t("service.modeQuick"), short: "Quick" },
    { value: "private", label: t("service.modePrivate"), short: "Private" },
    { value: "public", label: t("service.modePublic"), short: "Public" },
  ];

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
      setError(errorMessage(caught, "error.serviceSave"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <DialogFrame className="service-dialog" labelledBy="service-dialog-title" onClose={onClose}>
      <div className="dialog-header"><div><p className="eyebrow">{t("service.dialogEyebrow")}</p><h2 id="service-dialog-title">{t(value ? "service.editTitle" : "service.createTitle")}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label={t("service.closeDialog")} title={t("service.closeDialog")}><X size={18} /></button></div>
      <form className="dialog-form" onSubmit={submit}>
        <div className="mode-picker" role="radiogroup" aria-label={t("service.exposureMode")}>
          {modeOptions.map((option) => <button key={option.value} type="button" role="radio" aria-checked={form.mode === option.value} className={`mode-option ${form.mode === option.value ? "selected" : ""}`} onClick={() => chooseMode(option.value)}><strong>{option.label}</strong><span>{option.short}</span></button>)}
        </div>
        <div className={`mode-help ${form.mode}`}>
          {form.mode === "quick" && <><strong>{t("service.quickHelpTitle")}</strong><span>{t("service.quickHelpBody")}</span></>}
          {form.mode === "private" && <><strong>{t("service.privateHelpTitle")}</strong><span>{t("service.privateHelpBody")}</span></>}
          {form.mode === "public" && <><strong>{t("service.publicHelpTitle")}</strong><span>{t("service.publicHelpBody")}</span></>}
        </div>
        <label>{t("service.name")}<input value={form.name} onChange={(event) => update("name", event.target.value)} placeholder={t("service.namePlaceholder")} required maxLength={120} /></label>
        {form.mode !== "quick" && <label>{t(form.mode === "private" ? "service.privateIP" : "service.publicHostname")}<input value={form.hostname} onChange={(event) => update("hostname", event.target.value)} placeholder={form.mode === "private" ? "192.168.1.20" : "docs.example.com"} required /></label>}
        <label>Origin URL <span className="label-hint">{t("service.originHint")}</span><input type="url" value={form.origin_url} onChange={(event) => update("origin_url", event.target.value)} placeholder={form.mode === "private" ? "http://192.168.1.20:8080" : "http://127.0.0.1:3000"} required /></label>
        {form.mode !== "quick" && <div className="form-grid"><label>{t("service.allowCondition")}<select value={form.allow_type || "email"} onChange={(event) => update("allow_type", event.target.value as AllowType)}><option value="email">{t("service.allowEmail")}</option><option value="email_domain">{t("service.allowDomain")}</option></select></label><label>{t("service.conditionValue")}<input type={form.allow_type === "email" ? "email" : "text"} value={form.allow_value} onChange={(event) => update("allow_value", event.target.value)} placeholder={form.allow_type === "email" ? "you@example.com" : "example.com"} required /></label></div>}
        {form.mode === "quick" && <p className="form-note">{t("service.quickNote")}</p>}
        {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
        <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>{t("common.cancel")}</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? t("service.saving") : t("service.save")}</button></div>
      </form>
    </DialogFrame>
  );
}
