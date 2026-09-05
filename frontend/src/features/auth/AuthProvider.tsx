import { useQueryClient } from '@tanstack/react-query';
import { useCallback, useEffect, useMemo, useState, useSyncExternalStore } from 'react';
import type { ReactNode } from 'react';

import {
  authApi,
  getSession,
  patchSessionUser,
  refreshSession,
  setSessionLostHandler,
  subscribeSession,
} from '@/lib/api';
import type { CurrentUser, Role, Session } from '@/lib/api';

import { AuthContext } from './auth-context';
import type { AuthContextValue, AuthStatus } from './auth-context';

/** Время до истечения токена, за которое запускается обновление.
 *  Обновляемся заранее, чтобы фоновая вкладка не встречала пользователя
 *  просроченной сессией сразу после возвращения. */
const REFRESH_LEAD_MS = 60_000;

export function AuthProvider({ children }: { children: ReactNode }) {
  const queryClient = useQueryClient();

  // Сессия живёт вне React: её обновляет транспортный слой при любом 401,
  // а не только действия пользователя. Подписка через useSyncExternalStore
  // избавляет от рассинхронизации между хранилищем и состоянием компонента.
  const session = useSyncExternalStore<Session | null>(subscribeSession, getSession, () => null);

  // Пока не проверена cookie, интерфейс не знает, гость перед ним или
  // вернувшийся пользователь. Показывать в этот момент кнопку «Войти»
  // значит мигать ей на каждой перезагрузке страницы.
  const [restoring, setRestoring] = useState(true);

  useEffect(() => {
    let cancelled = false;
    void refreshSession().finally(() => {
      if (!cancelled) setRestoring(false);
    });
    return () => {
      cancelled = true;
    };
  }, []);

  // Принудительный выход, когда обновление окончательно провалилось.
  useEffect(() => {
    setSessionLostHandler(() => {
      queryClient.clear();
    });
    return () => setSessionLostHandler(null);
  }, [queryClient]);

  // Упреждающее продление: без него длинная работа в одной вкладке
  // упирается в 401 на первом же действии после паузы.
  useEffect(() => {
    if (!session) return;

    const delay = Math.max(session.accessExpiresAt - Date.now() - REFRESH_LEAD_MS, 5_000);
    const timer = setTimeout(() => void refreshSession(), delay);
    return () => clearTimeout(timer);
  }, [session]);

  const status: AuthStatus = restoring ? 'loading' : session ? 'authenticated' : 'guest';
  const user = session?.user ?? null;

  const login = useCallback(
    async (loginValue: string, password: string): Promise<CurrentUser> => {
      const result = await authApi.login(loginValue, password);
      // Кеш прежнего пользователя обязан уйти: иначе после смены учётной
      // записи в списках на мгновение видны чужие сделки.
      queryClient.clear();
      return result.user;
    },
    [queryClient],
  );

  const register = useCallback<AuthContextValue['register']>(
    async (input) => {
      const result = await authApi.register(input);
      queryClient.clear();
      return result.user;
    },
    [queryClient],
  );

  const logout = useCallback(async () => {
    await authApi.logout();
    queryClient.clear();
  }, [queryClient]);

  const reload = useCallback(async () => {
    await refreshSession();
  }, []);

  const applyUser = useCallback((next: CurrentUser) => {
    patchSessionUser(next);
  }, []);

  const value = useMemo<AuthContextValue>(
    () => ({
      status,
      user,
      isVerified: Boolean(user?.email_verified),
      hasRole: (...roles: readonly Role[]) => Boolean(user && roles.includes(user.role)),
      login,
      register,
      logout,
      reload,
      applyUser,
    }),
    [status, user, login, register, logout, reload, applyUser],
  );

  return <AuthContext value={value}>{children}</AuthContext>;
}
