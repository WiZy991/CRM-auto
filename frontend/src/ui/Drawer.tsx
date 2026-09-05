import { useEffect, useId, type ReactNode } from 'react';

import { cn } from './cn';
import { Button } from './Button';

/**
 * Боковая или нижняя панель. Не модальное «стекло»: плотный лист поверх
 * страницы, как дополнительная колонка прайс-таблицы.
 */
export function Drawer({
  open,
  onClose,
  title,
  children,
  side = 'bottom',
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  side?: 'bottom' | 'right';
}) {
  const titleId = useId();

  useEffect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose();
    }
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  if (!open) return null;

  return (
    <div className="fixed inset-0 z-50">
      <button
        type="button"
        aria-label="Закрыть"
        className="absolute inset-0 bg-ink-900/40"
        onClick={onClose}
      />
      <aside
        role="dialog"
        aria-modal="true"
        aria-labelledby={titleId}
        className={cn(
          'absolute border border-[var(--border-hairline)] bg-[var(--surface-raised)] shadow-[var(--shadow-raise)]',
          side === 'bottom' &&
            'inset-x-0 bottom-0 max-h-[min(80vh,calc(100dvh-env(safe-area-inset-top)))] overflow-auto rounded-t-[var(--radius-card)] pb-[env(safe-area-inset-bottom)]',
          side === 'right' && 'inset-y-0 right-0 w-full max-w-sm overflow-auto',
        )}
      >
        <header className="flex items-center justify-between border-b border-[var(--border-hairline)] px-4 py-3">
          <h2 id={titleId} className="text-sm font-semibold">
            {title}
          </h2>
          <Button size="sm" variant="ghost" onClick={onClose}>
            Закрыть
          </Button>
        </header>
        <div className="p-4">{children}</div>
      </aside>
    </div>
  );
}
