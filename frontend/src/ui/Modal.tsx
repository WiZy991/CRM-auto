import { useEffect, useId, useRef, type ReactNode } from 'react';

import { cn } from './cn';

export function Modal({
  open,
  onClose,
  title,
  children,
  footer,
  wide,
}: {
  open: boolean;
  onClose: () => void;
  title: string;
  children: ReactNode;
  footer?: ReactNode;
  wide?: boolean;
}) {
  const ref = useRef<HTMLDialogElement>(null);
  const titleId = useId();

  useEffect(() => {
    const node = ref.current;
    if (!node) return;
    if (open && !node.open) node.showModal();
    if (!open && node.open) node.close();
  }, [open]);

  return (
    <dialog
      ref={ref}
      aria-labelledby={titleId}
      onClose={onClose}
      onClick={(event) => {
        if (event.target === ref.current) onClose();
      }}
      className={cn(
        'm-auto max-h-[min(90dvh,calc(100dvh-1.5rem))] w-[calc(100%-1.5rem)] overflow-y-auto rounded-[var(--radius-card)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] p-0 text-[var(--text-primary)] shadow-[var(--shadow-raise)]',
        'backdrop:bg-ink-900/45',
        wide ? 'max-w-3xl' : 'max-w-lg',
      )}
    >
      <header className="flex items-start justify-between gap-4 border-b border-[var(--border-hairline)] px-5 py-4">
        <h2 id={titleId} className="text-lg font-semibold">
          {title}
        </h2>
        <button
          type="button"
          onClick={onClose}
          className="text-sm text-[var(--text-muted)] hover:text-[var(--text-primary)]"
        >
          Закрыть
        </button>
      </header>
      <div className="px-5 py-4">{children}</div>
      {footer && (
        <footer className="flex flex-wrap justify-end gap-2 border-t border-[var(--border-hairline)] px-5 py-3">
          {footer}
        </footer>
      )}
    </dialog>
  );
}
