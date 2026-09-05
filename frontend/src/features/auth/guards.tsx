import { Navigate, Outlet, useLocation } from 'react-router-dom';

import { FullPageSpinner } from '@/ui/Spinner';
import type { Role } from '@/lib/api';

import { useAuth } from './auth-context';

/**
 * Проверки на клиенте — это удобство, а не защита.
 *
 * Скрытая ссылка не мешает открыть адрес напрямую, а собранный бандл
 * читается целиком. Настоящее ограничение стоит на сервере: RequireAuth,
 * RequireRole и RequireVerified в маршрутизаторе Go. Здесь мы лишь
 * избавляем пользователя от заведомо пустых экранов и ответов 403.
 */

export function RequireAuth() {
  const { status } = useAuth();
  const location = useLocation();

  if (status === 'loading') return <FullPageSpinner label="Восстанавливаем сессию" />;

  if (status === 'guest') {
    // Адрес запоминается, чтобы после входа вернуть человека туда, куда он
    // шёл, а не на общую страницу кабинета.
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }
  return <Outlet />;
}

export function RequireRole({ roles }: { roles: readonly Role[] }) {
  const { status, user } = useAuth();
  const location = useLocation();

  if (status === 'loading') return <FullPageSpinner label="Проверяем доступ" />;
  if (!user) {
    return <Navigate to="/login" replace state={{ from: location.pathname + location.search }} />;
  }
  if (!roles.includes(user.role)) return <Navigate to="/403" replace />;

  return <Outlet />;
}

/** Гостевые страницы: вошедшему пользователю форма входа не нужна. */
export function RequireGuest() {
  const { status } = useAuth();

  if (status === 'loading') return <FullPageSpinner label="Загрузка" />;
  if (status === 'authenticated') return <Navigate to="/app" replace />;

  return <Outlet />;
}
