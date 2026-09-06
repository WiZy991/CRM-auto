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
 * Логотип GoImport: знак (G + авто + стрелка) и слово.
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
        src="/brand/mark.svg"
        alt=""
        width={32}
        height={32}
        className="size-8 shrink-0 rounded-[9px]"
        decoding="async"
      />
      {!markOnly && (
        <span className="min-w-0">
          <span
            className={cn(
              'block font-display text-[17px] font-semibold leading-none tracking-tight',
              onDark ? 'text-[var(--nav-text)]' : 'text-[var(--text-primary)]',
            )}
          >
            Go<span className="text-[var(--accent)]">Import</span>
          </span>
          {subtitle ? (
            <span
              className={cn(
                'mt-1 block truncate text-xs',
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

  const classes = cn('inline-flex min-w-0 items-center gap-2.5', className);

  if (to == null) {
    return <span className={classes}>{body}</span>;
  }

  return (
    <Link to={to} className={classes} aria-label="GoImport — на главную">
      {body}
    </Link>
  );
}
