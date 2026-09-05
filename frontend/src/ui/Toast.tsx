import { createContext, use, useCallback, useMemo, useState, type ReactNode } from 'react';

import { cn } from './cn';

export type ToastTone = 'ok' | 'error' | 'info';

interface ToastItem {
  id: number;
  tone: ToastTone;
  title: string;
  description?: string;
}

interface ToastApi {
  push: (tone: ToastTone, title: string, description?: string) => void;
  success: (title: string, description?: string) => void;
  error: (title: string, description?: string) => void;
  info: (title: string, description?: string) => void;
}

const ToastContext = createContext<ToastApi | null>(null);

let nextId = 1;

export function ToastProvider({ children }: { children: ReactNode }) {
  const [items, setItems] = useState<ToastItem[]>([]);

  const push = useCallback((tone: ToastTone, title: string, description?: string) => {
    const id = nextId++;
    const item: ToastItem = description
      ? { id, tone, title, description }
      : { id, tone, title };
    setItems((current) => [...current, item].slice(-4));
    window.setTimeout(() => {
      setItems((current) => current.filter((entry) => entry.id !== id));
    }, 4200);
  }, []);

  const api = useMemo<ToastApi>(
    () => ({
      push,
      success: (title, description) => push('ok', title, description),
      error: (title, description) => push('error', title, description),
      info: (title, description) => push('info', title, description),
    }),
    [push],
  );

  return (
    <ToastContext value={api}>
      {children}
      <ol className="pointer-events-none fixed right-4 bottom-[calc(4.5rem+env(safe-area-inset-bottom))] z-50 flex w-[min(22rem,calc(100%-2rem))] flex-col gap-2 lg:bottom-4">
        {items.map((item) => (
          <li
            key={item.id}
            className={cn(
              'pointer-events-auto rounded-[var(--radius-sheet)] border px-4 py-3 text-sm shadow-[var(--shadow-raise)]',
              item.tone === 'error' && 'border-lacquer-500/50 bg-[var(--surface-raised)] text-lacquer-600',
              item.tone === 'ok' && 'border-jade-500/40 bg-[var(--surface-raised)]',
              item.tone === 'info' && 'border-[var(--border-hairline)] bg-[var(--surface-raised)]',
            )}
            role="status"
          >
            <p className="font-medium">{item.title}</p>
            {item.description && (
              <p className="mt-1 text-xs text-[var(--text-secondary)]">{item.description}</p>
            )}
          </li>
        ))}
      </ol>
    </ToastContext>
  );
}

export function useToast(): ToastApi {
  const value = use(ToastContext);
  if (!value) throw new Error('useToast вызван вне ToastProvider');
  return value;
}
