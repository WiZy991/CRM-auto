import { useQuery } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';

import { bannersApi, carsApi, dealersApi, dealsApi, errorMessage, requestsApi } from '@/lib/api';
import type { Banner, CarListItem, DealListItem, PipelineSummary, RequestListItem } from '@/lib/api';
import { formatDate, formatMonth, formatNumber, formatPercent, formatRubMinor } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { carStatusTitle, stageBoardTitle, stageIndexLabel } from '@/lib/status';
import { Button, EmptyState, Spinner } from '@/ui';

import { DealerReviews } from './AnalyticsPage';
import { downloadCsv, findReport, type ReportKind } from './analyticsReports';

export function ReportViewPage() {
  const { kind } = useParams();
  const meta = findReport(kind);

  if (!meta) {
    return (
      <EmptyState
        title="Такого отчёта нет"
        description="Вернитесь к списку и выберите таблицу из каталога."
        action={
          <Link to="/app/analytics/reports" className="text-sm text-[var(--link)] underline underline-offset-2">
            Все отчёты
          </Link>
        }
      />
    );
  }

  return (
    <div className="flex flex-col gap-6">
      <div className="flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between">
        <div>
          <Link to="/app/analytics/reports" className="text-xs text-[var(--link)] underline underline-offset-2">
            Все отчёты
          </Link>
          <h2 className="mt-1 text-lg font-semibold">{meta.title}</h2>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">{meta.text}</p>
        </div>
      </div>
      <ReportBody kind={meta.kind} title={meta.title} />
    </div>
  );
}

function ReportBody({ kind, title }: { kind: ReportKind; title: string }) {
  switch (kind) {
    case 'funnel':
    case 'revenue':
    case 'created':
    case 'markets':
    case 'cycle':
      return <SummaryReport kind={kind} title={title} />;
    case 'pipeline':
      return <DealsReport title={title} query={{ outcome: 'open', limit: 200 }} filename="pipeline" />;
    case 'won':
      return <DealsReport title={title} query={{ outcome: 'won', limit: 200 }} filename="won" />;
    case 'lost':
      return <DealsReport title={title} query={{ outcome: 'lost', limit: 200 }} filename="lost" />;
    case 'stale':
      return <DealsReport title={title} query={{ stale: true, limit: 200 }} filename="stale" />;
    case 'requests':
      return <RequestsReport title={title} />;
    case 'listings':
      return <ListingsReport title={title} />;
    case 'ads':
      return <AdsReport title={title} />;
    case 'reviews':
      return <ReviewsReport />;
  }
}

function SummaryReport({ kind, title }: { kind: Exclude<ReportKind, 'pipeline' | 'won' | 'lost' | 'stale' | 'requests' | 'listings' | 'ads' | 'reviews'>; title: string }) {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const stats = summary.data?.summary;

  if (summary.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (summary.isError) {
    return <EmptyState title="Не удалось загрузить отчёт" description={errorMessage(summary.error)} />;
  }
  if (!stats) return null;

  const pack = summaryPack(kind, stats);
  return (
    <ReportTable
      title={title}
      filename={kind}
      headers={pack.headers}
      rows={pack.rows}
      empty="По этому разрезу пока нет данных."
    />
  );
}

function summaryPack(kind: 'funnel' | 'revenue' | 'created' | 'markets' | 'cycle', stats: PipelineSummary) {
  if (kind === 'funnel') {
    const open = Math.max(stats.open_count, 1);
    return {
      headers: ['Этап', 'Сделок', 'Сумма', 'Зависли', 'Дней, ср.', 'Доля'],
      rows: stats.stages.map((item, index) => [
        `${stageIndexLabel(index + 1)} ${stageBoardTitle(item.stage, item.title)}`,
        String(item.count),
        formatRubMinor(item.amount_rub_minor),
        String(item.stale_count),
        item.count === 0 ? '—' : item.avg_days_on_stage.toFixed(1),
        formatPercent(item.count / open),
      ]),
    };
  }
  if (kind === 'revenue') {
    return {
      headers: ['Месяц', 'Выручка', 'Выдач'],
      rows: stats.revenue_by_month.map((row) => [
        formatMonth(row.month),
        formatRubMinor(row.amount_rub_minor),
        String(row.deals_count),
      ]),
    };
  }
  if (kind === 'created') {
    return {
      headers: ['Месяц', 'Новых сделок'],
      rows: stats.created_by_month.map((row) => [formatMonth(row.month), String(row.count)]),
    };
  }
  if (kind === 'markets') {
    return {
      headers: ['Рынок', 'В работе', 'Сумма в работе', 'Выдано', 'Выручка', 'Сорвано'],
      rows: stats.by_origin.map((row) => [
        row.title,
        String(row.open),
        formatRubMinor(row.open_amount_rub_minor),
        String(row.won),
        formatRubMinor(row.won_amount_rub_minor),
        String(row.lost),
      ]),
    };
  }
  return {
    headers: ['Этап', 'Сделок', 'Зависли', 'Дней на этапе, ср.', 'Цикл до выдачи, дн.'],
    rows: stats.stages.map((item, index) => [
      `${stageIndexLabel(index + 1)} ${stageBoardTitle(item.stage, item.title)}`,
      String(item.count),
      String(item.stale_count),
      item.count === 0 ? '—' : item.avg_days_on_stage.toFixed(1),
      index === 0 && stats.won_count > 0 ? stats.avg_cycle_days.toFixed(1) : '—',
    ]),
  };
}

function DealsReport({
  title,
  query,
  filename,
}: {
  title: string;
  query: { outcome?: string; stale?: boolean; limit?: number };
  filename: string;
}) {
  const list = useQuery({
    queryKey: queryKeys.deals(query),
    queryFn: ({ signal }) => dealsApi.list(query, signal),
  });
  if (list.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (list.isError) {
    return <EmptyState title="Не удалось загрузить сделки" description={errorMessage(list.error)} />;
  }
  const items = list.data?.items ?? [];
  return (
    <>
      {list.data && (
        <p className="text-sm text-[var(--text-muted)]">
          В выборке {formatNumber(items.length)}
          {list.data.total > items.length ? ` из ${formatNumber(list.data.total)}` : ''}
        </p>
      )}
      <ReportTable
        title={title}
        filename={filename}
        headers={['№', 'Клиент', 'Лот', 'Этап', 'Сумма', 'Дней', 'Обновлено']}
        rows={items.map(dealRow)}
        empty="Подходящих сделок нет."
        links={items.map((item) => `/app/deals/${item.id}`)}
      />
    </>
  );
}

function dealRow(item: DealListItem) {
  return [
    String(item.number),
    item.client_name,
    item.car_title || '—',
    stageBoardTitle(item.stage, item.stage_title),
    item.amount_label || (item.amount_rub_minor != null ? formatRubMinor(item.amount_rub_minor) : '—'),
    String(item.days_on_stage),
    formatDate(item.updated_at),
  ];
}

function RequestsReport({ title }: { title: string }) {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const list = useQuery({
    queryKey: queryKeys.requestsDealer({ limit: 200 }),
    queryFn: ({ signal }) => requestsApi.dealer({ limit: 200 }, signal),
  });
  if (summary.isPending || list.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (list.isError) {
    return <EmptyState title="Не удалось загрузить заявки" description={errorMessage(list.error)} />;
  }
  const buckets = summary.data?.summary.requests_by_status ?? [];
  const items = list.data?.items ?? [];
  return (
    <div className="flex flex-col gap-6">
      <ReportTable
        title={`${title} — статусы`}
        filename="requests-status"
        headers={['Статус', 'Заявок']}
        rows={buckets.map((item) => [item.title, String(item.count)])}
        empty="Заявок нет."
      />
      <ReportTable
        title={`${title} — список`}
        filename="requests"
        headers={['№', 'Клиент', 'Суть', 'Статус', 'Дата']}
        rows={items.map(requestRow)}
        empty="Списка заявок нет."
      />
    </div>
  );
}

function requestRow(item: RequestListItem) {
  return [
    String(item.number),
    item.client_name || '—',
    item.summary,
    item.status_title,
    formatDate(item.created_at),
  ];
}

function ListingsReport({ title }: { title: string }) {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const list = useQuery({
    queryKey: queryKeys.myCars({ limit: 50 }),
    queryFn: ({ signal }) => carsApi.mine({ limit: 50 }, signal),
  });
  if (summary.isPending || list.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (list.isError) {
    return <EmptyState title="Не удалось загрузить объявления" description={errorMessage(list.error)} />;
  }
  const buckets = summary.data?.summary.listings_by_status ?? [];
  const items = list.data?.items ?? [];
  return (
    <div className="flex flex-col gap-6">
      <ReportTable
        title={`${title} — статусы`}
        filename="listings-status"
        headers={['Статус', 'Лотов']}
        rows={buckets.map((item) => [item.title, String(item.count)])}
        empty="Объявлений нет."
      />
      <ReportTable
        title={`${title} — витрина`}
        filename="listings"
        headers={['Лот', 'Рынок', 'Статус', 'Цена']}
        rows={items.map(listingRow)}
        empty="Лотов на витрине нет."
        links={items.map((item) => `/catalog/${item.id}`)}
      />
    </div>
  );
}

function listingRow(item: CarListItem) {
  return [
    item.title,
    item.origin === 'cn' ? 'Китай' : 'Япония',
    carStatusTitle(item.status),
    item.price_label,
  ];
}

function AdsReport({ title }: { title: string }) {
  const list = useQuery({
    queryKey: queryKeys.bannersMine,
    queryFn: ({ signal }) => bannersApi.mine(signal),
  });
  if (list.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (list.isError) {
    return <EmptyState title="Не удалось загрузить рекламу" description={errorMessage(list.error)} />;
  }
  const items = list.data?.items ?? [];
  const stats = list.data?.stats;
  return (
    <div className="flex flex-col gap-6">
      {stats && (
        <p className="text-sm text-[var(--text-secondary)]">
          Всего {formatNumber(stats.total)}, активных {formatNumber(stats.active)}, показы{' '}
          {formatNumber(stats.impressions)}, клики {formatNumber(stats.clicks)}, CTR {formatPercent(stats.ctr)}.
        </p>
      )}
      <ReportTable
        title={title}
        filename="ads"
        headers={['Баннер', 'Место', 'Статус', 'Показы', 'Клики', 'CTR']}
        rows={items.map(adRow)}
        empty="Баннеров нет."
      />
    </div>
  );
}

function adRow(item: Banner) {
  return [
    item.title,
    item.placement_title,
    item.status_title,
    String(item.impressions),
    String(item.clicks),
    formatPercent(item.ctr),
  ];
}

function ReviewsReport() {
  const reviews = useQuery({
    queryKey: queryKeys.dealerMineReviews,
    queryFn: ({ signal }) => dealersApi.mineReviews(signal),
  });
  if (reviews.isPending) return <Spinner className="text-[var(--accent)]" />;
  if (reviews.isError) {
    return <EmptyState title="Не удалось загрузить отзывы" description={errorMessage(reviews.error)} />;
  }
  const items = reviews.data?.items ?? [];
  return (
    <div className="flex flex-col gap-6">
      <ReportTable
        title="Оценки"
        filename="reviews"
        headers={['Клиент', 'Оценка', 'Текст', 'Ответ']}
        rows={items.map((item) => [
          item.author_name || 'Клиент',
          String(item.rating),
          item.text || '',
          item.dealer_reply || '',
        ])}
        empty="Отзывов нет."
      />
      <DealerReviews items={items} />
    </div>
  );
}

function ReportTable({
  title,
  filename,
  headers,
  rows,
  empty,
  links,
}: {
  title: string;
  filename: string;
  headers: string[];
  rows: string[][];
  empty: string;
  links?: string[];
}) {
  return (
    <section>
      <div className="mb-3 flex items-center justify-between gap-3">
        <h3 className="text-sm font-medium">{title}</h3>
        <Button
          size="sm"
          variant="secondary"
          disabled={rows.length === 0}
          onClick={() => downloadCsv(`${filename}.csv`, headers, rows)}
        >
          CSV
        </Button>
      </div>
      {rows.length === 0 ? (
        <EmptyState title={empty} />
      ) : (
        <div className="overflow-x-auto">
          <table className="panel w-full text-sm">
            <thead className="bg-[var(--surface-sunken)] text-2xs tracking-[0.08em] text-[var(--text-muted)] uppercase">
              <tr>
                {headers.map((header) => (
                  <th key={header} className="px-3 py-2 text-left font-medium">
                    {header}
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {rows.map((row, index) => {
                const href = links?.[index];
                return (
                  <tr key={`${row[0] ?? 'row'}-${index}`} className="border-t border-[var(--border-hairline)]">
                    {row.map((cell, cellIndex) => (
                      <td key={`${headers[cellIndex] ?? cellIndex}-${cellIndex}`} className="px-3 py-2">
                        {cellIndex === 0 && href ? (
                          <Link to={href} className="text-[var(--link)] underline underline-offset-2">
                            {cell}
                          </Link>
                        ) : (
                          cell
                        )}
                      </td>
                    ))}
                  </tr>
                );
              })}
            </tbody>
          </table>
        </div>
      )}
    </section>
  );
}
