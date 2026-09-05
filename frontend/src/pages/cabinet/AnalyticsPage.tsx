import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { useState } from 'react';

import { dealersApi, dealsApi, errorMessage } from '@/lib/api';
import type { CountBucket, DealReview, PipelineSummary } from '@/lib/api';
import { formatMonth, formatNumber, formatPercent, formatRubMinor } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { stageBoardTitle, stageIndexLabel } from '@/lib/status';
import { BarChart, Button, EmptyState, FunnelStrip, PageGuide, Spinner, TextField, useToast } from '@/ui';

export function AnalyticsPage() {
  const summary = useQuery({
    queryKey: queryKeys.dealSummary,
    queryFn: ({ signal }) => dealsApi.summary(signal),
  });
  const reviews = useQuery({
    queryKey: queryKeys.dealerMineReviews,
    queryFn: ({ signal }) => dealersApi.mineReviews(signal),
  });

  const stats = summary.data?.summary;
  const months = stats?.revenue_by_month ?? [];
  const created = stats?.created_by_month ?? [];
  const origin = stats?.by_origin ?? [];
  const ads = stats?.ads;
  const openAmount =
    stats?.open_amount_rub_minor ??
    stats?.stages.reduce((sum, item) => sum + item.amount_rub_minor, 0) ??
    0;
  const staleCount =
    stats?.stale_count ?? stats?.stages.reduce((sum, item) => sum + item.stale_count, 0) ?? 0;

  return (
    <div className="flex flex-col gap-8">
      <PageGuide
        items={[
          { title: 'Сводка', text: 'Воронка, выручка, рынки, заявки и реклама на одном экране.' },
          { title: 'Отчёты', text: 'Вкладка справа: таблицы по каждому разрезу и выгрузка в CSV.' },
          { title: 'Карточка', text: 'Из отчёта по сделкам можно сразу открыть нужную воронку.' },
        ]}
      />
      {summary.isPending && <Spinner className="text-[var(--accent)]" />}
      {summary.isError && (
        <EmptyState title="Не удалось загрузить сводку" description={errorMessage(summary.error)} />
      )}
      {stats && (
        <>
          <div className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-8">
            <Kpi label="В работе" value={formatNumber(stats.open_count)} />
            <Kpi label="Выдано" value={formatNumber(stats.won_count)} />
            <Kpi label="Сорвано" value={formatNumber(stats.lost_count)} />
            <Kpi label="Конверсия" value={formatPercent(stats.conversion)} />
            <Kpi label="Выручка" value={formatRubMinor(stats.won_amount_rub_minor)} />
            <Kpi label="В воронке" value={formatRubMinor(openAmount)} />
            <Kpi label="Цикл, дни" value={stats.won_count === 0 ? '—' : stats.avg_cycle_days.toFixed(1)} />
            <Kpi label="Зависли" value={formatNumber(staleCount)} />
          </div>

          <FunnelStrip
            items={stats.stages.map((item) => ({
              stage: item.stage,
              title: item.title,
              count: item.count,
            }))}
          />

          <div className="grid gap-6 xl:grid-cols-2">
            <section>
              <SectionHead
                title="Выручка по месяцам"
                to="/app/analytics/reports/revenue"
                link="Отчёт"
              />
              {months.length === 0 ? (
                <EmptyState title="Выдач за год нет" description="Сумма считается по сделкам со статусом «выдана»." />
              ) : (
                <BarChart
                  items={months.map((row) => ({
                    label: formatMonth(row.month),
                    value: row.amount_rub_minor,
                    display: `${formatRubMinor(row.amount_rub_minor)} · ${formatNumber(row.deals_count)}`,
                  }))}
                />
              )}
            </section>
            <section>
              <SectionHead
                title="Новые сделки"
                to="/app/analytics/reports/created"
                link="Отчёт"
              />
              {created.length === 0 ? (
                <EmptyState title="Открытых сделок за год нет" description="График считает карточки по дате создания." />
              ) : (
                <BarChart
                  items={created.map((row) => ({
                    label: formatMonth(row.month),
                    value: row.count,
                    display: formatNumber(row.count),
                  }))}
                />
              )}
            </section>
          </div>

          <div className="grid gap-6 xl:grid-cols-2">
            <section>
              <SectionHead title="Китай и Япония" to="/app/analytics/reports/markets" link="Отчёт" />
              {origin.length === 0 ? (
                <EmptyState title="Сделок нет" description="Рынок берётся из лота в карточке сделки." />
              ) : (
                <OriginTable items={origin} />
              )}
            </section>
            <section>
              <SectionHead title="Срок на этапе" to="/app/analytics/reports/cycle" link="Отчёт" />
              <StageTable stats={stats} />
            </section>
          </div>

          <div className="grid gap-3 lg:grid-cols-3">
            <BucketCard
              title="Заявки"
              to="/app/analytics/reports/requests"
              items={stats.requests_by_status ?? []}
            />
            <BucketCard
              title="Объявления"
              to="/app/analytics/reports/listings"
              items={stats.listings_by_status ?? []}
            />
            <div className="panel px-4 py-4">
              <SectionHead title="Реклама" to="/app/analytics/reports/ads" link="Отчёт" />
              <dl className="mt-3 grid grid-cols-2 gap-3 text-sm">
                <div>
                  <dt className="text-[var(--text-muted)]">Баннеров</dt>
                  <dd className="numeric mt-1 text-lg font-semibold">{formatNumber(ads?.count ?? 0)}</dd>
                </div>
                <div>
                  <dt className="text-[var(--text-muted)]">CTR</dt>
                  <dd className="numeric mt-1 text-lg font-semibold">{formatPercent(ads?.ctr ?? 0)}</dd>
                </div>
                <div>
                  <dt className="text-[var(--text-muted)]">Показы</dt>
                  <dd className="numeric mt-1">{formatNumber(ads?.impressions ?? 0)}</dd>
                </div>
                <div>
                  <dt className="text-[var(--text-muted)]">Клики</dt>
                  <dd className="numeric mt-1">{formatNumber(ads?.clicks ?? 0)}</dd>
                </div>
              </dl>
            </div>
          </div>

          <DealerReviews items={reviews.data?.items ?? []} />
        </>
      )}
    </div>
  );
}

export function DealerReviews({ items }: { items: DealReview[] }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [replyFor, setReplyFor] = useState('');
  const [reply, setReply] = useState('');

  const send = useMutation({
    mutationFn: () => dealersApi.replyReview(replyFor, reply),
    onSuccess: () => {
      setReply('');
      setReplyFor('');
      toast.success('Ответ опубликован');
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerMineReviews });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <section>
      <SectionHead title="Отзывы клиентов" to="/app/analytics/reports/reviews" link="Отчёт" />
      {items.length === 0 ? (
        <EmptyState title="Отзывов нет" description="Появятся после выдачи автомобиля." />
      ) : (
        <ul className="panel divide-y divide-[var(--border-hairline)]">
          {items.map((item) => (
            <li key={item.id} className="px-4 py-3">
              <p className="text-sm font-medium">
                {item.author_name || 'Клиент'} · {item.rating} из 5
              </p>
              {item.text && <p className="mt-1 text-sm text-[var(--text-secondary)]">{item.text}</p>}
              {item.dealer_reply ? (
                <p className="mt-2 text-xs text-[var(--text-muted)]">Ваш ответ: {item.dealer_reply}</p>
              ) : replyFor === item.id ? (
                <div className="mt-2 flex flex-col gap-2 sm:flex-row">
                  <TextField
                    value={reply}
                    onChange={(event) => setReply(event.target.value)}
                    placeholder="Ответ"
                    fieldClassName="flex-1"
                  />
                  <Button size="sm" loading={send.isPending} onClick={() => send.mutate()}>
                    Отправить
                  </Button>
                </div>
              ) : (
                <Button className="mt-2" size="sm" variant="ghost" onClick={() => setReplyFor(item.id)}>
                  Ответить
                </Button>
              )}
            </li>
          ))}
        </ul>
      )}
    </section>
  );
}

function SectionHead({ title, to, link }: { title: string; to: string; link: string }) {
  return (
    <div className="mb-3 flex items-baseline justify-between gap-3">
      <h2 className="text-sm font-medium">{title}</h2>
      <Link to={to} className="text-xs text-[var(--link)] underline underline-offset-2">
        {link}
      </Link>
    </div>
  );
}

function OriginTable({ items }: { items: PipelineSummary['by_origin'] }) {
  return (
    <div className="overflow-x-auto">
      <table className="panel w-full text-sm">
        <thead className="bg-[var(--surface-sunken)] text-2xs tracking-[0.08em] text-[var(--text-muted)] uppercase">
          <tr>
            <th className="px-3 py-2 text-left font-medium">Рынок</th>
            <th className="numeric px-3 py-2 text-right font-medium">В работе</th>
            <th className="numeric px-3 py-2 text-right font-medium">Выдано</th>
            <th className="numeric px-3 py-2 text-right font-medium">Сорвано</th>
            <th className="numeric px-3 py-2 text-right font-medium">Выручка</th>
          </tr>
        </thead>
        <tbody>
          {items.map((item) => (
            <tr key={item.origin || 'none'} className="border-t border-[var(--border-hairline)]">
              <td className="px-3 py-2">{item.title}</td>
              <td className="numeric px-3 py-2 text-right">{formatNumber(item.open)}</td>
              <td className="numeric px-3 py-2 text-right">{formatNumber(item.won)}</td>
              <td className="numeric px-3 py-2 text-right">{formatNumber(item.lost)}</td>
              <td className="numeric px-3 py-2 text-right">{formatRubMinor(item.won_amount_rub_minor)}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function StageTable({ stats }: { stats: PipelineSummary }) {
  return (
    <div className="overflow-x-auto">
      <table className="panel w-full text-sm">
        <thead className="bg-[var(--surface-sunken)] text-2xs tracking-[0.08em] text-[var(--text-muted)] uppercase">
          <tr>
            <th className="px-3 py-2 text-left font-medium">Этап</th>
            <th className="numeric px-3 py-2 text-right font-medium">Сделок</th>
            <th className="numeric px-3 py-2 text-right font-medium">Зависли</th>
            <th className="numeric px-3 py-2 text-right font-medium">Дней, ср.</th>
          </tr>
        </thead>
        <tbody>
          {stats.stages.map((item, index) => (
            <tr key={item.stage} className="border-t border-[var(--border-hairline)]">
              <td className="px-3 py-2">
                <span className="numeric text-[var(--text-muted)]">{stageIndexLabel(index + 1)}</span>{' '}
                {stageBoardTitle(item.stage, item.title)}
              </td>
              <td className="numeric px-3 py-2 text-right">{item.count}</td>
              <td className="numeric px-3 py-2 text-right">{item.stale_count}</td>
              <td className="numeric px-3 py-2 text-right">
                {item.count === 0 ? '—' : item.avg_days_on_stage.toFixed(1)}
              </td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

function BucketCard({ title, to, items }: { title: string; to: string; items: CountBucket[] }) {
  const total = items.reduce((sum, item) => sum + item.count, 0);
  return (
    <div className="panel px-4 py-4">
      <SectionHead title={title} to={to} link="Отчёт" />
      <p className="numeric text-2xl font-semibold">{formatNumber(total)}</p>
      {items.length === 0 ? (
        <p className="mt-2 text-sm text-[var(--text-muted)]">Пока пусто</p>
      ) : (
        <ul className="mt-3 flex flex-col gap-1.5 text-sm">
          {items.map((item) => (
            <li key={item.key} className="flex justify-between gap-3">
              <span className="text-[var(--text-secondary)]">{item.title}</span>
              <span className="numeric">{formatNumber(item.count)}</span>
            </li>
          ))}
        </ul>
      )}
    </div>
  );
}

function Kpi({ label, value }: { label: string; value: string }) {
  return (
    <div className="panel px-4 py-4">
      <p className="text-sm text-[var(--text-muted)]">{label}</p>
      <p className="numeric mt-2 text-xl font-semibold sm:text-2xl">{value}</p>
    </div>
  );
}
