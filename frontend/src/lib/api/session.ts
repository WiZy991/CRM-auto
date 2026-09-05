/**
 * Хранилище текущей сессии на клиенте.
 *
 * Токен доступа держится только в памяти вкладки. В localStorage он не
 * попадает намеренно: при межсайтовом внедрении скрипта содержимое хранилища
 * выгружается одной строкой, а переменную модуля из чужого контекста не
 * достать. Долгоживущий refresh лежит в HttpOnly-cookie и коду недоступен —
 * поэтому «запомнить меня» обеспечивает не память вкладки, а обновление
 * сессии по cookie при загрузке приложения.
 */

import type { CurrentUser } from './types';

export interface Session {
  readonly user: CurrentUser;
  readonly accessToken: string;
  readonly accessExpiresAt: number;
  readonly csrfToken: string;
}

type Listener = (session: Session | null) => void;

let current: Session | null = null;
const listeners = new Set<Listener>();

export function getSession(): Session | null {
  return current;
}

export function setSession(next: Session | null): void {
  current = next;
  for (const listener of listeners) listener(current);
}

export function patchSessionUser(user: CurrentUser): void {
  if (!current) return;
  setSession({
    user,
    accessToken: current.accessToken,
    accessExpiresAt: current.accessExpiresAt,
    csrfToken: current.csrfToken,
  });
}

export function subscribeSession(listener: Listener): () => void {
  listeners.add(listener);
  return () => {
    listeners.delete(listener);
  };
}

/**
 * Токен считается просроченным чуть заранее.
 *
 * Часы браузера и сервера расходятся, а запрос идёт по сети ненулевое время.
 * Без запаса вполне рабочий на момент отправки токен успевает протухнуть в
 * пути, и пользователь получает случайные 401 на каждом длинном запросе.
 */
const EXPIRY_SKEW_MS = 30_000;

export function accessTokenExpired(session: Session | null = current): boolean {
  if (!session) return true;
  return Date.now() >= session.accessExpiresAt - EXPIRY_SKEW_MS;
}

/**
 * Значение cookie защиты от подделки запроса.
 *
 * Читается из document.cookie, а не из памяти: сервер обновляет её при каждом
 * продлении сессии, и cookie всегда свежее сохранённой копии.
 */
export function readCsrfCookie(): string {
  const prefix = 'ai_csrf=';
  for (const chunk of document.cookie.split('; ')) {
    if (chunk.startsWith(prefix)) {
      return decodeURIComponent(chunk.slice(prefix.length));
    }
  }
  return '';
}
