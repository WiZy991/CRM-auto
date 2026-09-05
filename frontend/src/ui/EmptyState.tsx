import type { ReactNode } from 'react';

import { cn } from './cn';

export function EmptyState({
  title,
  description,
  action,
  className,
}: {
  title: string;
  description?: string;
  action?: ReactNode;
  className?: string;
}) {
  return (
    <div
      className={cn(
        'flex flex-col items-start gap-3 rounded-[var(--radius-card)] border border-[var(--border-hairline)]',
        'bg-[var(--surface-raised)] px-6 py-12 shadow-[var(--shadow-card)]',
        className,
      )}
    >
      <p className="text-sm font-medium text-[var(--text-primary)]">{title}</p>
      {description && <p className="max-w-md text-sm text-[var(--text-secondary)]">{description}</p>}
      {action}
    </div>
  );
}
