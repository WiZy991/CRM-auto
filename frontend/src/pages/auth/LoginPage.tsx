import { useState, type FormEvent } from 'react';
import { Link, useLocation, useNavigate } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { errorMessage, isApiError } from '@/lib/api';
import { BrandLogo, Button, TextField } from '@/ui';

export function LoginPage() {
  const { login } = useAuth();
  const navigate = useNavigate();
  const location = useLocation();
  const fromPath = (location.state as { from?: string } | null)?.from;
  const from = fromPath && fromPath.startsWith('/') ? fromPath : '/app';

  const [loginValue, setLoginValue] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError('');
    setPending(true);
    try {
      await login(loginValue.trim(), password);
      void navigate(from, { replace: true });
    } catch (caught) {
      setError(
        isApiError(caught) && caught.status === 401
          ? 'Неверный логин или пароль'
          : errorMessage(caught),
      );
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="mx-auto max-w-md px-4 py-10 md:py-16">
      <BrandLogo className="mb-8" subtitle="Китай · Япония" />
      <p className="text-sm font-medium text-[var(--accent)]">Вход</p>
      <h1 className="mt-2 text-2xl font-semibold md:text-3xl">В кабинет</h1>
      <p className="mt-3 text-sm text-[var(--text-secondary)]">
        Покупатель видит сделки и заявки, дилер — воронку и витрину, поставщик — свою карточку.
      </p>

      <form onSubmit={(event) => void onSubmit(event)} className="mt-8 flex flex-col gap-4">
        <TextField
          label="Почта или телефон"
          autoComplete="username"
          required
          value={loginValue}
          onChange={(event) => setLoginValue(event.target.value)}
        />
        <TextField
          label="Пароль"
          type="password"
          autoComplete="current-password"
          required
          value={password}
          onChange={(event) => setPassword(event.target.value)}
        />
        {error && (
          <p className="text-sm text-lacquer-600" role="alert">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" loading={pending} block>
          Войти
        </Button>
      </form>

      <p className="mt-6 text-sm text-[var(--text-secondary)]">
        Нет учётки?{' '}
        <Link to="/register" className="text-[var(--link)] underline underline-offset-4">
          Регистрация
        </Link>
      </p>
    </div>
  );
}
