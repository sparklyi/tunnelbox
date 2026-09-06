import { Languages } from "lucide-react";
import { useLocale } from "../i18n";

export function LanguageSwitch() {
  const { locale, setLocale, t } = useLocale();
  return (
    <div className="language-control">
      <Languages size={15} aria-hidden="true" />
      <div className="language-switch" role="group" aria-label={t("language.label")}>
        <button type="button" className={locale === "zh-CN" ? "active" : ""} aria-pressed={locale === "zh-CN"} onClick={() => setLocale("zh-CN")}>{t("language.chinese")}</button>
        <button type="button" className={locale === "en" ? "active" : ""} aria-pressed={locale === "en"} onClick={() => setLocale("en")}>{t("language.english")}</button>
      </div>
    </div>
  );
}
