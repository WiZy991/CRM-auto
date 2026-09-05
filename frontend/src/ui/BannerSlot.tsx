import type { BannerPublic } from '@/lib/api';

import { cn } from './cn';

export function BannerSlot({
  banners,
  compact,
  className,
}: {
  banners: readonly BannerPublic[];
  compact?: boolean;
  className?: string;
}) {
  if (banners.length === 0) return null;

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
            'grid overflow-hidden rounded-[var(--radius-card)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] shadow-[var(--shadow-card)] md:grid-cols-[1.2fr_1fr]',
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
            <p className="text-xs font-medium text-[var(--accent)]">
              {banner.cta_label}
            </p>
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
