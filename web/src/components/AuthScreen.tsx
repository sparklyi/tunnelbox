import { motion } from "framer-motion";
import { ArrowRight, CircleAlert, Info } from "lucide-react";
import { useEffect, useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { AuthState } from "../api/types";
import { useLocale } from "../i18n";
import { LanguageSwitch } from "./LanguageSwitch";
import { Spinner } from "./Spinner";

type AuthScreenProps = {
  state: AuthState;
  onAuthenticated: () => void;
  onAbout: () => void;
  error: string;
};

export function AuthScreen({ state, onAuthenticated, onAbout, error }: AuthScreenProps) {
  const { locale, t, errorMessage } = useLocale();
  const setup = state === "setup";
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState("");

  useEffect(() => setMessage(""), [locale]);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    if (setup && password !== confirmation) {
      setMessage(t("auth.passwordMismatch"));
      return;
    }
    setSubmitting(true);
    try {
      await request<void>(setup ? "/api/v1/auth/setup" : "/api/v1/auth/login", {
        method: "POST",
        body: JSON.stringify({ password }),
      });
      onAuthenticated();
    } catch (caught) {
      setMessage(errorMessage(caught, "error.login"));
    } finally {
      setSubmitting(false);
    }
  }

  if (state === "loading") return <div className="auth-shell"><Spinner size={24} /></div>;
  return (
    <div className="auth-shell">
      <motion.form className="auth-card" onSubmit={submit} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
        <div className="auth-toolbar"><LanguageSwitch /></div>
        <img className="brand-mark" src="/assets/logo.svg" width="40" height="40" alt="" />
        <p className="eyebrow">{t("auth.setupEyebrow")}</p>
        <h1>{t(setup ? "auth.setupTitle" : "auth.loginTitle")}</h1>
        <p className="auth-intro">{t(setup ? "auth.setupIntro" : "auth.loginIntro")}</p>
        <label>{t("auth.password")}<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} maxLength={256} autoComplete={setup ? "new-password" : "current-password"} autoFocus required /></label>
        {setup && <label>{t("auth.confirmPassword")}<input type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} minLength={8} maxLength={256} autoComplete="new-password" required /></label>}
        {(message || error) && <p className="form-error"><CircleAlert size={16} />{message || error}</p>}
        <button type="submit" className="button button-primary auth-submit" disabled={submitting}>{submitting ? <Spinner /> : <ArrowRight size={16} />}{submitting ? t("auth.wait") : t(setup ? "auth.setupAction" : "auth.loginAction")}</button>
        <button type="button" className="auth-about" onClick={onAbout}><Info size={15} />{t("common.learnMore")}</button>
      </motion.form>
    </div>
  );
}
