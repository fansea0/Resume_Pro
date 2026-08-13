import { useState, type FormEvent } from 'react';
import { login, register, type AuthUser } from '../api/auth';

interface AuthScreenProps {
  onAuthenticated: (user: AuthUser) => void;
}

type AuthMode = 'login' | 'register';

export function AuthScreen({ onAuthenticated }: AuthScreenProps) {
  const [mode, setMode] = useState<AuthMode>('login');
  const [email, setEmail] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState<string | null>(null);
  const [submitting, setSubmitting] = useState(false);
  const isRegistering = mode === 'register';

  const switchMode = (next: AuthMode) => {
    setMode(next);
    setError(null);
  };

  const submit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    setError(null);
    if (!email.trim()) {
      setError('请输入邮箱地址。');
      return;
    }
    if (password.length < 12) {
      setError('密码至少需要 12 个字符。');
      return;
    }
    setSubmitting(true);
    try {
      const user = await (isRegistering ? register : login)({ email: email.trim(), password });
      onAuthenticated(user);
    } catch (reason) {
      setError(reason instanceof Error ? reason.message : '请求失败，请稍后重试。');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <section className="auth-screen" aria-labelledby="auth-title">
      <div className="auth-screen__intro">
        <p className="auth-screen__eyebrow">RESUME PRO</p>
        <h2 id="auth-title">{isRegistering ? '创建你的简历空间' : '欢迎回来'}</h2>
        <p>登录后，你的简历将保存在独立的个人工作区中。</p>
      </div>
      <form className="auth-form" onSubmit={submit}>
        <label className="editor-field" htmlFor="auth-email"><span>邮箱</span><input autoComplete="email" id="auth-email" onChange={(event) => setEmail(event.target.value)} type="email" value={email} /></label>
        <label className="editor-field" htmlFor="auth-password"><span>密码</span><input autoComplete={isRegistering ? 'new-password' : 'current-password'} id="auth-password" onChange={(event) => setPassword(event.target.value)} type="password" value={password} /></label>
        {isRegistering && <p className="auth-form__hint">密码至少 12 个字符。</p>}
        {error && <p className="auth-form__error" role="alert">{error}</p>}
        <button disabled={submitting} type="submit">{submitting ? '请稍候…' : isRegistering ? '创建账号' : '登录'}</button>
      </form>
      <p className="auth-screen__switch">
        {isRegistering ? '已有账号？' : '还没有账号？'}
        <button onClick={() => switchMode(isRegistering ? 'login' : 'register')} type="button">{isRegistering ? '登录' : '注册账号'}</button>
      </p>
    </section>
  );
}
