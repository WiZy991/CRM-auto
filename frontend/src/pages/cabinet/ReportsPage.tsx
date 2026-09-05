import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';

import { dealsApi, errorMessage } from '@/lib/api';
import type { PipelineSummary } from '@/lib/api';
import { formatNumber, formatRubMinor } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { EmptyState, Spinner } from '@/ui';

import { REPORT_GROUPS, reportsInGroup } from './analyticsReports';

export function ReportsPage() {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const stats = summary.data?.summary;

  return (
    <div className="flex flex-col gap-8">
      <p className="text-sm text-[var(--text-secondary)]">
        Выберите таблицу — внутри можно открыть карточку сделки и скачать CSV для Excel.
      </p>
      {summary.isPending && <Spinner className="text-[var(--accent)]" />}
      {summary.isError && (
        <EmptyState title="Не удалось загрузить цифры" description={errorMessage(summary.error)} />
      )}
      {REPORT_GROUPS.map((group) => (
        <section key={group.id}>
          <div className="mb-3">
            <h2 className="text-sm font-medium">{group.title}</h2>
            <p className="mt-1 text-sm text-[var(--text-muted)]">{group.text}</p>
          </div>
          <ul className="grid gap-3 sm:grid-cols-2">
            {reportsInGroup(group.id).map((item) => (
              <li key={item.kind}>
                <Link
                  to={`/app/analytics/reports/${item.kind}`}
                  className="panel flex h-full flex-col px-4 py-4 hover:bg-[var(--surface-sunken)]"
                >
                  <div className="flex items-start justify-between gap-3">
                    <p className="text-sm font-semibold">{item.title}</p>
                    {stats && (
                      <p className="numeric shrink-0 text-xs text-[var(--accent)]">
                        {badgeFor(item.kind, stats)}
                      </p>
                    )}
                  </div>
                  <p className="mt-2 flex-1 text-sm leading-relaxed text-[var(--text-secondary)]">
                    {item.text}
                  </p>
                  <p className="mt-3 text-xs text-[var(--link)]">Открыть →</p>
                </Link>
              </li>
            ))}
          </ul>
        </section>
      ))}
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
      return `${formatNumber((stats.created_by_month ?? []).reduce((sum: number, row) => sum + row.count, 0))} за год`;
    case 'markets':
      return `${formatNumber((stats.by_origin ?? []).length)} рынка`;
    case 'cycle':
      return stats.won_count === 0 ? 'нет выдач' : `${stats.avg_cycle_days.toFixed(1)} дн.`;
    case 'requests':
      return `${formatNumber((stats.requests_by_status ?? []).reduce((sum: number, row) => sum + row.count, 0))} заявок`;
    case 'listings':
      return `${formatNumber((stats.listings_by_status ?? []).reduce((sum: number, row) => sum + row.count, 0))} лотов`;
    case 'ads':
      return `${formatNumber(stats.ads?.impressions ?? 0)} показов`;
    case 'reviews':
      return 'после выдачи';
    default:
      return '';
  }
}
