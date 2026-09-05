import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';

import { dealsApi, errorMessage } from '@/lib/api';
import type { PipelineSummary } from '@/lib/api';
import { formatNumber, formatRubMinor } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { EmptyState, Spinner } from '@/ui';

import { REPORTS } from './analyticsReports';

export function ReportsPage() {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const stats = summary.data?.summary;

  return (
    <div className="flex flex-col gap-6">
      <p className="text-sm text-[var(--text-secondary)]">
        Каждый отчёт — отдельная таблица. Из списка можно выгрузить CSV для Excel.
      </p>
      {summary.isPending && <Spinner className="text-[var(--accent)]" />}
      {summary.isError && (
        <EmptyState title="Не удалось загрузить цифры" description={errorMessage(summary.error)} />
      )}
      <ol className="grid gap-3 sm:grid-cols-2 xl:grid-cols-3">
        {REPORTS.map((item, index) => (
          <li key={item.kind}>
            <Link
              to={`/app/analytics/reports/${item.kind}`}
              className="panel block h-full px-4 py-4 hover:bg-[var(--surface-sunken)]"
            >
              <p className="numeric text-xs text-[var(--text-muted)]">{String(index + 1).padStart(2, '0')}</p>
              <p className="mt-2 text-sm font-semibold">{item.title}</p>
              <p className="mt-1 text-sm leading-relaxed text-[var(--text-secondary)]">{item.text}</p>
              {stats && <p className="numeric mt-3 text-sm">{badgeFor(item.kind, stats)}</p>}
            </Link>
          </li>
        ))}
      </ol>
    </div>
  );
}

function badgeFor(kind: string, stats: PipelineSummary) {
  switch (kind) {
    case 'funnel':
    case 'pipeline':
      return `${formatNumber(stats.open_count)} в работе`;
    case 'won':
      return `${formatNumber(stats.won_count)} · ${formatRubMinor(stats.won_amount_rub_minor)}`;
    case 'lost':
      return `${formatNumber(stats.lost_count)} · ${formatRubMinor(stats.lost_amount_rub_minor ?? 0)}`;
    case 'stale':
      return `${formatNumber(stats.stale_count ?? 0)} зависли`;
    case 'revenue':
      return formatRubMinor(stats.won_amount_rub_minor);
    case 'created':
      return `${formatNumber((stats.created_by_month ?? []).reduce((sum, row) => sum + row.count, 0))} за год`;
    case 'markets':
      return `${formatNumber((stats.by_origin ?? []).length)} рынка`;
    case 'cycle':
      return stats.won_count === 0 ? 'нет выдач' : `${stats.avg_cycle_days.toFixed(1)} дн.`;
    case 'requests':
      return `${formatNumber((stats.requests_by_status ?? []).reduce((sum, row) => sum + row.count, 0))} заявок`;
    case 'listings':
      return `${formatNumber((stats.listings_by_status ?? []).reduce((sum, row) => sum + row.count, 0))} лотов`;
    case 'ads':
      return `${formatNumber(stats.ads?.impressions ?? 0)} показов`;
    case 'reviews':
      return 'после выдачи';
    default:
      return '';
  }
}
