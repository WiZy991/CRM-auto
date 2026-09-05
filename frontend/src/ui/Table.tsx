import type { ReactNode } from 'react';

import { cn } from './cn';

export interface TableColumn<T> {
  key: string;
  header: string;
  numeric?: boolean;
  className?: string;
  render: (row: T) => ReactNode;
}

export function DataTable<T>({
  columns,
  rows,
  rowKey,
  empty,
  onRowClick,
}: {
  columns: readonly TableColumn<T>[];
  rows: readonly T[];
  rowKey: (row: T) => string;
  empty?: ReactNode;
  onRowClick?: (row: T) => void;
}) {
  if (rows.length === 0) return <>{empty}</>;

  return (
    <>
      <ul className="flex flex-col gap-2 md:hidden">
        {rows.map((row) => (
          <li
            key={rowKey(row)}
            {...(onRowClick ? { onClick: () => onRowClick(row) } : {})}
            className={cn(
              'rounded-[var(--radius-card)] bg-[var(--surface-raised)] px-4 py-3 shadow-[var(--shadow-card)]',
              onRowClick && 'cursor-pointer active:bg-[var(--surface-sunken)]',
            )}
          >
            <dl className="flex flex-col gap-2">
              {columns.map((column) => (
                <div key={column.key} className="flex items-start justify-between gap-3">
                  {column.header ? (
                    <dt className="shrink-0 text-xs text-[var(--text-muted)]">
                      {column.header}
                    </dt>
                  ) : (
                    <dt className="sr-only">Действие</dt>
                  )}
                  <dd
                    className={cn(
                      'min-w-0 break-words text-right text-sm',
                      column.numeric && 'numeric',
                      !column.header && 'w-full text-right',
                    )}
                  >
                    {column.render(row)}
                  </dd>
                </div>
              ))}
            </dl>
          </li>
        ))}
      </ul>
      <div className="hidden overflow-hidden rounded-[var(--radius-card)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] shadow-[var(--shadow-card)] md:block">
        <table className="w-full min-w-[40rem] border-collapse text-sm">
          <thead>
            <tr className="border-b border-[var(--border-hairline)] bg-[var(--surface-sunken)]/70">
            {columns.map((column) => (
              <th
                key={column.key}
                className={cn(
                  'px-4 py-3 text-left text-[12px] font-medium text-[var(--text-muted)]',
                  column.numeric && 'text-right',
                  column.className,
                )}
              >
                {column.header}
              </th>
            ))}
          </tr>
        </thead>
        <tbody>
          {rows.map((row) => (
            <tr
              key={rowKey(row)}
              onClick={onRowClick ? () => onRowClick(row) : undefined}
              className={cn(
                'border-b border-[var(--border-hairline)] last:border-0',
                onRowClick && 'cursor-pointer hover:bg-[var(--surface-sunken)]/60',
              )}
            >
              {columns.map((column) => (
                <td
                  key={column.key}
                  className={cn(
                    'px-4 py-3.5 align-middle',
                    column.numeric && 'numeric text-right',
                    column.className,
                  )}
                >
                  {column.render(row)}
                </td>
              ))}
            </tr>
          ))}
        </tbody>
      </table>
      </div>
    </>
  );
}
