import { CircleAlert, KeyRound, Save, X } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import { useLocale } from "../i18n";
import { DialogFrame } from "./DialogFrame";
import { Spinner } from "./Spinner";

type ChangePasswordDialogProps = {
  onClose: () => void;
  onSaved: () => void;
};

export function ChangePasswordDialog({ onClose, onSaved }: ChangePasswordDialogProps) {
  const { t, errorMessage } = useLocale();
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");

  async function submit(event: FormEvent) {
    event.preventDefault();
    setError("");
    if (newPassword !== confirmation) {
      setError(t("auth.passwordMismatch"));
      return;
    }
    setSaving(true);
    try {
      await request<void>("/api/v1/auth/password", {
        method: "PUT",
        body: JSON.stringify({ current_password: currentPassword, new_password: newPassword }),
      });
      onSaved();
    } catch (caught) {
      setError(errorMessage(caught, "error.changePassword"));
    } finally {
      setSaving(false);
    }
  }

  return (
    <DialogFrame className="password-dialog" labelledBy="password-dialog-title" describedBy="password-dialog-description" onClose={onClose}>
      <div className="dialog-header">
        <div className="password-title-lockup">
          <div className="password-icon" aria-hidden="true"><KeyRound size={19} /></div>
          <div><p className="eyebrow">{t("auth.changePasswordEyebrow")}</p><h2 id="password-dialog-title">{t("auth.changePasswordTitle")}</h2></div>
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label={t("auth.closeChangePassword")} title={t("auth.closeChangePassword")}><X size={18} /></button>
      </div>
      <form className="dialog-form" onSubmit={submit}>
        <p id="password-dialog-description" className="form-note">{t("auth.changePasswordIntro")}</p>
        <label>{t("auth.currentPassword")}<input type="password" value={currentPassword} onChange={(event) => setCurrentPassword(event.target.value)} autoComplete="current-password" required /></label>
        <label>{t("auth.newPassword")}<input type="password" value={newPassword} onChange={(event) => setNewPassword(event.target.value)} minLength={8} maxLength={72} autoComplete="new-password" required /></label>
        <label>{t("auth.confirmPassword")}<input type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} minLength={8} maxLength={72} autoComplete="new-password" required /></label>
        {error && <p className="form-error"><CircleAlert size={16} />{error}</p>}
        <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>{t("common.cancel")}</button><button type="submit" className="button button-primary" disabled={saving}>{saving ? <Spinner /> : <Save size={16} />}{saving ? t("auth.wait") : t("auth.changePasswordAction")}</button></div>
      </form>
    </DialogFrame>
  );
}
