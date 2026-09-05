import { motion } from "framer-motion";
import { ArrowRight, CircleAlert } from "lucide-react";
import { useState } from "react";
import type { FormEvent } from "react";
import { request } from "../api/client";
import type { AuthState } from "../api/types";
import { Spinner } from "./Spinner";

type AuthScreenProps = {
  state: AuthState;
  onAuthenticated: () => void;
  error: string;
};

export function AuthScreen({ state, onAuthenticated, error }: AuthScreenProps) {
  const setup = state === "setup";
  const [password, setPassword] = useState("");
  const [confirmation, setConfirmation] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [message, setMessage] = useState(error);

  async function submit(event: FormEvent) {
    event.preventDefault();
    setMessage("");
    if (setup && password !== confirmation) {
      setMessage("两次输入的密码不一致");
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
      setMessage(caught instanceof Error ? caught.message : "登录失败");
    } finally {
      setSubmitting(false);
    }
  }

  if (state === "loading") return <div className="auth-shell"><Spinner size={24} /></div>;
  return (
    <div className="auth-shell">
      <motion.form className="auth-card" onSubmit={submit} initial={{ opacity: 0, y: 8 }} animate={{ opacity: 1, y: 0 }}>
        <img className="brand-mark" src="/assets/logo.svg" width="40" height="40" alt="" />
        <p className="eyebrow">TunnelBox 控制面</p>
        <h1>{setup ? "创建管理员密码" : "欢迎回来"}</h1>
        <p className="auth-intro">{setup ? "首次使用请设置密码，之后可直接登录控制面。" : "请输入管理员密码继续。"}</p>
        <label>管理员密码<input type="password" value={password} onChange={(event) => setPassword(event.target.value)} minLength={8} maxLength={256} autoComplete={setup ? "new-password" : "current-password"} autoFocus required /></label>
        {setup && <label>确认密码<input type="password" value={confirmation} onChange={(event) => setConfirmation(event.target.value)} minLength={8} maxLength={256} autoComplete="new-password" required /></label>}
        {(message || error) && <p className="form-error"><CircleAlert size={16} />{message || error}</p>}
        <button type="submit" className="button button-primary auth-submit" disabled={submitting}>{submitting ? <Spinner /> : <ArrowRight size={16} />}{submitting ? "请稍候" : setup ? "创建并登录" : "登录"}</button>
      </motion.form>
    </div>
  );
}
