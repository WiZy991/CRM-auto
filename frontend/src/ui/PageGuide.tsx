import { cn } from './cn';

export function PageGuide({
  items,
  className,
}: {
  items: readonly { title: string; text: string }[];
  className?: string;
}) {
  const cols =
    items.length >= 4
      ? 'sm:grid-cols-2 lg:grid-cols-4'
      : items.length === 3
        ? 'sm:grid-cols-3'
        : 'sm:grid-cols-2';

  return (
    <ol className={cn('grid gap-3', cols, className)}>
      {items.map((item, index) => (
        <li key={item.title} className="panel px-4 py-4">
          <p className="numeric text-xs text-[var(--accent)]">{String(index + 1).padStart(2, '0')}</p>
          <p className="mt-2 text-sm font-semibold">{item.title}</p>
          <p className="mt-1 text-sm leading-relaxed text-[var(--text-secondary)]">{item.text}</p>
        </li>
      ))}
    </ol>
  );
}
