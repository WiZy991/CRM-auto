import type { BannerPublic } from '@/lib/api';

import { cn } from './cn';

type BannerSlotVariant = 'editorial' | 'strip' | 'card';

export function BannerSlot({
  banners,
  compact,
  variant = 'card',
  className,
}: {
  banners: readonly BannerPublic[];
  compact?: boolean;
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
            className="flex min-h-16 items-stretch overflow-hidden border border-[var(--border-hairline)] bg-[var(--surface)]"
          >
            <div className="w-24 shrink-0 bg-[var(--surface-sunken)] sm:w-32">
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

  return (
    <div
      className={cn(
        'grid gap-3',
        banners.length > 1 && 'md:grid-cols-2',
        className,
      )}
    >
      {banners.map((banner) => (
        <a
          key={banner.id}
          href={banner.href}
          rel="noopener noreferrer"
          className={cn(
            'grid overflow-hidden border border-[var(--border-hairline)] bg-[var(--surface)] md:grid-cols-[1.2fr_1fr]',
            compact ? 'min-h-24' : 'min-h-28 md:min-h-36',
          )}
        >
          <div className="bg-[var(--surface-sunken)]">
            {banner.image_url ? (
              <img src={banner.image_url} alt="" className="size-full object-cover" />
            ) : (
              <div className="bg-hatch size-full" />
            )}
          </div>
          <div className="flex flex-col justify-end p-4">
            <p className="text-xs font-medium text-[var(--accent)]">{banner.cta_label}</p>
            <p className="mt-1 text-base font-semibold">{banner.title}</p>
            {banner.subtitle && (
              <p className="mt-1 text-sm text-[var(--text-secondary)]">{banner.subtitle}</p>
            )}
          </div>
        </a>
      ))}
    </div>
  );
}
