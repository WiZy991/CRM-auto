/**
 * Типы ответов API.
 *
 * Зеркалят структуры из internal/httpx: имена полей совпадают с тегами json,
 * необязательные поля отмечены `?` там, где на сервере стоит `omitempty`.
 * Схема ответа не проверяется во время работы для каждого запроса — сервер
 * свой, и лишний разбор на каждой карточке каталога стоит дороже пользы;
 * проверяются только данные сессии, от которых зависят права в интерфейсе.
 */

export type UUID = string;
/** Момент времени в RFC 3339, как его отдаёт encoding/json. */
export type Timestamp = string;

export type Role = 'client' | 'dealer' | 'seller' | 'admin';
export type UserStatus = 'pending' | 'active' | 'suspended' | 'deleted';

export interface CurrentUser {
  id: UUID;
  role: Role;
  role_title: string;
  status: UserStatus;
  email: string;
  phone: string;
  full_name: string;
  avatar_url?: string;
  email_verified: boolean;
  phone_verified: boolean;
  passport?: string;
  address?: string;
  created_at: Timestamp;
}

export interface AuthResponse {
  user: CurrentUser;
  access_token: string;
  access_expires_at: Timestamp;
  csrf_token: string;
}

export interface SessionInfo {
  id: UUID;
  device: string;
  ip?: string;
  current: boolean;
  created_at: Timestamp;
  last_used_at?: Timestamp;
  expires_at: Timestamp;
}

// --- Каталог ----------------------------------------------------------------

export type Origin = 'cn' | 'jp';
export type CarStatus = 'draft' | 'moderation' | 'active' | 'reserved' | 'sold' | 'archived';
export type CatalogSort = 'fresh' | 'price_asc' | 'price_desc' | 'year_desc' | 'mileage_asc';

export interface CarListItem {
  id: UUID;
  status: CarStatus;
  origin: Origin;

  brand: string;
  model: string;
  generation?: string;
  year: number;
  mileage_km: number;

  fuel: string;
  gearbox: string;
  drive: string;
  body: string;

  engine_cc?: number;
  power_hp?: number;

  steering_right: boolean;
  auction_grade?: string;

  price_minor: number;
  currency: string;
  price_rub_minor: number;
  /** Готовая подпись с сервера: правила разрядов и валюты живут в одном месте. */
  price_label: string;
  turnkey_label?: string;

  title: string;
  cover_url?: string;
  photo_count: number;
  /** Первые фото для точек на карточке каталога, без захода в лот. */
  photo_urls?: string[];

  is_favorite: boolean;
  published_at?: Timestamp;
}

export interface CarPhoto {
  id: UUID;
  url: string;
  thumb_url?: string;
  width?: number;
  height?: number;
  sort_order: number;
}

export interface CarDetail extends CarListItem {
  trim_level?: string;
  color?: string;
  seats?: number;

  interior_grade?: string;
  auction_lot_number?: string;
  auction_date?: Timestamp;

  vin?: string;

  customs_rub_minor?: number;
  delivery_days?: number;

  description: string;
  equipment: string[];
  photos: CarPhoto[];

  views_count: number;
  requests_count: number;
  can_edit: boolean;

  display_name: string;
}

export interface DictionaryEntry {
  value: string;
  title: string;
}

export interface CarDictionaries {
  dictionaries: Record<string, DictionaryEntry[]>;
  brands: DictionaryEntry[];
  sorts: DictionaryEntry[];
}

/** Курсорная страница. Общее число приходит только для первой страницы:
 *  считать его на каждой прокрутке — лишняя нагрузка на базу. */
export interface CursorPage<T> {
  items: T[];
  next_cursor: string;
  total: number;
}

// --- Воронка ----------------------------------------------------------------

export type Stage =
  | 'lead'
  | 'needs'
  | 'contract'
  | 'payment'
  | 'shipping'
  | 'customs'
  | 'handover';

export type DealOutcome = 'open' | 'won' | 'lost';

export interface StageMeta {
  stage: Stage;
  position: number;
  title: string;
  description: string;
  client_hint: string;
  normative_days: number;
}

export interface DealClientMatch {
  id: UUID;
  full_name: string;
  email: string;
  phone: string;
}

export interface Deal {
  id: UUID;
  number: number;

  client_id: UUID;
  dealer_id: UUID;
  car_id?: UUID;
  seller_id?: UUID;

  stage: Stage;
  stage_title: string;
  stage_position: number;
  outcome: DealOutcome;

  title: string;
  amount_minor?: number;
  currency: string;
  amount_rub_minor?: number;
  amount_label?: string;
  paid_rub_minor: number;
  paid_share: number;

  services_note?: string;
  destination_port?: string;
  shipping_tracking?: string;
  customs_duties_rub_minor?: number;
  sbkts_number?: string;
  sbkts_issued_at?: Timestamp;
  first_contacted_at?: Timestamp;
  contract_signed_at?: Timestamp;
  paid_at?: Timestamp;
  shipped_at?: Timestamp;
  arrived_at?: Timestamp;
  customs_cleared_at?: Timestamp;
  handed_over_at?: Timestamp;

  stage_changed_at: Timestamp;
  days_on_stage: number;
  is_stale: boolean;
  expected_handover_at?: Timestamp;
  client_hint?: string;
  normative_days?: number;

  lost_reason?: string;
  manager_note?: string;

  closed_at?: Timestamp;
  created_at: Timestamp;
  updated_at: Timestamp;
}

export interface DealListItem extends Deal {
  client_name: string;
  dealer_name: string;
  car_title?: string;
  open_tasks: number;
  unread_messages: number;
}

export interface DealMessage {
  id: number;
  author_id: UUID;
  author_name: string;
  author_role: Role | '';
  body: string;
  is_system: boolean;
  attachment_url?: string;
  read_at?: Timestamp;
  created_at: Timestamp;
}

export interface DealTask {
  id: UUID;
  title: string;
  description?: string;
  stage?: Stage;
  assignee_name?: string;
  due_at?: Timestamp;
  done_at?: Timestamp;
  overdue: boolean;
  created_at: Timestamp;
}

// --- Уведомления ------------------------------------------------------------

export interface Notification {
  id: number;
  kind: string;
  title: string;
  body?: string;
  link?: string;
  payload?: Record<string, unknown>;
  read_at?: Timestamp;
  created_at: Timestamp;
}

export interface NotificationPage {
  items: Notification[];
  unread: number;
}

/** Страница со смещением. Для коротких списков кабинета, где нужна
 *  произвольная страница, а не бесконечная лента. */
export interface OffsetPage<T> {
  items: T[];
  total: number;
}

export interface StageHistoryEntry {
  from_stage?: Stage;
  to_stage: Stage;
  stage_title: string;
  outcome: DealOutcome;
  changed_by: string;
  comment?: string;
  duration_days?: number;
  created_at: Timestamp;
}

export interface DealDocument {
  id: UUID;
  kind: string;
  title: string;
  url: string;
  mime_type?: string;
  bytes: number;
  size_label: string;
  uploader_name?: string;
  visible_to_client: boolean;
  created_at: Timestamp;
}

export interface DealReview {
  id: UUID;
  rating: number;
  text: string;
  author_name?: string;
  dealer_reply?: string;
  replied_at?: Timestamp;
  created_at: Timestamp;
}

export interface DealDetails {
  deal: Deal;
  history: StageHistoryEntry[];
  tasks: DealTask[];
  documents: DealDocument[];
  next_stages: DictionaryEntry[];
  can_manage: boolean;
  can_review: boolean;
  review?: DealReview | null;
}

export interface StageStat {
  stage: Stage;
  title: string;
  count: number;
  amount_rub_minor: number;
  stale_count: number;
  avg_days_on_stage: number;
}

export interface PipelineSummary {
  stages: StageStat[];
  open_count: number;
  won_count: number;
  lost_count: number;
  stale_count: number;
  open_amount_rub_minor: number;
  won_amount_rub_minor: number;
  lost_amount_rub_minor: number;
  conversion: number;
  avg_cycle_days: number;
  revenue_by_month: MonthRevenue[];
  created_by_month: MonthCount[];
  by_origin: OriginStat[];
  requests_by_status: CountBucket[];
  listings_by_status: CountBucket[];
  ads: PipelineAdStats;
}

export interface MonthRevenue {
  month: string;
  amount_rub_minor: number;
  deals_count: number;
}

export interface MonthCount {
  month: string;
  count: number;
}

export interface OriginStat {
  origin: string;
  title: string;
  open: number;
  won: number;
  lost: number;
  open_amount_rub_minor: number;
  won_amount_rub_minor: number;
}

export interface CountBucket {
  key: string;
  title: string;
  count: number;
}

export interface PipelineAdStats {
  count: number;
  impressions: number;
  clicks: number;
  ctr: number;
}

// --- Заявки -----------------------------------------------------------------

export type RequestStatus =
  | 'new'
  | 'in_progress'
  | 'answered'
  | 'converted'
  | 'rejected'
  | 'closed';

export interface ClientRequest {
  id: UUID;
  number: number;
  status: RequestStatus;
  status_title: string;
  summary: string;

  dealer_id?: UUID;
  car_id?: UUID;

  desired_brand?: string;
  desired_model?: string;
  year_from?: number;
  year_to?: number;
  origin?: Origin;
  body?: string;
  gearbox?: string;

  budget_from_rub_minor?: number;
  budget_to_rub_minor?: number;
  budget_label?: string;

  comment?: string;
  contact_preference: string;

  dealer_reply?: string;
  replied_at?: Timestamp;
  rejected_reason?: string;

  created_at: Timestamp;
  updated_at: Timestamp;
}

export interface RequestListItem extends ClientRequest {
  client_name?: string;
  client_phone?: string;
  dealer_name?: string;
  car_title?: string;
  has_deal: boolean;
}

export interface CreateRequestInput {
  dealer_id?: string;
  car_id?: string;
  desired_brand?: string;
  desired_model?: string;
  year_from?: number;
  year_to?: number;
  origin?: string;
  budget_from_rub?: number;
  budget_to_rub?: number;
  body?: string;
  gearbox?: string;
  comment?: string;
  contact_preference?: string;
}

// --- Поставщики -------------------------------------------------------------

export type SellerKind = 'auction' | 'exporter' | 'dealership' | 'factory' | 'broker';

export interface SellerBrandFacet {
  brand: string;
  count: number;
}

export interface SellerFacets {
  brands?: SellerBrandFacet[];
  regions?: { country: string; region: string; count: number }[];
  dictionaries?: Record<string, DictionaryEntry[]>;
  sorts?: DictionaryEntry[];
}

export interface Seller {
  id: UUID;
  country: Origin;
  country_title: string;
  kind: SellerKind;
  kind_title: string;

  name: string;
  name_local?: string;
  display_name: string;
  region: string;
  city?: string;
  address?: string;

  brands: string[];
  description?: string;
  website?: string;
  logo_url?: string;
  contacts: Record<string, string>;

  min_order_qty: number;
  export_experience_years?: number;

  rating_avg: number;
  rating_count: number;

  is_verified: boolean;
  verified_at?: Timestamp;
  is_active: boolean;
  can_edit: boolean;

  note?: string;
  is_trusted?: boolean;

  created_at: Timestamp;
}

export interface SellerForm {
  country: string;
  kind: string;
  name: string;
  name_local?: string;
  region: string;
  city?: string;
  address?: string;
  brands?: string[];
  description?: string;
  website?: string;
  contacts?: Record<string, string>;
  logo_url?: string;
  min_order_qty?: number;
  export_experience_years?: number;
}

// --- Баннеры ----------------------------------------------------------------

export type BannerPlacement =
  | 'home_hero'
  | 'home_inline'
  | 'catalog_top'
  | 'catalog_sidebar'
  | 'car_page';

export type BannerStatus =
  | 'draft'
  | 'moderation'
  | 'active'
  | 'paused'
  | 'rejected'
  | 'expired';

export interface BannerPublic {
  id: UUID;
  title: string;
  subtitle?: string;
  image_url: string;
  image_mobile_url?: string;
  cta_label: string;
  href: string;
}

export interface Banner {
  id: UUID;
  dealer_id: UUID;
  placement: BannerPlacement;
  placement_title: string;
  status: BannerStatus;
  status_title: string;

  title: string;
  subtitle?: string;
  image_url: string;
  image_mobile_url?: string;
  target_url: string;
  cta_label: string;

  starts_at: Timestamp;
  ends_at: Timestamp;
  weight: number;

  impressions: number;
  clicks: number;
  ctr: number;

  reject_reason?: string;
  is_showable: boolean;

  created_at: Timestamp;
  updated_at: Timestamp;
}

export interface BannerStats {
  total: number;
  active: number;
  impressions: number;
  clicks: number;
  ctr: number;
}

export interface BannerForm {
  placement: string;
  title: string;
  subtitle?: string;
  image_url: string;
  image_mobile_url?: string;
  target_url: string;
  cta_label?: string;
  starts_at: string;
  ends_at: string;
  weight?: number;
}

// --- Админка ----------------------------------------------------------------

export interface AdminOverview {
  users_total: number;
  users_new_7d: number;
  dealers_total: number;
  cars_active: number;
  cars_moderation: number;
  requests_open: number;
  deals_open: number;
  deals_won_30d: number;
  banners_moderation: number;
  sellers_pending: number;
  security_events_24h: number;
  blocked_ips: number;
}

export interface AdminUserRow {
  id: UUID;
  email: string;
  phone: string;
  full_name: string;
  role: Role;
  status: UserStatus;
  email_verified: boolean;
  phone_verified: boolean;
  deals_count: number;
  requests_count: number;
  cars_count: number;
  last_login_at?: Timestamp;
  created_at: Timestamp;
}

export interface AuditRow {
  id: number;
  actor_id?: UUID;
  actor_name?: string;
  actor_role?: string;
  action: string;
  entity: string;
  entity_id?: string;
  diff?: Record<string, unknown>;
  ip?: string;
  request_id?: string;
  created_at: Timestamp;
}

export interface SecurityEventRow {
  id: number;
  kind: string;
  severity: number;
  user_id?: UUID;
  user_name?: string;
  ip?: string;
  route?: string;
  details?: Record<string, unknown>;
  created_at: Timestamp;
}

export interface CarForm {
  origin: Origin;
  brand: string;
  model: string;
  generation?: string;
  trim_level?: string;
  year: number;
  mileage_km: number;
  engine_cc?: number;
  power_hp?: number;
  fuel: string;
  gearbox: string;
  drive: string;
  body: string;
  color?: string;
  seats?: number;
  steering_right?: boolean;
  auction_grade?: string;
  interior_grade?: string;
  vin?: string;
  price_minor: number;
  currency: string;
  title?: string;
  description?: string;
  equipment?: string[];
  photo_urls?: string[];
}

export interface PublicDealer {
  user_id: UUID;
  slug: string;
  company_name: string;
  city: string;
  description: string;
  logo_url?: string;
  services: string[];
  work_countries: string[];
  rating_avg: number;
  rating_count: number;
  deals_won: number;
  deals_total: number;
  cars_active: number;
  verified: boolean;
  verified_at?: Timestamp;
}

export interface DealerProfile extends PublicDealer {
  legal_name?: string;
  inn?: string;
  address?: string;
  website?: string;
  cover_url?: string;
}

export interface DealerForm {
  slug: string;
  company_name: string;
  legal_name?: string;
  inn?: string;
  city: string;
  address?: string;
  description?: string;
  website?: string;
  logo_url?: string;
  cover_url?: string;
  services: string[];
  work_countries: string[];
}

export type SocialNetwork = 'telegram' | 'vk' | 'whatsapp' | 'instagram' | 'youtube' | 'rutube';
export type SocialAccountStatus = 'disconnected' | 'connected' | 'needs_reauth' | 'error';

export interface SocialChannel {
  network: SocialNetwork;
  title: string;
  auth_kind: 'keys' | 'oauth';
  status: SocialAccountStatus;
  external_id?: string;
  last_error?: string;
  auto_post: boolean;
  token_mask?: string;
  chat_id?: string;
  owner_id?: string;
  phone_number_id?: string;
  business_account_id?: string;
  destination?: string;
  platform_ready: boolean;
  platform_hint?: string;
  publish_hint?: string;
}

export interface SocialChannelsResponse {
  items: SocialChannel[];
  platform: {
    vk: boolean;
    meta: boolean;
    google: boolean;
  };
}
