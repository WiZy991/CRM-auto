import { useEffect, useId, useMemo, useRef, useState } from 'react';

import { cn } from './cn';

export interface ComboboxOption {
  value: string;
  title: string;
}

/**
 * Поиск по списку без выпадающих «облаков».
 *
 * Нативный select не умеет фильтровать по вводу, а готовые виджеты тянут
 * порталы и анимации. Здесь поле и список — одна колонка, как в прайс-листе.
 */
export function Combobox({
  label,
  hint,
  error,
  placeholder = 'Начните вводить',
  value,
  onChange,
  options,
  allowEmpty = true,
  emptyTitle = 'Все',
}: {
  label?: string;
  hint?: string;
  error?: string;
  placeholder?: string;
  value: string;
  onChange: (value: string) => void;
  options: readonly ComboboxOption[];
  allowEmpty?: boolean;
  emptyTitle?: string;
}) {
  const id = useId();
  const listId = `${id}-list`;
  const [query, setQuery] = useState('');
  const [open, setOpen] = useState(false);
  const root = useRef<HTMLDivElement>(null);

  const selected = options.find((item) => item.value === value);

  const filtered = useMemo(() => {
    const needle = query.trim().toLowerCase();
    if (!needle) return options;
    return options.filter(
      (item) =>
        item.title.toLowerCase().includes(needle) || item.value.toLowerCase().includes(needle),
    );
  }, [options, query]);

  useEffect(() => {
    function onDoc(event: MouseEvent) {
      if (!root.current?.contains(event.target as Node)) setOpen(false);
    }
    document.addEventListener('mousedown', onDoc);
    return () => document.removeEventListener('mousedown', onDoc);
  }, []);

  return (
    <div ref={root} className="flex flex-col gap-1.5">
      {label && (
        <label htmlFor={id} className="text-[13px] font-medium text-[var(--text-secondary)]">
          {label}
        </label>
      )}
      <input
        id={id}
        role="combobox"
        aria-expanded={open}
        aria-controls={listId}
        autoComplete="off"
        placeholder={selected?.title || placeholder}
        value={open ? query : (selected?.title ?? '')}
        onFocus={() => {
          setOpen(true);
          setQuery('');
        }}
        onChange={(event) => {
          setQuery(event.target.value);
          setOpen(true);
        }}
        className={cn(
          'h-11 w-full rounded-[var(--radius-sheet)] border bg-[var(--surface-raised)] px-3 text-base outline-none md:h-10 md:text-sm',
          error
            ? 'border-lacquer-500'
            : 'border-[var(--border-hairline)] hover:border-[var(--text-muted)] focus:border-[var(--accent)]',
        )}
      />
      {open && (
        <ul
          id={listId}
          role="listbox"
          className="max-h-56 overflow-auto rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] shadow-[var(--shadow-raise)]"
        >
          {allowEmpty && (
            <li>
              <button
                type="button"
                className="w-full px-3 py-2 text-left text-sm text-[var(--text-muted)] hover:bg-[var(--surface-sunken)]"
                onClick={() => {
                  onChange('');
                  setOpen(false);
                }}
              >
                {emptyTitle}
              </button>
            </li>
          )}
          {filtered.map((item) => (
            <li key={item.value}>
              <button
                type="button"
                role="option"
                aria-selected={item.value === value}
                className={cn(
                  'w-full px-3 py-2 text-left text-sm hover:bg-[var(--surface-sunken)]',
                  item.value === value && 'text-[var(--accent)]',
                )}
                onClick={() => {
                  onChange(item.value);
                  setOpen(false);
                }}
              >
                {item.title}
              </button>
            </li>
          ))}
          {filtered.length === 0 && (
            <li className="px-3 py-2 text-sm text-[var(--text-muted)]">Нет совпадений</li>
          )}
        </ul>
      )}
      {(error ?? hint) && (
        <p className={cn('text-xs', error ? 'text-lacquer-600' : 'text-[var(--text-muted)]')}>
          {error ?? hint}
        </p>
      )}
    </div>
  );
}
