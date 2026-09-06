import { Cloud, Globe2, Network, ShieldCheck, X } from "lucide-react";
import { useLocale } from "../i18n";
import { DialogFrame } from "./DialogFrame";

type AboutDialogProps = { onClose: () => void };

export function AboutDialog({ onClose }: AboutDialogProps) {
  const { t } = useLocale();
  const capabilities = [
    { icon: Globe2, title: t("about.quickTitle"), body: t("about.quickBody") },
    { icon: Network, title: t("about.privateTitle"), body: t("about.privateBody") },
    { icon: Cloud, title: t("about.publicTitle"), body: t("about.publicBody") },
    { icon: ShieldCheck, title: t("about.localTitle"), body: t("about.localBody") },
  ];
  return (
    <DialogFrame className="about-dialog" labelledBy="about-dialog-title" describedBy="about-dialog-description" onClose={onClose}>
      <div className="dialog-header">
        <div className="about-title-lockup">
          <img className="brand-mark" src="/assets/logo.svg" width="40" height="40" alt="" />
          <div><p className="eyebrow">{t("about.eyebrow")}</p><h2 id="about-dialog-title">TunnelBox</h2></div>
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label={t("common.close")} title={t("common.close")}><X size={18} /></button>
      </div>
      <div className="about-content">
        <h3>{t("about.title")}</h3>
        <p id="about-dialog-description" className="about-intro">{t("about.intro")}</p>
        <div className="about-capabilities">
          {capabilities.map(({ icon: Icon, title, body }) => (
            <section className="about-capability" key={title}>
              <Icon size={19} aria-hidden="true" />
              <div><h4>{title}</h4><p>{body}</p></div>
            </section>
          ))}
        </div>
      </div>
      <div className="dialog-actions about-actions"><button type="button" className="button button-primary" onClick={onClose}>{t("common.close")}</button></div>
    </DialogFrame>
  );
}
