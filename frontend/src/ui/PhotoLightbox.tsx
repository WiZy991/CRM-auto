import { useEffect, useId } from 'react';

import { cn } from './cn';

export function PhotoLightbox({
  photos,
  index,
  open,
  title,
  onClose,
  onIndex,
}: {
  photos: readonly string[];
  index: number;
  open: boolean;
  title?: string;
  onClose: () => void;
  onIndex: (index: number) => void;
}) {
  const titleId = useId();
  const current = photos[index];
  const total = photos.length;

  useEffect(() => {
    if (!open) return;
    function onKey(event: KeyboardEvent) {
      if (event.key === 'Escape') onClose();
      if (event.key === 'ArrowLeft') onIndex((index - 1 + total) % total);
      if (event.key === 'ArrowRight') onIndex((index + 1) % total);
    }
    document.addEventListener('keydown', onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = 'hidden';
    return () => {
      document.removeEventListener('keydown', onKey);
      document.body.style.overflow = prev;
    };
  }, [open, index, total, onClose, onIndex]);

  if (!open || !current) return null;

  return (
    <div className="fixed inset-0 z-50 flex flex-col bg-ink-900/92" role="dialog" aria-modal="true" aria-labelledby={titleId}>
      <header className="flex shrink-0 items-center justify-between gap-3 px-4 py-3 text-paper-50">
        <p id={titleId} className="truncate text-sm">
          {title ?? 'Фото'}
          <span className="numeric ml-3 text-[var(--text-muted)]">
            {index + 1} / {total}
          </span>
        </p>
        <button type="button" className="text-sm text-paper-100 hover:text-white" onClick={onClose}>
          Закрыть
        </button>
      </header>

      <div className="relative flex min-h-0 flex-1 items-center justify-center px-12 py-2">
        {total > 1 && (
          <button
            type="button"
            className="absolute left-2 top-1/2 z-10 -translate-y-1/2 rounded-[var(--radius-sheet)] bg-ink-800 px-3 py-4 text-paper-50 hover:bg-ink-700"
            onClick={() => onIndex((index - 1 + total) % total)}
            aria-label="Предыдущее фото"
          >
            ‹
          </button>
        )}
        <img src={current} alt="" className="max-h-full max-w-full object-contain" />
        {total > 1 && (
          <button
            type="button"
            className="absolute right-2 top-1/2 z-10 -translate-y-1/2 rounded-[var(--radius-sheet)] bg-ink-800 px-3 py-4 text-paper-50 hover:bg-ink-700"
            onClick={() => onIndex((index + 1) % total)}
            aria-label="Следующее фото"
          >
            ›
          </button>
        )}
      </div>

      {total > 1 && (
        <div className="flex shrink-0 justify-center gap-1 overflow-x-auto px-4 py-3">
          {photos.map((src, i) => (
            <button
              key={src + String(i)}
              type="button"
              aria-label={`Фото ${i + 1}`}
              aria-current={i === index}
              className={cn(
                'h-12 w-16 shrink-0 overflow-hidden rounded-[var(--radius-sheet)] border',
                i === index ? 'border-[var(--accent)]' : 'border-transparent opacity-70 hover:opacity-100',
              )}
              onClick={() => onIndex(i)}
            >
              <img src={src} alt="" className="size-full object-cover" />
            </button>
          ))}
        </div>
      )}
    </div>
  );
}
