import { cn } from './cn';

export function Sparkline({ values, className }: { values: readonly number[]; className?: string }) {
  if (values.length === 0) return null;
  const max = Math.max(...values, 1);
  const points = values
    .map((value, index) => {
      const x = values.length === 1 ? 0 : (index / (values.length - 1)) * 100;
      const y = 22 - (value / max) * 20;
      return `${x.toFixed(1)},${y.toFixed(1)}`;
    })
    .join(' ');

  return (
    <svg viewBox="0 0 100 24" className={cn('h-6 w-24 text-[var(--accent)]', className)} aria-hidden>
      <polyline fill="none" stroke="currentColor" strokeWidth="1.5" points={points} />
    </svg>
  );
}

export function BarChart({
  items,
  className,
}: {
  items: readonly { label: string; value: number; display?: string }[];
  className?: string;
}) {
  const max = Math.max(...items.map((item) => item.value), 1);

  return (
    <ul className={cn('flex flex-col gap-3', className)} role="img">
      {items.map((item) => {
        const width = item.value <= 0 ? 0 : Math.max(4, (item.value / max) * 100);
        return (
          <li key={item.label}>
            <div className="mb-1 flex items-baseline justify-between gap-3">
              <span className="text-xs text-[var(--text-secondary)]">{item.label}</span>
              <span className="numeric text-xs text-[var(--text-primary)]">
                {item.display ?? String(item.value)}
              </span>
            </div>
            <div className="h-2.5 bg-[var(--surface-sunken)]">
              <div className="h-full bg-[var(--accent)]" style={{ width: `${width}%` }} />
            </div>
          </li>
        );
      })}
    </ul>
  );
}
