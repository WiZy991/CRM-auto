import { Link } from 'react-router-dom';

import { cn } from './cn';

type BrandLogoProps = {
  to?: string | null;
  className?: string;
  /** Только знак без подписи — для узких мест. */
  markOnly?: boolean;
  /** Светлый текст на тёмном фоне (кабинет). */
  onDark?: boolean;
  subtitle?: string;
  /** Скрыть подпись на узком экране (шапка сайта). */
  hideSubtitleOnMobile?: boolean;
};

/**
 * GoImport: кремовая G + оранжевая стрелка «Go» — dual-read буква/движение.
 * В UI берём PNG-знак (оптика и антиалиасинг), не упрощённый клипарт.
 */
export function BrandLogo({
  to = '/',
  className,
  markOnly = false,
  onDark = false,
  subtitle,
  hideSubtitleOnMobile = false,
}: BrandLogoProps) {
  const body = (
    <>
      <img
        src="/brand/mark.png"
        alt=""
        width={36}
        height={36}
        className="size-9 shrink-0 rounded-[10px] object-cover shadow-[0_1px_0_rgb(20_18_14/0.2)]"
        decoding="async"
      />
      {!markOnly && (
        <span className="min-w-0">
          <span
            className={cn(
              'block font-display text-[18px] font-semibold leading-none tracking-[-0.02em]',
              onDark ? 'text-[var(--nav-text)]' : 'text-[var(--text-primary)]',
            )}
          >
            <span className="text-[var(--accent)]">Go</span>Import
          </span>
          {subtitle ? (
            <span
              className={cn(
                'mt-1 block truncate text-xs tracking-wide',
                hideSubtitleOnMobile && 'hidden sm:block',
                onDark ? 'text-[var(--nav-muted)]' : 'text-[var(--text-muted)]',
              )}
            >
              {subtitle}
            </span>
          ) : null}
        </span>
      )}
    </>
  );

  const classes = cn('inline-flex min-w-0 items-center gap-3', className);

  if (to == null) {
    return <span className={classes}>{body}</span>;
  }

  return (
    <Link to={to} className={classes} aria-label="GoImport — на главную">
      {body}
    </Link>
  );
}
