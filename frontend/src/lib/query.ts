import { QueryClient } from '@tanstack/react-query';

import { ApiError } from './api';

/**
 * Настройки кеша запросов.
 *
 * Значения по умолчанию у библиотеки рассчитаны на публичные ленты с частым
 * обновлением. Здесь другая нагрузка: справочники и каталог меняются редко,
 * а рабочие списки — по действию пользователя, а не сами по себе. Поэтому
 * фоновые перезапросы приглушены, а свежесть обеспечивается точечным сбросом
 * после мутаций.
 */
export function createQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: {
        staleTime: 60_000,
        gcTime: 10 * 60_000,

        // Возврат на вкладку не должен перезапрашивать всё подряд: у
        // менеджера открыто несколько вкладок, и переключение между ними
        // создавало бы волну запросов на ровном месте.
        refetchOnWindowFocus: false,
        refetchOnReconnect: true,

        retry: (failureCount, error) => {
          // Ошибку прав, валидации или отсутствия объекта повтор не
          // исправит — он только умножает нагрузку и приближает 429.
          if (error instanceof ApiError && !error.isRetryable) return false;
          return failureCount < 2;
        },
        retryDelay: (attempt) => Math.min(1000 * 2 ** attempt, 8000),
      },

      mutations: {
        // Изменяющие запросы не повторяются автоматически: при обрыве связи
        // неизвестно, дошёл ли первый, и повтор может создать вторую заявку.
        retry: false,
      },
    },
  });
}

/**
 * Ключи запросов в одном месте.
 *
 * Разложенные по файлам массивы-ключи расходятся с теми, что используются
 * при сбросе кеша, и мутация перестаёт обновлять список — молча, без ошибки.
 */
export const queryKeys = {
  currentUser: ['auth', 'me'] as const,
  sessions: ['auth', 'sessions'] as const,

  catalog: (filters: unknown) => ['cars', 'list', filters] as const,
  myCars: (filters: unknown) => ['cars', 'my', filters] as const,
  car: (id: string) => ['cars', 'item', id] as const,
  carDictionaries: (origin?: readonly string[]) => ['cars', 'dictionaries', origin ?? []] as const,

  stages: ['meta', 'stages'] as const,
  deals: (filters: unknown) => ['deals', 'list', filters] as const,
  deal: (id: string) => ['deals', 'item', id] as const,
  dealMessages: (id: string) => ['deals', 'messages', id] as const,

  notifications: (unreadOnly: boolean) => ['notifications', 'list', unreadOnly] as const,
  unreadCount: ['notifications', 'unread'] as const,

  requestsMine: (filters: unknown) => ['requests', 'mine', filters] as const,
  requestsDealer: (filters: unknown) => ['requests', 'dealer', filters] as const,
  requestsPool: (filters: unknown) => ['requests', 'pool', filters] as const,

  dealSummary: ['deals', 'summary'] as const,
  dealClients: (query: string) => ['deals', 'clients', query] as const,

  sellers: (filters: unknown) => ['sellers', 'list', filters] as const,
  seller: (id: string) => ['sellers', 'item', id] as const,
  sellerDirectory: (filters: unknown) => ['sellers', 'directory', filters] as const,
  sellerPublic: (id: string) => ['sellers', 'public', id] as const,
  sellerMine: ['sellers', 'mine'] as const,
  sellerFacets: (country?: readonly string[]) => ['sellers', 'facets', country ?? []] as const,

  bannersActive: (placement: string) => ['banners', 'active', placement] as const,
  bannersMine: ['banners', 'mine'] as const,
  bannersPending: ['banners', 'pending'] as const,

  adminOverview: ['admin', 'overview'] as const,
  adminUsers: (filters: unknown) => ['admin', 'users', filters] as const,
  adminAudit: (filters: unknown) => ['admin', 'audit', filters] as const,
  adminSecurity: (filters: unknown) => ['admin', 'security', filters] as const,
  adminCars: (filters: unknown) => ['admin', 'cars', filters] as const,

  dealers: (filters: unknown) => ['dealers', 'list', filters] as const,
  dealer: (slug: string) => ['dealers', 'item', slug] as const,
  dealerCars: (slug: string) => ['dealers', 'cars', slug] as const,
  dealerReviews: (slug: string) => ['dealers', 'reviews', slug] as const,
  dealerMine: ['dealers', 'mine'] as const,
  dealerMineReviews: ['dealers', 'reviews', 'mine'] as const,
  dealerChannels: ['dealer', 'channels'] as const,
} as const;
