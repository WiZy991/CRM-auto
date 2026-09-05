import type { Stage, StageMeta } from '@/lib/api';
import { stageBoardTitle, stageIndexLabel } from '@/lib/status';

import { cn } from './cn';

/**
 * Полоса этапов сделки.
 *
 * Семь позиций из домена, не из макета: порядок и названия приходят с
 * сервера, чтобы кабинет дилера и карточка клиента не разъехались.
 */
export function StageBar({
  stages,
  current,
  stale = false,
}: {
  stages: readonly StageMeta[];
  current: Stage;
  stale?: boolean;
}) {
  const currentIndex = stages.findIndex((item) => item.stage === current);
  const currentMeta = currentIndex >= 0 ? stages[currentIndex] : undefined;

  return (
    <div>
      <ol className="flex w-full min-w-0 gap-0" aria-label="Этапы сделки">
        {stages.map((meta, index) => {
          const done = index < currentIndex;
          const active = index === currentIndex;

          return (
            <li key={meta.stage} className="min-w-0 flex-1">
              <div
                className={cn(
                  'h-1',
                  done && 'bg-jade-500',
                  active && (stale ? 'bg-amber-signal' : 'bg-[var(--accent)]'),
                  !done && !active && 'bg-[var(--border-hairline)]',
                )}
              />
              <p
                className={cn(
                  'mt-2 hidden truncate text-xs sm:block',
                  active ? 'text-[var(--text-primary)]' : 'text-[var(--text-muted)]',
                )}
                title={meta.title}
              >
                {stageIndexLabel(index + 1)} {stageBoardTitle(meta.stage, meta.title)}
              </p>
            </li>
          );
        })}
      </ol>
      {currentMeta && (
        <p className="mt-2 text-sm sm:hidden">
          <span className="numeric text-[var(--text-muted)]">
            {stageIndexLabel(currentIndex + 1)}
          </span>{' '}
          {stageBoardTitle(currentMeta.stage, currentMeta.title)}
          <span className="text-[var(--text-muted)]">
            {' '}
            · {currentIndex + 1} из {stages.length}
          </span>
        </p>
      )}
      {currentMeta?.client_hint && (
        <p className="mt-2 text-sm text-[var(--text-secondary)]">{currentMeta.client_hint}</p>
      )}
    </div>
  );
}

/**
 * Сводка воронки: семь пронумерованных ячеек, как в техдокументации.
 * Это не график — подписи видны целиком, нули читаются сразу.
 */
export function FunnelStrip({
  items,
}: {
  items: readonly { stage: Stage; title: string; count: number }[];
}) {
  return (
    <ol className="grid grid-cols-2 gap-3 sm:grid-cols-4 xl:grid-cols-7">
      {items.map((item, index) => (
        <li key={item.stage} className="panel px-3.5 py-3.5">
          <p className="numeric text-xs text-[var(--text-muted)]">{stageIndexLabel(index + 1)}</p>
          <p className="mt-1 text-sm font-medium text-[var(--text-secondary)]">
            {stageBoardTitle(item.stage, item.title)}
          </p>
          <p
            className={cn(
              'numeric mt-2 text-2xl font-semibold',
              item.count > 0 ? 'text-[var(--text-primary)]' : 'text-[var(--text-muted)]',
            )}
          >
            {item.count}
          </p>
        </li>
      ))}
    </ol>
  );
}
