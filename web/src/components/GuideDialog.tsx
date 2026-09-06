import { ArrowRight, CircleHelp, ExternalLink, Settings2, X } from "lucide-react";
import { useLocale } from "../i18n";
import { DialogFrame } from "./DialogFrame";

type GuideDialogProps = {
  onClose: () => void;
  onQuick: () => void;
  onConfigure: () => void;
};

export function GuideDialog({ onClose, onQuick, onConfigure }: GuideDialogProps) {
  const { t } = useLocale();
  return (
    <DialogFrame className="guide-dialog" labelledBy="guide-dialog-title" describedBy="guide-dialog-description" onClose={onClose}>
      <div className="dialog-header">
        <div className="guide-title-lockup">
          <div className="guide-icon" aria-hidden="true"><CircleHelp size={20} /></div>
          <div><p className="eyebrow">{t("guide.eyebrow")}</p><h2 id="guide-dialog-title">{t("guide.title")}</h2></div>
        </div>
        <button type="button" className="icon-button" onClick={onClose} aria-label={t("guide.close")} title={t("guide.close")}><X size={18} /></button>
      </div>
      <div className="guide-content">
        <p id="guide-dialog-description" className="guide-intro">{t("guide.intro")}</p>
        <ol className="guide-steps">
          <li className="guide-step">
            <span className="guide-step-number" aria-hidden="true">1</span>
            <div>
              <h3>{t("guide.step1Title")}</h3>
              <p>{t("guide.step1Body")}</p>
              <div className="guide-links">
                <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/do-more-with-tunnels/trycloudflare/" target="_blank" rel="noreferrer">{t("guide.quickDocs")} <ExternalLink size={13} /></a>
                <a href="https://developers.cloudflare.com/cloudflare-one/networks/connectors/cloudflare-tunnel/private-net/cloudflared/connect-cidr/" target="_blank" rel="noreferrer">{t("guide.privateDocs")} <ExternalLink size={13} /></a>
                <a href="https://developers.cloudflare.com/cloudflare-one/team-and-resources/devices/cloudflare-one-client/configure/route-traffic/split-tunnels/" target="_blank" rel="noreferrer">{t("guide.splitDocs")} <ExternalLink size={13} /></a>
              </div>
            </div>
          </li>
          <li className="guide-step">
            <span className="guide-step-number" aria-hidden="true">2</span>
            <div>
              <h3>{t("guide.step2Title")}</h3>
              <p>{t("guide.step2Body")}</p>
              <div className="guide-note">
                <strong>{t("guide.permissions")}</strong>
                <ul className="guide-permissions">
                  <li>Account / Cloudflare Tunnel / Edit</li>
                  <li>Account / Access: Apps and Policies / Edit</li>
                  <li>Account / Zero Trust / Write</li>
                  <li>{t("guide.publicPermission")}</li>
                </ul>
              </div>
            </div>
          </li>
          <li className="guide-step"><span className="guide-step-number" aria-hidden="true">3</span><div><h3>{t("guide.step3Title")}</h3><p>{t("guide.step3Body")}</p></div></li>
          <li className="guide-step"><span className="guide-step-number" aria-hidden="true">4</span><div><h3>{t("guide.step4Title")}</h3><p>{t("guide.step4Body")}</p></div></li>
        </ol>
        <p className="guide-footnote">{t("guide.footnote")}</p>
      </div>
      <div className="dialog-actions guide-actions">
        <button type="button" className="button button-secondary" onClick={onConfigure}><Settings2 size={16} />{t("guide.configure")}</button>
        <button type="button" className="button button-primary" onClick={onQuick}><ArrowRight size={16} />{t("guide.createQuick")}</button>
      </div>
    </DialogFrame>
  );
}
