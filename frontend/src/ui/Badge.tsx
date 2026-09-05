import type { ReactNode } from 'react';

import { cn } from './cn';

export type BadgeTone = 'neutral' | 'accent' | 'harbor' | 'jade' | 'amber' | 'danger';

const TONES: Record<BadgeTone, string> = {
  neutral: 'bg-[var(--surface-sunken)] text-[var(--text-secondary)]',
  accent: 'bg-oxide-500/12 text-oxide-700',
  harbor: 'bg-harbor-500/12 text-harbor-700',
  jade: 'bg-jade-500/12 text-jade-600',
  amber: 'bg-amber-signal/15 text-amber-signal',
  danger: 'bg-lacquer-500/12 text-lacquer-600',
};

export function Badge({
  tone = 'neutral',
  children,
  className,
}: {
  tone?: BadgeTone;
  children: ReactNode;
  className?: string;
}) {
  return (
    <span
      className={cn(
        'inline-flex items-center rounded-md px-2 py-0.5',
        'text-[11px] font-medium tracking-[0.02em]',
        TONES[tone],
        className,
      )}
    >
      {children}
    </span>
  );
}

export function originTone(origin: string): BadgeTone {
  return origin === 'jp' ? 'harbor' : 'accent';
}

export function originTitle(origin: string): string {
  return origin === 'jp' ? 'Япония' : 'Китай';
}
