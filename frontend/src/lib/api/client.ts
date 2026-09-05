/**
 * Транспорт до API.
 *
 * Три вещи, ради которых существует этот слой:
 *
 *  1. Единый разбор ошибок. Сервер отвечает конвертом {"error": {...}},
 *     и превращать его в ApiError в каждом вызове — путь к разнобою.
 *  2. Прозрачное обновление сессии. Access-токен живёт минуты; без
 *     автоматического обновления пользователь ловил бы 401 посреди работы.
 *     Обновление выполняется в одном экземпляре: при параллельных запросах
 *     иначе улетело бы несколько refresh подряд, а ротация токенов сочла бы
 *     второй из них повторным использованием и погасила сессию целиком.
 *  3. Защита от подделки межсайтового запроса: значение cookie дублируется
 *     в заголовке для всех изменяющих методов.
 */

import { ApiError, NetworkError } from './errors';
import { accessTokenExpired, getSession, readCsrfCookie, setSession } from './session';
import type { AuthResponse } from './types';

const BASE_URL = '/api/v1';

/** Запрос не должен висеть вечно: без предела вкладка накапливает
 *  подвешенные соединения, а пользователь видит бесконечный индикатор. */
const DEFAULT_TIMEOUT_MS = 20_000;

export type HttpMethod = 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';

export type QueryValue = string | number | boolean | null | undefined;
export type QueryParams = Record<string, QueryValue | readonly QueryValue[]>;

export interface RequestOptions {
  method?: HttpMethod;
  query?: QueryParams;
  body?: unknown;
  signal?: AbortSignal;
  timeoutMs?: number;
  /** Служебный флаг: у повторной попытки после обновления токена он снят,
   *  иначе неудача обновления закрутила бы бесконечный цикл. */
  allowRefresh?: boolean;
}

const UNSAFE_METHODS = new Set<HttpMethod>(['POST', 'PUT', 'PATCH', 'DELETE']);

function buildUrl(path: string, query?: QueryParams): string {
  const url = BASE_URL + path;
  if (!query) return url;

  const search = new URLSearchParams();
  for (const [key, value] of Object.entries(query)) {
    if (value === undefined || value === null || value === '') continue;

    // Множественный выбор в фильтрах передаётся повторяющимся ключом
    // (?brand=toyota&brand=honda) — так его читает store.CarFilter.
    if (Array.isArray(value)) {
      for (const item of value) {
        if (item === undefined || item === null || item === '') continue;
        search.append(key, String(item));
      }
      continue;
    }
    search.append(key, String(value));
  }

  const qs = search.toString();
  return qs ? `${url}?${qs}` : url;
}

/** Слияние отмены от вызывающего кода с собственным таймаутом. */
function withTimeout(
  signal: AbortSignal | undefined,
  timeoutMs: number,
): { signal: AbortSignal; done: () => void } {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(new Error('timeout')), timeoutMs);

  const abort = () => controller.abort(signal?.reason);
  if (signal) {
    if (signal.aborted) abort();
    else signal.addEventListener('abort', abort, { once: true });
  }

  return {
    signal: controller.signal,
    done: () => {
      clearTimeout(timer);
      signal?.removeEventListener('abort', abort);
    },
  };
}

async function parseError(response: Response): Promise<ApiError> {
  const requestId = response.headers.get('X-Request-Id') ?? '';

  let code = 'internal_error';
  let message = 'Сервис недоступен. Попробуйте позже.';
  let details: Record<string, string> | undefined;

  try {
    const payload: unknown = await response.json();
    if (payload && typeof payload === 'object' && 'error' in payload) {
      const body = (payload as { error: Record<string, unknown> }).error;
      if (typeof body.code === 'string') code = body.code;
      if (typeof body.message === 'string') message = body.message;
      if (body.details && typeof body.details === 'object') {
        details = body.details as Record<string, string>;
      }
    }
  } catch {
    // Ответ без JSON — например, страница ошибки от nginx при перегрузке.
    // Код и текст остаются подстановочными.
  }

  return new ApiError({
    status: response.status,
    code,
    message,
    ...(details ? { details } : {}),
    requestId,
  });
}

// --- Обновление сессии ------------------------------------------------------

let refreshInFlight: Promise<boolean> | null = null;

/** Вызывается при окончательной потере сессии: провайдер аутентификации
 *  подписывается и уводит пользователя на вход. */
let onSessionLost: (() => void) | null = null;

export function setSessionLostHandler(handler: (() => void) | null): void {
  onSessionLost = handler;
}

/**
 * Обновляет пару токенов по HttpOnly-cookie.
 *
 * Возвращает признак успеха, а не бросает исключение: вызывающий код
 * различает «не вышло, но это нормально» (гость на публичной странице) и
 * реальную ошибку запроса.
 */
export async function refreshSession(): Promise<boolean> {
  if (refreshInFlight) return refreshInFlight;

  refreshInFlight = (async () => {
    try {
      const response = await fetch(`${BASE_URL}/auth/refresh`, {
        method: 'POST',
        credentials: 'include',
        headers: {
          'X-CSRF-Token': readCsrfCookie(),
          'X-Requested-With': 'XMLHttpRequest',
        },
      });

      if (!response.ok) {
        // Не гасим живую сессию на сбое refresh (CSRF/сеть/гонка):
        // иначе успешные действия вроде confirm почты заканчивались выходом.
        if (!getSession() || accessTokenExpired()) {
          setSession(null);
        }
        return false;
      }

      const data = (await response.json()) as AuthResponse;
      setSession({
        user: data.user,
        accessToken: data.access_token,
        accessExpiresAt: Date.parse(data.access_expires_at),
        csrfToken: data.csrf_token,
      });
      return true;
    } catch {
      // Сеть отвалилась: сессию не сбрасываем — токен в памяти может быть
      // ещё жив, а связь вернуться. Сбрасывает её только явный отказ сервера.
      return false;
    } finally {
      refreshInFlight = null;
    }
  })();

  return refreshInFlight;
}

// --- Основной вызов ---------------------------------------------------------

export async function request<T>(path: string, options: RequestOptions = {}): Promise<T> {
  const { method = 'GET', query, body, signal, timeoutMs = DEFAULT_TIMEOUT_MS } = options;
  const allowRefresh = options.allowRefresh ?? true;

  // Заведомо просроченный токен обновляем до отправки, а не после отказа:
  // так на один сетевой круг меньше и в логах сервера нет ложных 401.
  const session = getSession();
  if (allowRefresh && session && accessTokenExpired(session)) {
    await refreshSession();
  }

  const headers = new Headers({ Accept: 'application/json' });

  // Заголовок заставляет браузер считать запрос сложным и выполнять
  // предполётную проверку: форма с чужого сайта такой запрос не отправит.
  headers.set('X-Requested-With', 'XMLHttpRequest');

  const accessToken = getSession()?.accessToken;
  if (accessToken) headers.set('Authorization', `Bearer ${accessToken}`);

  if (UNSAFE_METHODS.has(method)) {
    // Cookie предпочтительна (double-submit); если Secure-cookie не сохранилась
    // на HTTP — берём значение из сессии после login/refresh.
    const csrf = readCsrfCookie() || getSession()?.csrfToken || '';
    if (csrf) headers.set('X-CSRF-Token', csrf);
  }

  let payload: BodyInit | undefined;
  if (body !== undefined) {
    if (body instanceof FormData) {
      // Content-Type для multipart ставит сам браузер: он должен добавить
      // границу частей, а вручную собранный заголовок её не содержит.
      payload = body;
    } else {
      headers.set('Content-Type', 'application/json');
      payload = JSON.stringify(body);
    }
  }

  const timeout = withTimeout(signal, timeoutMs);

  let response: Response;
  try {
    response = await fetch(buildUrl(path, query), {
      method,
      headers,
      credentials: 'include',
      // Ответы API кешировать нельзя: иначе список сделок останется
      // прежним после изменения этапа.
      cache: 'no-store',
      redirect: 'error',
      ...(payload !== undefined ? { body: payload } : {}),
      signal: timeout.signal,
    });
  } catch (error) {
    if (signal?.aborted) throw error;
    throw new NetworkError(error);
  } finally {
    timeout.done();
  }

  if (response.status === 401 && allowRefresh) {
    const refreshed = await refreshSession();
    if (refreshed) {
      return request<T>(path, { ...options, allowRefresh: false });
    }
    setSession(null);
    onSessionLost?.();
  }

  if (!response.ok) {
    throw await parseError(response);
  }

  if (response.status === 204 || response.headers.get('Content-Length') === '0') {
    return undefined as T;
  }
  return (await response.json()) as T;
}

export const api = {
  get: <T>(path: string, query?: QueryParams, signal?: AbortSignal) =>
    request<T>(path, { method: 'GET', ...(query ? { query } : {}), ...(signal ? { signal } : {}) }),

  post: <T>(path: string, body?: unknown, query?: QueryParams) =>
    request<T>(path, {
      method: 'POST',
      ...(body !== undefined ? { body } : {}),
      ...(query ? { query } : {}),
    }),

  put: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PUT', ...(body !== undefined ? { body } : {}) }),

  patch: <T>(path: string, body?: unknown) =>
    request<T>(path, { method: 'PATCH', ...(body !== undefined ? { body } : {}) }),

  delete: <T>(path: string) => request<T>(path, { method: 'DELETE' }),
};
