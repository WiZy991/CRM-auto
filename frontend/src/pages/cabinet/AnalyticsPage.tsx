import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link } from 'react-router-dom';
import { useState } from 'react';

import { dealersApi, dealsApi, errorMessage } from '@/lib/api';
import type { CountBucket, DealReview, PipelineSummary, StageStat } from '@/lib/api';
import { formatMonth, formatNumber, formatPercent, formatRubMinor } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { stageBoardTitle, stageIndexLabel } from '@/lib/status';
import { BarChart, Button, EmptyState, Spinner, TextField, useToast } from '@/ui';

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
      {summary.isPending && <Spinner className="text-[var(--accent)]" />}
      {summary.isError && (
        <EmptyState title="Не удалось загрузить сводку" description={errorMessage(summary.error)} />
      )}
      {stats && (
        <>
          <nav className="flex flex-wrap gap-2" aria-label="Быстрый доступ к отчётам">
            <JumpLink to="/app/analytics/reports/won" label="Выданные" value={formatNumber(stats.won_count)} />
            <JumpLink to="/app/analytics/reports/lost" label="Отказы" value={formatNumber(stats.lost_count)} />
            <JumpLink to="/app/analytics/reports/stale" label="Зависли" value={formatNumber(staleCount)} />
            <JumpLink to="/app/analytics/reports/pipeline" label="В работе" value={formatNumber(stats.open_count)} />
            <Link
              to="/app/analytics/reports"
              className="inline-flex h-9 items-center rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] px-3 text-xs text-[var(--text-secondary)] hover:bg-[var(--surface-sunken)] hover:text-[var(--text-primary)]"
            >
              Все отчёты
            </Link>
          </nav>

          <section aria-label="Ключевые показатели">
            <div className="grid gap-3 sm:grid-cols-2 xl:grid-cols-4">
              <HeroKpi
                label="Выручка"
                value={formatRubMinor(stats.won_amount_rub_minor)}
                hint="По выданным сделкам"
                to="/app/analytics/reports/revenue"
              />
              <HeroKpi
                label="В работе"
                value={formatNumber(stats.open_count)}
                hint={formatRubMinor(openAmount)}
                to="/app/analytics/reports/pipeline"
              />
              <HeroKpi
                label="Выдано"
                value={formatNumber(stats.won_count)}
                hint={`конверсия ${formatPercent(stats.conversion)}`}
                to="/app/analytics/reports/won"
              />
              <HeroKpi
                label="Цикл"
                value={stats.won_count === 0 ? '—' : `${stats.avg_cycle_days.toFixed(1)} дн.`}
                hint="От открытия до выдачи"
                to="/app/analytics/reports/cycle"
              />
            </div>
            <dl className="mt-3 grid grid-cols-2 gap-x-4 gap-y-2 border-t border-[var(--border-hairline)] pt-3 sm:grid-cols-4">
              <MiniStat label="Сорвано" value={formatNumber(stats.lost_count)} />
              <MiniStat label="В воронке" value={formatRubMinor(openAmount)} />
              <MiniStat label="Зависли" value={formatNumber(staleCount)} />
              <MiniStat label="Конверсия" value={formatPercent(stats.conversion)} />
            </dl>
          </section>

          <section>
            <SectionHead title="Воронка" to="/app/analytics/reports/funnel" link="Подробнее" />
            <FunnelBoard stages={stats.stages} />
          </section>

          <div className="grid gap-6 xl:grid-cols-2">
            <section className="panel p-4">
              <SectionHead title="Выручка по месяцам" to="/app/analytics/reports/revenue" link="Отчёт" />
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
            <section className="panel p-4">
              <SectionHead title="Новые сделки" to="/app/analytics/reports/created" link="Отчёт" />
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

          <section>
            <SectionHead title="Китай и Япония" to="/app/analytics/reports/markets" link="Отчёт" />
            {origin.length === 0 ? (
              <EmptyState title="Сделок нет" description="Рынок берётся из лота в карточке сделки." />
            ) : (
              <OriginCards items={origin} />
            )}
          </section>

          <section>
            <h2 className="mb-3 text-sm font-medium">Заявки, лоты и реклама</h2>
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
          </section>

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

function JumpLink({ to, label, value }: { to: string; label: string; value: string }) {
  return (
    <Link
      to={to}
      className="inline-flex h-9 items-center gap-2 rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] bg-[var(--surface)] px-3 text-xs hover:bg-[var(--surface-sunken)]"
    >
      <span className="text-[var(--text-secondary)]">{label}</span>
      <span className="numeric font-semibold text-[var(--text-primary)]">{value}</span>
    </Link>
  );
}

function HeroKpi({
  label,
  value,
  hint,
  to,
}: {
  label: string;
  value: string;
  hint: string;
  to: string;
}) {
  return (
    <Link to={to} className="panel block px-4 py-4 transition-colors hover:bg-[var(--surface-sunken)]">
      <p className="text-sm text-[var(--text-muted)]">{label}</p>
      <p className="numeric mt-2 text-2xl font-semibold tracking-tight sm:text-3xl">{value}</p>
      <p className="mt-1 text-xs text-[var(--text-secondary)]">{hint}</p>
    </Link>
  );
}

function MiniStat({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-baseline justify-between gap-2 sm:block">
      <dt className="text-xs text-[var(--text-muted)]">{label}</dt>
      <dd className="numeric text-sm font-medium sm:mt-0.5">{value}</dd>
    </div>
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

function FunnelBoard({ stages }: { stages: readonly StageStat[] }) {
  const max = Math.max(...stages.map((item) => item.count), 1);

  return (
    <ol className="panel divide-y divide-[var(--border-hairline)]">
      {stages.map((item, index) => {
        const width = item.count <= 0 ? 0 : Math.max(6, (item.count / max) * 100);
        return (
          <li key={item.stage} className="px-4 py-3">
            <div className="flex flex-wrap items-baseline justify-between gap-2">
              <p className="text-sm font-medium">
                <span className="numeric text-[var(--text-muted)]">{stageIndexLabel(index + 1)}</span>{' '}
                {stageBoardTitle(item.stage, item.title)}
              </p>
              <p className="numeric text-sm">
                <span className="font-semibold">{formatNumber(item.count)}</span>
                <span className="text-[var(--text-muted)]">
                  {' '}
                  · {item.count === 0 ? '—' : `${item.avg_days_on_stage.toFixed(1)} дн.`}
                  {item.stale_count > 0 ? ` · зависло ${formatNumber(item.stale_count)}` : ''}
                </span>
              </p>
            </div>
            <div className="mt-2 h-1.5 bg-[var(--surface-sunken)]">
              <div
                className={item.count > 0 ? 'h-full bg-[var(--accent)]' : 'h-full'}
                style={{ width: `${width}%` }}
              />
            </div>
          </li>
        );
      })}
    </ol>
  );
}

function OriginCards({ items }: { items: PipelineSummary['by_origin'] }) {
  return (
    <div className="grid gap-3 sm:grid-cols-2">
      {items.map((item) => (
        <article key={item.origin || 'none'} className="panel px-4 py-4">
          <h3 className="text-sm font-semibold">{item.title}</h3>
          <p className="numeric mt-3 text-2xl font-semibold">{formatRubMinor(item.won_amount_rub_minor)}</p>
          <p className="mt-1 text-xs text-[var(--text-muted)]">выручка по выдачам</p>
          <dl className="mt-4 grid grid-cols-3 gap-2 border-t border-[var(--border-hairline)] pt-3 text-center">
            <div>
              <dt className="text-2xs text-[var(--text-muted)]">В работе</dt>
              <dd className="numeric mt-1 text-sm font-medium">{formatNumber(item.open)}</dd>
            </div>
            <div>
              <dt className="text-2xs text-[var(--text-muted)]">Выдано</dt>
              <dd className="numeric mt-1 text-sm font-medium">{formatNumber(item.won)}</dd>
            </div>
            <div>
              <dt className="text-2xs text-[var(--text-muted)]">Сорвано</dt>
              <dd className="numeric mt-1 text-sm font-medium">{formatNumber(item.lost)}</dd>
            </div>
          </dl>
        </article>
      ))}
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
