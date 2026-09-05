import { cn } from './cn';

/**
 * Индикатор ожидания.
 *
 * Нарисован дугой на SVG, а не рамкой div со скруглением: так толщина линии
 * не зависит от размера, и на 14 пикселях он остаётся читаемым.
 */
export function Spinner({ size = 16, className }: { size?: number; className?: string }) {
  return (
    <svg
      width={size}
      height={size}
      viewBox="0 0 24 24"
      fill="none"
      className={cn('animate-spin', className)}
      aria-hidden="true"
    >
      <circle cx="12" cy="12" r="9" stroke="currentColor" strokeOpacity="0.2" strokeWidth="2.5" />
      <path
        d="M21 12a9 9 0 0 0-9-9"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
      />
    </svg>
  );
}

export function FullPageSpinner({ label = 'Загрузка' }: { label?: string }) {
  return (
    <div
      className="flex min-h-[60vh] flex-col items-center justify-center gap-3"
      role="status"
      aria-live="polite"
    >
      <Spinner size={28} className="text-[var(--accent)]" />
      <span className="text-sm text-[var(--text-muted)]">{label}</span>
    </div>
  );
}
