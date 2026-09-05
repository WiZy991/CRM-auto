import { useState } from 'react';
import { Link } from 'react-router-dom';

import type { CarListItem } from '@/lib/api';

import { Badge, originTitle, originTone } from './Badge';
import { cn } from './cn';

function listingPhotos(car: CarListItem): string[] {
  if (car.photo_urls && car.photo_urls.length > 0) return car.photo_urls;
  if (car.cover_url) return [car.cover_url];
  return [];
}

export function LotCard({ car }: { car: CarListItem }) {
  const photos = listingPhotos(car);
  const [index, setIndex] = useState(0);
  const current = photos[index] ?? photos[0];

  return (
    <Link
      to={`/catalog/${car.id}`}
      onMouseLeave={() => setIndex(0)}
      className={cn(
        'group flex flex-col overflow-hidden rounded-[var(--radius-card)] border border-[var(--border-hairline)]',
        'bg-[var(--surface-raised)] shadow-[var(--shadow-card)] transition-shadow hover:shadow-[var(--shadow-raise)]',
      )}
    >
      <div className="relative aspect-[16/10] overflow-hidden bg-[var(--surface-sunken)]">
        {current ? (
          <img
            src={current}
            alt=""
            className="size-full object-cover transition-transform duration-300 group-hover:scale-[1.03]"
          />
        ) : (
          <div className="flex size-full items-center justify-center bg-[var(--surface-sunken)] text-xs text-[var(--text-muted)]">
            Нет фото
          </div>
        )}

        <div className="absolute top-2 left-2 flex gap-1">
          <Badge tone={originTone(car.origin)}>{originTitle(car.origin)}</Badge>
          {car.steering_right && <Badge>Правый руль</Badge>}
        </div>

        {car.auction_grade && (
          <span className="absolute top-2 right-2 rounded-md bg-[var(--surface-raised)] px-1.5 py-0.5 text-xs">
            Оценка {car.auction_grade}
          </span>
        )}

        {photos.length > 1 && (
          <div
            className="absolute inset-x-0 bottom-0 flex justify-center gap-0.5 bg-gradient-to-t from-ink-900/55 to-transparent px-2 pt-8 pb-2"
            onClick={(event) => event.preventDefault()}
          >
            {photos.map((src, i) => (
              <span
                key={src + String(i)}
                className="flex h-4 w-4 items-center justify-center"
                onMouseEnter={() => setIndex(i)}
              >
                <span
                  className={cn(
                    'h-1.5 rounded-full',
                    i === index ? 'w-3 bg-white' : 'w-1.5 bg-white/50',
                  )}
                />
              </span>
            ))}
          </div>
        )}
      </div>

      <div className="flex flex-1 flex-col gap-3 p-4">
        <div className="flex items-start justify-between gap-3">
          <div>
            <p className="text-xs font-medium text-[var(--text-muted)]">{car.brand}</p>
            <h3 className="text-base leading-tight font-semibold text-balance">
              {car.model}
              {car.generation ? ` ${car.generation}` : ''}
            </h3>
          </div>
          <p className="numeric shrink-0 text-lg font-semibold">{car.year}</p>
        </div>

        <p className="text-xs text-[var(--text-secondary)]">
          {formatMileage(car.mileage_km)}
          {car.power_hp ? ` · ${car.power_hp} л.с.` : ''}
          {car.body ? ` · ${car.body}` : ''}
        </p>

        <div className="mt-auto flex items-end justify-between border-t border-[var(--border-hairline)] pt-3">
          <div>
            <p className="numeric text-lg leading-none font-semibold">{car.price_label}</p>
            {car.turnkey_label && (
              <p className="mt-1 text-2xs text-[var(--text-muted)]">Под ключ {car.turnkey_label}</p>
            )}
          </div>
          {car.photo_count > 1 && (
            <span className="text-2xs text-[var(--text-muted)]">{car.photo_count} фото</span>
          )}
        </div>
      </div>
    </Link>
  );
}

function formatMileage(km: number): string {
  return new Intl.NumberFormat('ru-RU').format(km) + ' км';
}
