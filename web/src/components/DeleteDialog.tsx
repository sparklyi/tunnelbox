import { Trash2, X } from "lucide-react";
import type { Service } from "../api/types";
import { useLocale } from "../i18n";
import { serviceHasRemoteResources } from "../presentation";
import { DialogFrame } from "./DialogFrame";

type DeleteDialogProps = {
  value: Service;
  onClose: () => void;
  onConfirm: () => void;
};

export function DeleteDialog({ value, onClose, onConfirm }: DeleteDialogProps) {
  const { t } = useLocale();
  const hasRemote = serviceHasRemoteResources(value);
  return (
    <DialogFrame className="delete-dialog" labelledBy="delete-dialog-title" describedBy="delete-dialog-description" onClose={onClose}>
      <div className="dialog-header">
        <div className="delete-title-lockup"><div className="delete-icon" aria-hidden="true"><Trash2 size={20} /></div><div><p className="eyebrow">{t("delete.eyebrow")}</p><h2 id="delete-dialog-title">{t("delete.title")}</h2></div></div>
        <button type="button" className="icon-button" onClick={onClose} aria-label={t("delete.close")} title={t("delete.close")}><X size={18} /></button>
      </div>
      <div className="delete-content">
        <p id="delete-dialog-description">{t("delete.description", { name: value.name })}</p>
        {hasRemote ? <p className="delete-warning">{t("delete.remoteWarning")}</p> : <p className="delete-note">{t("delete.localNote")}</p>}
      </div>
      <div className="dialog-actions"><button type="button" className="button button-secondary" onClick={onClose}>{t("common.cancel")}</button><button type="button" className="button button-danger" onClick={onConfirm}><Trash2 size={16} />{t("delete.confirm")}</button></div>
    </DialogFrame>
  );
}
