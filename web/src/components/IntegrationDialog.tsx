import { CircleAlert, Save, X } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { IntegrationStatus, Zone } from "../api/types";
import { useLocale } from "../i18n";
import { DialogFrame } from "./DialogFrame";
import { Spinner } from "./Spinner";

type IntegrationDialogProps = {
  initial: IntegrationStatus;
  zones: Zone[];
  onClose: () => void;
  onSaved: (status: IntegrationStatus) => void;
};

export function IntegrationDialog({ initial, zones, onClose, onSaved }: IntegrationDialogProps) {
  const { t, errorMessage } = useLocale();
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
      setError(errorMessage(caught, "error.integrationSave"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <DialogFrame className="integration-dialog" labelledBy="cloudflare-dialog-title" onClose={onClose}>
      <div className="dialog-header"><div><p className="eyebrow">Cloudflare</p><h2 id="cloudflare-dialog-title">{t("integration.title")}</h2></div><button type="button" className="icon-button" onClick={onClose} aria-label={t("integration.close")} title={t("integration.close")}><X size={18} /></button></div>
      <form className="dialog-form" onSubmit={submit}>
        <label>Account ID<input value={accountID} onChange={(event) => setAccountID(event.target.value)} required /></label>
        <label>Zone ID <span className="label-hint">{t("integration.zoneHint")}</span>{zones.length > 0 ? <select value={zoneID} onChange={(event) => setZoneID(event.target.value)}><option value="">{t("integration.noZone")}</option>{zones.map((zone) => <option key={zone.id} value={zone.id}>{zone.name} · {zone.id}</option>)}</select> : <input value={zoneID} onChange={(event) => setZoneID(event.target.value)} placeholder={t("integration.zonePlaceholder")} />}</label>
        <label>API Token<input type="password" value={secret} onChange={(event) => setSecret(event.target.value)} placeholder={t("integration.tokenPlaceholder")} autoComplete="new-password" required /></label>
        <p className="form-note">{t("integration.permissionNote")}</p>
        {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
        <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>{t("common.cancel")}</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? t("integration.validating") : t("integration.save")}</button></div>
      </form>
    </DialogFrame>
  );
}
