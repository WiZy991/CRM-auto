/**
 * Точки API, сгруппированные по разделам.
 *
 * Слой тонкий намеренно: он только фиксирует пути и типы ответов, чтобы
 * строковый адрес встречался в коде ровно один раз. Логика кеширования и
 * повторов живёт в запросах TanStack Query, а не здесь.
 */

import { api, request } from './client';
import { setSession } from './session';
import type {
  AdminOverview,
  AdminUserRow,
  AuditRow,
  AuthResponse,
  Banner,
  BannerForm,
  BannerPublic,
  BannerStats,
  CarDetail,
  CarDictionaries,
  CarForm,
  CarListItem,
  ClientRequest,
  CreateRequestInput,
  CursorPage,
  CurrentUser,
  Deal,
  DealClientMatch,
  DealDetails,
  DealDocument,
  DealListItem,
  DealMessage,
  DealReview,
  DealTask,
  DealerForm,
  DealerProfile,
  DictionaryEntry,
  Notification,
  NotificationPage,
  OffsetPage,
  PipelineSummary,
  PublicDealer,
  RequestListItem,
  Role,
  SecurityEventRow,
  Seller,
  SellerFacets,
  SellerForm,
  SessionInfo,
  SocialChannel,
  SocialChannelsResponse,
  Stage,
  StageMeta,
  UUID,
} from './types';

function storeAuth(data: AuthResponse): AuthResponse {
  setSession({
    user: data.user,
    accessToken: data.access_token,
    accessExpiresAt: Date.parse(data.access_expires_at),
    csrfToken: data.csrf_token,
  });
  return data;
}

export const authApi = {
  async login(login: string, password: string): Promise<AuthResponse> {
    return storeAuth(await api.post<AuthResponse>('/auth/login', { login, password }));
  },

  async register(input: {
    role: Exclude<Role, 'admin'>;
    email: string;
    phone: string;
    password: string;
    full_name: string;
  }): Promise<AuthResponse> {
    return storeAuth(await api.post<AuthResponse>('/auth/register', input));
  },

  /** Нужна ли капча. Ответ одинаков для существующего и несуществующего
   *  логина, поэтому запрос безопасен для показа до отправки формы. */
  precheck(login: string): Promise<{ captcha_required: boolean }> {
    return api.get<{ captcha_required: boolean }>('/auth/precheck', { login });
  },

  async logout(): Promise<void> {
    try {
      await api.post<void>('/auth/logout');
    } finally {
      // Сессия на клиенте гасится в любом случае: если сервер не ответил,
      // оставлять пользователя «наполовину вошедшим» хуже, чем разлогинить.
      setSession(null);
    }
  },

  async logoutEverywhere(): Promise<void> {
    try {
      await api.post<void>('/auth/logout-all');
    } finally {
      setSession(null);
    }
  },

  me(signal?: AbortSignal): Promise<{ user: CurrentUser }> {
    return request<{ user: CurrentUser }>('/auth/me', {
      method: 'GET',
      ...(signal ? { signal } : {}),
    });
  },

  sessions(): Promise<{ items: SessionInfo[] }> {
    return api.get<{ items: SessionInfo[] }>('/auth/sessions');
  },

  revokeSession(id: UUID): Promise<void> {
    return api.delete<void>(`/auth/sessions/${id}`);
  },

  sendCode(channel: 'email' | 'phone'): Promise<{ status: string; channel: string }> {
    return api.post<{ status: string; channel: string }>('/auth/verify/resend', { channel });
  },

  verify(
    channel: 'email' | 'phone',
    code: string,
  ): Promise<{ user: CurrentUser; refresh_needed: boolean }> {
    return api.post<{ user: CurrentUser; refresh_needed: boolean }>('/auth/verify', {
      channel,
      code,
    });
  },

  changePassword(oldPassword: string, newPassword: string): Promise<{ status: string }> {
    return api.post<{ status: string }>('/auth/password', {
      old_password: oldPassword,
      new_password: newPassword,
    });
  },

  updateProfile(input: {
    full_name?: string;
    passport?: string;
    address?: string;
  }): Promise<{ user: CurrentUser }> {
    return api.patch<{ user: CurrentUser }>('/auth/profile', input);
  },

  deleteAccount(password: string): Promise<void> {
    return api.post<void>('/auth/delete', { password });
  },
};

export interface CatalogQuery {
  origin?: readonly string[];
  brand?: readonly string[];
  model?: readonly string[];
  body?: readonly string[];
  fuel?: readonly string[];
  gearbox?: readonly string[];
  drive?: readonly string[];

  year_from?: number;
  year_to?: number;
  mileage_to?: number;
  /** Границы цены передаются в рублях: сервер сам переводит в копейки. */
  price_from?: number;
  price_to?: number;
  engine_from?: number;
  engine_to?: number;
  power_from?: number;

  steering_right?: boolean;
  q?: string;
  sort?: string;

  limit?: number;
  cursor?: string;
  status?: readonly string[];
  favorites?: boolean;
}

export const carsApi = {
  list(query: CatalogQuery, signal?: AbortSignal): Promise<CursorPage<CarListItem>> {
    return api.get<CursorPage<CarListItem>>('/cars', { ...query }, signal);
  },

  mine(query: CatalogQuery, signal?: AbortSignal): Promise<CursorPage<CarListItem>> {
    return api.get<CursorPage<CarListItem>>('/cars/my', { ...query }, signal);
  },

  get(id: UUID, signal?: AbortSignal): Promise<{ car: CarDetail }> {
    return api.get<{ car: CarDetail }>(`/cars/${id}`, undefined, signal);
  },

  dictionaries(origin?: readonly string[], signal?: AbortSignal): Promise<CarDictionaries> {
    return api.get<CarDictionaries>('/cars/dictionaries', origin ? { origin } : undefined, signal);
  },

  setFavorite(id: UUID, favorite: boolean): Promise<{ is_favorite: boolean }> {
    return favorite
      ? api.put<{ is_favorite: boolean }>(`/cars/${id}/favorite`)
      : api.delete<{ is_favorite: boolean }>(`/cars/${id}/favorite`);
  },

  changeStatus(id: UUID, status: string): Promise<{ status: string; title: string }> {
    return api.patch<{ status: string; title: string }>(`/cars/${id}/status`, { status });
  },

  create(input: CarForm): Promise<{ car: CarDetail }> {
    return api.post<{ car: CarDetail }>('/cars', input);
  },

  update(id: UUID, input: CarForm): Promise<{ car: CarDetail }> {
    return api.put<{ car: CarDetail }>(`/cars/${id}`, input);
  },

  remove(id: UUID): Promise<void> {
    return api.delete<void>(`/cars/${id}`);
  },

  publishSocial(id: UUID): Promise<{ queued: boolean }> {
    return api.post<{ queued: boolean }>(`/cars/${id}/publish-social`);
  },
};

export const uploadsApi = {
  image(file: File): Promise<{ url: string; width: number; height: number }> {
    const body = new FormData();
    body.append('file', file);
    return request('/uploads/images', { method: 'POST', body, timeoutMs: 60_000 });
  },
};

export const dealsApi = {
  stages(signal?: AbortSignal): Promise<{ items: StageMeta[] }> {
    return api.get<{ items: StageMeta[] }>('/meta/stages', undefined, signal);
  },

  list(
    query: {
      stage?: Stage;
      outcome?: string;
      q?: string;
      stale?: boolean;
      limit?: number;
      offset?: number;
    },
    signal?: AbortSignal,
  ): Promise<OffsetPage<DealListItem>> {
    return api.get<OffsetPage<DealListItem>>('/deals', { ...query }, signal);
  },

  summary(signal?: AbortSignal): Promise<{ summary: PipelineSummary }> {
    return api.get<{ summary: PipelineSummary }>('/deals/summary', undefined, signal);
  },

  get(id: UUID, signal?: AbortSignal): Promise<DealDetails> {
    return api.get<DealDetails>(`/deals/${id}`, undefined, signal);
  },

  create(input: {
    request_id?: string;
    client_id?: string;
    car_id?: string;
    seller_id?: string;
    client_name?: string;
    client_email?: string;
    client_phone?: string;
    title?: string;
    amount_minor?: number;
    currency?: string;
    stage?: Stage;
  }): Promise<{ deal: Deal }> {
    return api.post<{ deal: Deal }>('/deals', input);
  },

  searchClients(query = '', signal?: AbortSignal): Promise<{ items: DealClientMatch[] }> {
    return api.get<{ items: DealClientMatch[] }>(
      '/deals/clients',
      query ? { q: query } : undefined,
      signal,
    );
  },

  messages(id: UUID, signal?: AbortSignal): Promise<{ items: DealMessage[] }> {
    return api.get<{ items: DealMessage[] }>(`/deals/${id}/messages`, undefined, signal);
  },

  sendMessage(id: UUID, body: string): Promise<{ message: DealMessage }> {
    return api.post<{ message: DealMessage }>(`/deals/${id}/messages`, { body });
  },

  changeStage(id: UUID, stage: Stage, comment: string): Promise<{ deal: Deal }> {
    return api.post<{ deal: Deal }>(`/deals/${id}/stage`, { stage, comment });
  },

  update(
    id: UUID,
    input: {
      title?: string;
      amount_minor?: number;
      currency?: string;
      paid_rub_minor?: number;
      expected_handover_at?: string;
      manager_note?: string;
    },
  ): Promise<{ deal: Deal }> {
    return api.patch<{ deal: Deal }>(`/deals/${id}`, input);
  },

  close(id: UUID, outcome: 'won' | 'lost', reason: string): Promise<{ deal: Deal }> {
    return api.post<{ deal: Deal }>(`/deals/${id}/close`, { outcome, reason });
  },

  addDocument(
    id: UUID,
    input: {
      kind: string;
      title: string;
      file_url: string;
      mime_type?: string;
      bytes?: number;
      visible_to_client?: boolean;
    },
  ): Promise<{ document: DealDocument }> {
    return api.post<{ document: DealDocument }>(`/deals/${id}/documents`, input);
  },

  generateDocuments(
    id: UUID,
    input: { kind?: string; kinds?: string[]; visible_to_client?: boolean } = {},
  ): Promise<{ documents: DealDocument[]; warnings: string[] }> {
    return api.post<{ documents: DealDocument[]; warnings: string[] }>(
      `/deals/${id}/documents/generate`,
      input,
    );
  },

  createTask(
    id: UUID,
    input: { title: string; description?: string; stage?: Stage; due_at?: string },
  ): Promise<{ task: DealTask }> {
    return api.post<{ task: DealTask }>(`/deals/${id}/tasks`, input);
  },

  toggleTask(id: UUID, taskId: UUID, done: boolean): Promise<{ task: DealTask }> {
    return api.post<{ task: DealTask }>(`/deals/${id}/tasks/${taskId}/toggle`, { done });
  },

  leaveReview(id: UUID, rating: number, text: string): Promise<{ review: DealReview }> {
    return api.post<{ review: DealReview }>(`/deals/${id}/review`, { rating, text });
  },
};

export const requestsApi = {
  mine(
    query: { status?: readonly string[]; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<RequestListItem>> {
    return api.get<OffsetPage<RequestListItem>>('/requests/my', { ...query }, signal);
  },

  dealer(
    query: { status?: readonly string[]; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<RequestListItem>> {
    return api.get<OffsetPage<RequestListItem>>('/dealer/requests', { ...query }, signal);
  },

  pool(
    query: { limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<RequestListItem>> {
    return api.get<OffsetPage<RequestListItem>>('/dealer/requests/pool', { ...query }, signal);
  },

  get(id: UUID, signal?: AbortSignal): Promise<{ request: ClientRequest }> {
    return api.get<{ request: ClientRequest }>(`/requests/${id}`, undefined, signal);
  },

  create(input: CreateRequestInput): Promise<{ request: ClientRequest }> {
    return api.post<{ request: ClientRequest }>('/requests', input);
  },

  close(id: UUID): Promise<{ status: string }> {
    return api.post<{ status: string }>(`/requests/${id}/close`);
  },

  claim(id: UUID): Promise<{ request: ClientRequest }> {
    return api.post<{ request: ClientRequest }>(`/dealer/requests/${id}/claim`);
  },

  reply(id: UUID, reply: string): Promise<{ request: ClientRequest }> {
    return api.post<{ request: ClientRequest }>(`/dealer/requests/${id}/reply`, { reply });
  },

  reject(id: UUID, reason: string): Promise<{ request: ClientRequest }> {
    return api.post<{ request: ClientRequest }>(`/dealer/requests/${id}/reject`, { reason });
  },
};

export const sellersApi = {
  list(
    query: {
      country?: readonly string[];
      kind?: readonly string[];
      region?: readonly string[];
      brand?: readonly string[];
      q?: string;
      sort?: string;
      verified?: boolean;
      limit?: number;
      offset?: number;
    } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<Seller>> {
    return api.get<OffsetPage<Seller>>('/sellers', { ...query }, signal);
  },

  directory(
    query: {
      country?: readonly string[];
      kind?: readonly string[];
      region?: readonly string[];
      brand?: readonly string[];
      q?: string;
      limit?: number;
      offset?: number;
    } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<Seller>> {
    return api.get<OffsetPage<Seller>>('/directory/sellers', { ...query }, signal);
  },

  directoryGet(id: UUID, signal?: AbortSignal): Promise<{ seller: Seller }> {
    return api.get<{ seller: Seller }>(`/directory/sellers/${id}`, undefined, signal);
  },

  directoryFacets(country?: readonly string[], signal?: AbortSignal): Promise<SellerFacets> {
    return api.get<SellerFacets>(
      '/directory/sellers/facets',
      country ? { country } : undefined,
      signal,
    );
  },

  facets(country?: readonly string[], signal?: AbortSignal): Promise<SellerFacets> {
    return api.get<SellerFacets>('/sellers/facets', country ? { country } : undefined, signal);
  },

  mine(signal?: AbortSignal): Promise<{ items: Seller[] }> {
    return api.get<{ items: Seller[] }>('/sellers/my', undefined, signal);
  },

  get(id: UUID, signal?: AbortSignal): Promise<{ seller: Seller }> {
    return api.get<{ seller: Seller }>(`/sellers/${id}`, undefined, signal);
  },

  create(input: SellerForm): Promise<{ seller: Seller }> {
    return api.post<{ seller: Seller }>('/sellers', input);
  },

  update(id: UUID, input: SellerForm): Promise<{ seller: Seller }> {
    return api.put<{ seller: Seller }>(`/sellers/${id}`, input);
  },

  setActive(id: UUID, active: boolean): Promise<{ active: boolean }> {
    return api.patch<{ active: boolean }>(`/sellers/${id}/active`, { active });
  },

  saveNote(id: UUID, note: string, isTrusted: boolean): Promise<{ seller: Seller }> {
    return api.put<{ seller: Seller }>(`/sellers/${id}/note`, { note, is_trusted: isTrusted });
  },

  deleteNote(id: UUID): Promise<void> {
    return api.delete<void>(`/sellers/${id}/note`);
  },
};

export const bannersApi = {
  active(
    placement: string,
    limit = 3,
    signal?: AbortSignal,
  ): Promise<{ items: BannerPublic[] }> {
    return api.get<{ items: BannerPublic[] }>('/banners', { placement, limit }, signal);
  },

  mine(signal?: AbortSignal): Promise<{
    items: Banner[];
    stats: BannerStats;
    dictionaries: Record<string, DictionaryEntry[]>;
  }> {
    return api.get<{
      items: Banner[];
      stats: BannerStats;
      dictionaries: Record<string, DictionaryEntry[]>;
    }>('/dealer/banners', undefined, signal);
  },

  create(input: BannerForm): Promise<{ banner: Banner }> {
    return api.post<{ banner: Banner }>('/dealer/banners', input);
  },

  update(id: UUID, input: BannerForm): Promise<{ banner: Banner }> {
    return api.put<{ banner: Banner }>(`/dealer/banners/${id}`, input);
  },

  submit(id: UUID): Promise<{ banner: Banner }> {
    return api.post<{ banner: Banner }>(`/dealer/banners/${id}/submit`);
  },

  setPaused(id: UUID, paused: boolean): Promise<{ banner: Banner }> {
    return api.patch<{ banner: Banner }>(`/dealer/banners/${id}/pause`, { paused });
  },

  remove(id: UUID): Promise<void> {
    return api.delete<void>(`/dealer/banners/${id}`);
  },

  pending(signal?: AbortSignal): Promise<{ items: Banner[] }> {
    return api.get<{ items: Banner[] }>('/admin/banners', undefined, signal);
  },

  moderate(id: UUID, approve: boolean, reason = ''): Promise<{ banner: Banner }> {
    return api.post<{ banner: Banner }>(`/admin/banners/${id}/moderate`, { approve, reason });
  },
};

export const adminApi = {
  overview(signal?: AbortSignal): Promise<{ overview: AdminOverview }> {
    return api.get<{ overview: AdminOverview }>('/admin/overview', undefined, signal);
  },

  users(
    query: {
      role?: readonly string[];
      status?: readonly string[];
      q?: string;
      unverified?: boolean;
      limit?: number;
      offset?: number;
    } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<AdminUserRow>> {
    return api.get<OffsetPage<AdminUserRow>>('/admin/users', { ...query }, signal);
  },

  setUserStatus(id: UUID, status: string): Promise<{ status: string }> {
    return api.patch<{ status: string }>(`/admin/users/${id}/status`, { status });
  },

  setUserRole(id: UUID, role: string): Promise<{ role: string }> {
    return api.patch<{ role: string }>(`/admin/users/${id}/role`, { role });
  },

  audit(
    query: { entity?: string; action?: string; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<{ items: AuditRow[] }> {
    return api.get<{ items: AuditRow[] }>('/admin/audit', { ...query }, signal);
  },

  securityEvents(
    query: { kind?: readonly string[]; min_severity?: number; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<{ items: SecurityEventRow[] }> {
    return api.get<{ items: SecurityEventRow[] }>('/admin/security-events', { ...query }, signal);
  },

  verifySeller(id: UUID, verified: boolean): Promise<{ verified: boolean }> {
    return api.patch<{ verified: boolean }>(`/admin/sellers/${id}/verify`, { verified });
  },

  cars(
    query: { status?: readonly string[]; q?: string; limit?: number } = {},
    signal?: AbortSignal,
  ): Promise<CursorPage<CarListItem>> {
    return api.get<CursorPage<CarListItem>>('/admin/cars', { ...query }, signal);
  },

  moderateCar(id: UUID, approve: boolean, reason = ''): Promise<{ status: string }> {
    return api.post<{ status: string }>(`/admin/cars/${id}/moderate`, { approve, reason });
  },
};

export const dealersApi = {
  list(
    query: { city?: string; limit?: number; offset?: number } = {},
    signal?: AbortSignal,
  ): Promise<OffsetPage<PublicDealer>> {
    return api.get<OffsetPage<PublicDealer>>('/dealers', { ...query }, signal);
  },

  get(slug: string, signal?: AbortSignal): Promise<{ dealer: PublicDealer }> {
    return api.get<{ dealer: PublicDealer }>(`/dealers/${slug}`, undefined, signal);
  },

  cars(slug: string, signal?: AbortSignal): Promise<CursorPage<CarListItem>> {
    return api.get<CursorPage<CarListItem>>(`/dealers/${slug}/cars`, undefined, signal);
  },

  mine(signal?: AbortSignal): Promise<{ dealer: DealerProfile }> {
    return api.get<{ dealer: DealerProfile }>('/dealers/me', undefined, signal);
  },

  saveMine(input: DealerForm): Promise<{ dealer: DealerProfile }> {
    return api.put<{ dealer: DealerProfile }>('/dealers/me', input);
  },

  reviews(slug: string, signal?: AbortSignal): Promise<{ items: DealReview[] }> {
    return api.get<{ items: DealReview[] }>(`/dealers/${slug}/reviews`, undefined, signal);
  },

  mineReviews(signal?: AbortSignal): Promise<{ items: DealReview[] }> {
    return api.get<{ items: DealReview[] }>('/dealer/reviews', undefined, signal);
  },

  replyReview(id: UUID, reply: string): Promise<{ review: DealReview }> {
    return api.post<{ review: DealReview }>(`/dealer/reviews/${id}/reply`, { reply });
  },
};

export const notificationsApi = {
  list(
    params: { unread?: boolean; limit?: number; before_id?: number },
    signal?: AbortSignal,
  ): Promise<NotificationPage> {
    return api.get<NotificationPage>('/notifications', { ...params }, signal);
  },

  unreadCount(signal?: AbortSignal): Promise<{ unread: number }> {
    return api.get<{ unread: number }>('/notifications/unread', undefined, signal);
  },

  markRead(ids: number[]): Promise<{ unread: number }> {
    return api.post<{ unread: number }>('/notifications/read', { ids, all: false });
  },

  markAllRead(): Promise<{ unread: number }> {
    return api.post<{ unread: number }>('/notifications/read', { ids: [], all: true });
  },
};

export const channelsApi = {
  list(signal?: AbortSignal): Promise<SocialChannelsResponse> {
    return api.get<SocialChannelsResponse>('/dealer/channels', undefined, signal);
  },

  save(input: {
    network: string;
    token?: string;
    api_key?: string;
    chat_id?: string;
    owner_id?: string;
    phone_number_id?: string;
    business_account_id?: string;
    destination?: string;
    auto_post?: boolean;
    disconnect?: boolean;
  }): Promise<{ channel: SocialChannel }> {
    return api.put<{ channel: SocialChannel }>('/dealer/channels', input);
  },

  test(network: string): Promise<{ channel: SocialChannel; ok: boolean }> {
    return api.post<{ channel: SocialChannel; ok: boolean }>(`/dealer/channels/${network}/test`);
  },

  oauthStart(network: string): Promise<{ url: string }> {
    return api.get<{ url: string }>(`/dealer/channels/${network}/oauth/start`);
  },
};

export type { Notification };
