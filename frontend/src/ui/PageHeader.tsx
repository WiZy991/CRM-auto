import type { ReactNode } from 'react';

import { cn } from './cn';

export function PageHeader({
  kicker,
  title,
  description,
  actions,
  className,
}: {
  kicker?: string;
  title: string;
  description?: string;
  actions?: ReactNode;
  className?: string;
}) {
  return (
    <header
      className={cn(
        'flex flex-col gap-3 sm:flex-row sm:items-end sm:justify-between',
        className,
      )}
    >
      <div>
        {kicker && (
          <p className="text-xs font-medium text-[var(--accent)]">{kicker}</p>
        )}
        <h1 className="mt-1 text-2xl font-semibold tracking-tight text-balance md:text-[1.75rem]">{title}</h1>
        {description && (
          <p className="mt-2 max-w-3xl text-[15px] leading-relaxed text-[var(--text-secondary)]">{description}</p>
        )}
      </div>
      {actions && <div className="flex flex-wrap gap-2">{actions}</div>}
    </header>
  );
}
