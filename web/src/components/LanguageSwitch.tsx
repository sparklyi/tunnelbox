import { Languages } from "lucide-react";
import { useLocale } from "../i18n";

export function LanguageSwitch() {
  const { locale, setLocale, t } = useLocale();
  const nextLocale = locale === "zh-CN" ? "en" : "zh-CN";
  const label = nextLocale === "en" ? t("language.switchToEnglish") : t("language.switchToChinese");
  return (
    <button type="button" className="icon-button language-button" onClick={() => setLocale(nextLocale)} aria-label={label} title={label}>
      <Languages size={17} aria-hidden="true" />
    </button>
  );
}
