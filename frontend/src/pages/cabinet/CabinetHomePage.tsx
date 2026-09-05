import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { Link } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { adminApi, carsApi, dealsApi, errorMessage, sellersApi } from '@/lib/api';
import type { DealListItem, Stage } from '@/lib/api';
import { formatNumber, formatPercent } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { stageBoardTitle, stageIndexLabel } from '@/lib/status';
import {
  Badge,
  Button,
  EmptyState,
  FunnelStrip,
  LinkButton,
  LotCard,
  PageGuide,
  PageHeader,
  Spinner,
  StageBar,
  useToast,
} from '@/ui';

import { CreateDealModal } from './CreateDealModal';

export function CabinetHomePage() {
  const { hasRole } = useAuth();
  if (hasRole('admin')) return <AdminHome />;
  if (hasRole('seller')) return <SellerHome />;
  if (hasRole('dealer')) return <DealerHome />;
  return <ClientHome />;
}

function ClientHome() {
  const { user } = useAuth();
  const [tab, setTab] = useState<'open' | 'history'>('open');
  const stages = useQuery({
    queryKey: queryKeys.stages,
    queryFn: ({ signal }) => dealsApi.stages(signal),
  });
  const deals = useQuery({
    queryKey: queryKeys.deals({ limit: 50 }),
    queryFn: ({ signal }) => dealsApi.list({ limit: 50 }, signal),
  });
  const lots = useQuery({
    queryKey: queryKeys.catalog({ limit: 6, sort: 'fresh' }),
    queryFn: ({ signal }) => carsApi.list({ limit: 6, sort: 'fresh' }, signal),
  });

  const items = (deals.data?.items ?? []).filter((deal) =>
    tab === 'open' ? deal.outcome === 'open' : deal.outcome !== 'open',
  );

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        {...(user?.role_title ? { kicker: user.role_title } : {})}
        title={`Здравствуйте, ${user?.full_name ?? ''}`}
        description="Здесь ваши покупки: открытая сделка с этапами и история выдачи. Машину выбирайте в каталоге — дилер откроет сделку по заявке."
      />
      {lots.data && lots.data.items.length > 0 && (
        <section>
          <div className="mb-3 flex items-end justify-between gap-3">
            <h2 className="text-sm font-medium">Доступные автомобили</h2>
            <Link to="/catalog" className="text-sm text-[var(--link)] underline underline-offset-2">
              Весь каталог
            </Link>
          </div>
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
            {lots.data.items.map((car) => (
              <LotCard key={car.id} car={car} />
            ))}
          </div>
        </section>
      )}
      <div className="flex gap-2">
        <Button size="sm" variant={tab === 'open' ? 'primary' : 'secondary'} onClick={() => setTab('open')}>
          Активные
        </Button>
        <Button size="sm" variant={tab === 'history' ? 'primary' : 'secondary'} onClick={() => setTab('history')}>
          История
        </Button>
      </div>
      {deals.isPending && <Spinner className="text-[var(--accent)]" />}
      {deals.isError && (
        <EmptyState title="Не удалось загрузить сделки" description={errorMessage(deals.error)} />
      )}
      {deals.data && items.length === 0 && (
        <EmptyState
          title={tab === 'open' ? 'Открытых сделок нет' : 'Закрытых сделок нет'}
          description={
            tab === 'open'
              ? 'Оставьте заявку по автомобилю из каталога — дилер откроет сделку.'
              : 'Здесь появятся выданные и закрытые сделки.'
          }
          action={tab === 'open' ? <LinkButton to="/catalog">В каталог</LinkButton> : undefined}
        />
      )}
      {items.length > 0 && (
        <ul className="panel divide-y divide-[var(--border-hairline)]">
          {items.map((deal) => (
            <li key={deal.id}>
              <Link to={`/app/deals/${deal.id}`} className="flex flex-col gap-3 px-4 py-3 sm:flex-row sm:items-center sm:justify-between">
                <div className="min-w-0">
                  <p className="text-sm font-medium">{deal.title}</p>
                  <p className="text-xs text-[var(--text-muted)]">
                    № {deal.number} · {deal.stage_title}
                    {deal.outcome !== 'open' ? ` · ${deal.outcome === 'won' ? 'выдана' : 'отказ'}` : ''}
                  </p>
                  {stages.data && (
                    <div className="mt-3">
                      <StageBar stages={stages.data.items} current={deal.stage} stale={deal.is_stale} />
                    </div>
                  )}
                </div>
                <div className="flex flex-wrap items-center gap-2">
                  {deal.is_stale && <Badge tone="amber">Зависла</Badge>}
                  {deal.outcome === 'won' && <Badge tone="jade">Выдана</Badge>}
                  {deal.outcome === 'lost' && <Badge tone="danger">Отказ</Badge>}
                  {deal.amount_label && <span className="numeric text-sm">{deal.amount_label}</span>}
                </div>
              </Link>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function DealerHome() {
  const { user } = useAuth();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [createStage, setCreateStage] = useState<Stage>('lead');

  const stages = useQuery({
    queryKey: queryKeys.stages,
    queryFn: ({ signal }) => dealsApi.stages(signal),
  });
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const deals = useQuery({
    queryKey: queryKeys.deals({ outcome: 'open', limit: 200 }),
    queryFn: ({ signal }) => dealsApi.list({ outcome: 'open', limit: 200 }, signal),
  });

  const move = useMutation({
    mutationFn: ({ id, stage }: { id: string; stage: Stage }) =>
      dealsApi.changeStage(id, stage, 'Перенос в канбане'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['deals'] });
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealSummary });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const stats = summary.data?.summary;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        {...(user?.role_title ? { kicker: user.role_title } : {})}
        title="Воронка"
        description="Рабочий стол дилера: колонки — этапы сделки от лида до выдачи. Новую сделку создаёте сами или берёте заявку покупателя из пула."
        actions={
          <div className="flex flex-wrap gap-2">
            <Button
              variant="primary"
              size="sm"
              onClick={() => {
                setCreateStage('lead');
                setCreateOpen(true);
              }}
            >
              Новая сделка
            </Button>
            <LinkButton to="/app/analytics" size="sm">
              Аналитика
            </LinkButton>
            <LinkButton to="/app/analytics/reports" size="sm">
              Отчёты
            </LinkButton>
          </div>
        }
      />

      <PageGuide
        items={[
          { title: 'Новая сделка', text: 'Кнопка справа вверху: клиент, сумма, этап — как в классической CRM.' },
          { title: 'Перетащите карточку', text: 'Сдвиг вправо, когда этап сделан. В карточке — документы и переписка.' },
          { title: 'Заявки рядом', text: 'Пул покупателей — в разделе «Заявки». Лоты на витрину — в «Объявлениях».' },
        ]}
      />

      {stats && (
        <div className="grid grid-cols-2 gap-3 sm:grid-cols-4">
          <Kpi label="Сделок в работе" value={formatNumber(stats.open_count)} />
          <Kpi label="Выдано клиентам" value={formatNumber(stats.won_count)} />
          <Kpi label="Конверсия" value={formatPercent(stats.conversion)} />
          <Kpi label="Цикл, дн." value={stats.avg_cycle_days.toFixed(0)} />
        </div>
      )}

      {stats && (
        <FunnelStrip
          items={stats.stages.map((item) => ({
            stage: item.stage,
            title: item.title,
            count: item.count,
          }))}
        />
      )}

      {stages.isPending || deals.isPending ? (
        <Spinner className="text-[var(--accent)]" />
      ) : (
        <Kanban
          stages={stages.data?.items ?? []}
          deals={deals.data?.items ?? []}
          onDrop={(id, stage) => move.mutate({ id, stage })}
          onCreate={(stage) => {
            setCreateStage(stage);
            setCreateOpen(true);
          }}
        />
      )}

      {deals.data && deals.data.items.length === 0 && (
        <EmptyState
          title="На этом аккаунте открытых сделок нет"
          description="Создайте сделку вручную — как в CRM: клиент, сумма, этап. Либо возьмите заявку из пула."
          action={
            <Button
              variant="primary"
              onClick={() => {
                setCreateStage('lead');
                setCreateOpen(true);
              }}
            >
              Новая сделка
            </Button>
          }
        />
      )}

      <CreateDealModal
        open={createOpen}
        defaultStage={createStage}
        onClose={() => setCreateOpen(false)}
      />
    </div>
  );
}

function Kpi({ label, value }: { label: string; value: string }) {
  return (
    <div className="panel px-4 py-4">
      <p className="text-sm text-[var(--text-muted)]">{label}</p>
      <p className="numeric mt-2 text-2xl font-semibold">{value}</p>
    </div>
  );
}

function Kanban({
  stages,
  deals,
  onDrop,
  onCreate,
}: {
  stages: readonly { stage: Stage; title: string }[];
  deals: readonly DealListItem[];
  onDrop: (id: string, stage: Stage) => void;
  onCreate: (stage: Stage) => void;
}) {
  return (
    <div className="-mx-1 overflow-x-auto overscroll-x-contain px-1 pb-1 [scrollbar-width:thin]">
      <p className="mb-2 text-xs text-[var(--text-muted)] xl:hidden">Листайте этапы вбок</p>
      <div className="flex min-w-max snap-x snap-mandatory gap-3 xl:min-w-0 xl:grid xl:grid-cols-7">
        {stages.map((meta, index) => {
          const column = deals.filter((deal) => deal.stage === meta.stage);
          return (
            <section
              key={meta.stage}
              className="panel flex w-[min(16.5rem,85vw)] shrink-0 snap-start flex-col xl:min-h-[calc(100dvh-14rem)] xl:w-auto"
              onDragOver={(event) => event.preventDefault()}
              onDrop={(event) => {
                event.preventDefault();
                const id = event.dataTransfer.getData('text/plain');
                const current = deals.find((item) => item.id === id);
                if (id && current && current.stage !== meta.stage) onDrop(id, meta.stage);
              }}
            >
              <header className="flex items-baseline justify-between gap-2 border-b border-[var(--border-hairline)] px-3 py-2.5">
                <h2 className="text-sm font-medium">
                  <span className="numeric text-[var(--text-muted)]">
                    {stageIndexLabel(index + 1)}
                  </span>{' '}
                  {stageBoardTitle(meta.stage, meta.title)}
                </h2>
                <span className="flex items-center gap-1">
                  <span className="numeric text-xs text-[var(--text-muted)]">{column.length}</span>
                  <button
                    type="button"
                    className="text-xs text-[var(--text-muted)] hover:text-[var(--text-primary)]"
                    onClick={() => onCreate(meta.stage)}
                    aria-label={`Добавить сделку на этап ${meta.title}`}
                  >
                    +
                  </button>
                </span>
              </header>
              <ul className="flex min-h-48 flex-1 flex-col gap-2 p-2">
                {column.map((deal) => (
                  <li
                    key={deal.id}
                    draggable
                    onDragStart={(event) => event.dataTransfer.setData('text/plain', deal.id)}
                    className="cursor-grab rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] bg-[var(--surface)] p-3 active:cursor-grabbing"
                  >
                    <Link to={`/app/deals/${deal.id}`} className="block">
                      <p className="text-sm font-medium leading-snug">{deal.title}</p>
                      <p className="mt-1 text-2xs text-[var(--text-muted)]">
                        № {deal.number}
                        {deal.client_name ? ` · ${deal.client_name}` : ''}
                      </p>
                      {deal.amount_label && (
                        <p className="numeric mt-2 text-sm">{deal.amount_label}</p>
                      )}
                      {deal.is_stale && (
                        <Badge className="mt-2" tone="amber">
                          Зависла
                        </Badge>
                      )}
                    </Link>
                  </li>
                ))}
              </ul>
            </section>
          );
        })}
      </div>
    </div>
  );
}

function SellerHome() {
  const mine = useQuery({
    queryKey: queryKeys.sellerMine,
    queryFn: ({ signal }) => sellersApi.mine(signal),
  });
  const seller = mine.data?.items[0];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Поставщик"
        title={seller?.display_name ?? 'Карточка поставщика'}
        description="Так вас видят дилеры в справочнике: страна, тип, бренды. Контакты гостю сайта не показываем."
      />
      {mine.isPending && <Spinner className="text-[var(--accent)]" />}
      {mine.isError && <EmptyState title="Карточка недоступна" description={errorMessage(mine.error)} />}
      {seller && (
        <div className="panel grid gap-4 p-5 sm:grid-cols-2">
          <p className="text-sm">
            {seller.country_title} · {seller.kind_title}
          </p>
          <p className="text-sm">{seller.region}</p>
          <p className="text-sm sm:col-span-2">{seller.description || 'Описание не заполнено.'}</p>
          <p className="text-sm">Бренды: {seller.brands.join(', ') || '—'}</p>
          <p className="text-sm">Мин. партия: {seller.min_order_qty || 'не указана'}</p>
          <LinkButton to={`/app/sellers/${seller.id}`}>Редактировать</LinkButton>
        </div>
      )}
      {!mine.isPending && !seller && (
        <EmptyState
          title="Карточка ещё не создана"
          description="Администратор или вы сами можете завести карточку поставщика."
        />
      )}
    </div>
  );
}

function AdminHome() {
  const overview = useQuery({
    queryKey: queryKeys.adminOverview,
    queryFn: ({ signal }) => adminApi.overview(signal),
  });
  const data = overview.data?.overview;

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        kicker="Админка"
        title="Сводка площадки"
        description="Очереди на проверку, живые сделки и безопасность. Модерация лотов и баннеров — отдельный раздел слева."
      />
      {overview.isPending && <Spinner className="text-[var(--accent)]" />}
      {overview.isError && (
        <EmptyState title="Сводка недоступна" description={errorMessage(overview.error)} />
      )}
      {data && (
        <div className="grid gap-3 sm:grid-cols-3">
          <Kpi label="Пользователи" value={formatNumber(data.users_total)} />
          <Kpi label="Новые за 7 дн." value={formatNumber(data.users_new_7d)} />
          <Kpi label="Дилеры" value={formatNumber(data.dealers_total)} />
          <Kpi label="Лоты в продаже" value={formatNumber(data.cars_active)} />
          <Kpi label="На модерации" value={formatNumber(data.cars_moderation)} />
          <Kpi label="Открытые заявки" value={formatNumber(data.requests_open)} />
          <Kpi label="Открытые сделки" value={formatNumber(data.deals_open)} />
          <Kpi label="Выдано за 30 дн." value={formatNumber(data.deals_won_30d)} />
          <Kpi label="Баннеры на проверке" value={formatNumber(data.banners_moderation)} />
          <Kpi label="События за сутки" value={formatNumber(data.security_events_24h)} />
          <Kpi label="Заблокированные IP" value={formatNumber(data.blocked_ips)} />
        </div>
      )}
    </div>
  );
}
