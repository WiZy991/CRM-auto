import type { BannerPublic } from '@/lib/api';

import { cn } from './cn';

type BannerSlotVariant = 'editorial' | 'strip' | 'rail' | 'card';

export function BannerSlot({
  banners,
  compact,
  variant = 'card',
  className,
}: {
  banners: readonly BannerPublic[];
  compact?: boolean;
  /** editorial — одна широкая полоса; strip — низкие 1–2; rail — узкая колонка; card — сетка карточек */
  variant?: BannerSlotVariant;
  className?: string;
}) {
  if (banners.length === 0) return null;

  if (variant === 'editorial') {
    const banner = banners[0];
    if (!banner) return null;
    return (
      <a
        href={banner.href}
        rel="noopener noreferrer"
        className={cn(
          'group relative block overflow-hidden border-y border-[var(--border-hairline)]',
          className,
        )}
      >
        <div className="relative min-h-44 w-full bg-[var(--surface-sunken)] sm:min-h-56 md:min-h-64">
          {banner.image_url ? (
            <img
              src={banner.image_url}
              alt=""
              className="absolute inset-0 size-full object-cover transition duration-500 group-hover:scale-[1.02]"
            />
          ) : (
            <div className="bg-hatch absolute inset-0" />
          )}
          <div className="absolute inset-0 bg-gradient-to-t from-black/55 via-black/15 to-transparent" />
          <div className="relative z-10 mx-auto flex min-h-44 w-full max-w-[1600px] flex-col justify-end px-4 py-8 sm:min-h-56 sm:px-6 md:min-h-64 lg:px-8">
            <p className="text-xs font-medium tracking-wide text-white/80">{banner.cta_label}</p>
            <p className="mt-1 max-w-xl text-xl font-semibold text-white sm:text-2xl">{banner.title}</p>
            {banner.subtitle ? (
              <p className="mt-2 max-w-lg text-sm text-white/85">{banner.subtitle}</p>
            ) : null}
          </div>
        </div>
      </a>
    );
  }

  if (variant === 'strip') {
    return (
      <div className={cn('flex flex-col gap-2', className)}>
        {banners.slice(0, 2).map((banner) => (
          <a
            key={banner.id}
            href={banner.href}
            rel="noopener noreferrer"
            className="flex min-h-14 items-stretch overflow-hidden border border-[var(--border-hairline)] bg-[var(--surface)]"
          >
            <div className="w-20 shrink-0 bg-[var(--surface-sunken)] sm:w-28">
              {banner.image_url ? (
                <img src={banner.image_url} alt="" className="size-full object-cover" />
              ) : (
                <div className="bg-hatch size-full" />
              )}
            </div>
            <div className="flex min-w-0 flex-1 flex-col justify-center px-3 py-2">
              <p className="truncate text-sm font-medium">{banner.title}</p>
              <p className="truncate text-xs text-[var(--text-muted)]">
                {banner.cta_label}
                {banner.subtitle ? ` · ${banner.subtitle}` : ''}
              </p>
            </div>
          </a>
        ))}
      </div>
    );
  }

  if (variant === 'rail') {
    // Узкая колонка: максимум 2 баннера, картинка сверху — не раздувает страницу.
    return (
      <aside className={cn('flex flex-col gap-3', className)}>
        {banners.slice(0, 2).map((banner) => (
          <a
            key={banner.id}
            href={banner.href}
            rel="noopener noreferrer"
            className="block overflow-hidden border border-[var(--border-hairline)] bg-[var(--surface)]"
          >
            <div className="aspect-[4/3] bg-[var(--surface-sunken)]">
              {banner.image_url ? (
                <img src={banner.image_url} alt="" className="size-full object-cover" />
              ) : (
                <div className="bg-hatch size-full" />
              )}
            </div>
            <div className="space-y-1 p-3">
              <p className="text-xs font-medium text-[var(--accent)]">{banner.cta_label}</p>
              <p className="text-sm font-semibold leading-snug">{banner.title}</p>
              {banner.subtitle ? (
                <p className="line-clamp-2 text-xs text-[var(--text-secondary)]">{banner.subtitle}</p>
              ) : null}
            </div>
          </a>
        ))}
      </aside>
    );
  }

  return (
    <div
      className={cn(
        'grid gap-3',
        banners.length > 1 && 'md:grid-cols-2',
        className,
      )}
    >
      {banners.slice(0, 2).map((banner) => (
        <a
          key={banner.id}
          href={banner.href}
          rel="noopener noreferrer"
          className={cn(
            'grid overflow-hidden border border-[var(--border-hairline)] bg-[var(--surface)] md:grid-cols-[1fr_1.1fr]',
            compact ? 'min-h-20' : 'min-h-24',
          )}
        >
          <div className="min-h-20 bg-[var(--surface-sunken)]">
            {banner.image_url ? (
              <img src={banner.image_url} alt="" className="size-full object-cover" />
            ) : (
              <div className="bg-hatch size-full" />
            )}
          </div>
          <div className="flex flex-col justify-end p-3">
            <p className="text-xs font-medium text-[var(--accent)]">{banner.cta_label}</p>
            <p className="mt-1 text-sm font-semibold">{banner.title}</p>
            {banner.subtitle && (
              <p className="mt-1 line-clamp-2 text-xs text-[var(--text-secondary)]">{banner.subtitle}</p>
            )}
          </div>
        </a>
      ))}
    </div>
  );
}
