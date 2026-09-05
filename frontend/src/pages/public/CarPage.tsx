import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState, type FormEvent } from 'react';
import { Link, useParams } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { bannersApi, carsApi, errorMessage, requestsApi } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  BannerSlot,
  Button,
  EmptyState,
  FullPageSpinner,
  LinkButton,
  originTitle,
  originTone,
  PhotoLightbox,
  TextAreaField,
  TextField,
  useToast,
} from '@/ui';

export function CarPage() {
  const { id = '' } = useParams();
  const { status, user, hasRole } = useAuth();
  const toast = useToast();
  const queryClient = useQueryClient();

  const carQuery = useQuery({
    queryKey: queryKeys.car(id),
    queryFn: ({ signal }) => carsApi.get(id, signal),
    enabled: Boolean(id),
  });

  const pageBanner = useQuery({
    queryKey: queryKeys.bannersActive('car_page'),
    queryFn: ({ signal }) => bannersApi.active('car_page', 1, signal),
  });

  const favorite = useMutation({
    mutationFn: (next: boolean) => carsApi.setFavorite(id, next),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.car(id) });
      void queryClient.invalidateQueries({ queryKey: ['cars'] });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (carQuery.isPending) return <FullPageSpinner label="Загружаем карточку" />;

  if (carQuery.isError || !carQuery.data) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-16">
        <EmptyState
          title="Объявление не найдено"
          description="Ссылка устарела или лот снят с публикации."
          action={<LinkButton to="/catalog">В каталог</LinkButton>}
        />
      </div>
    );
  }

  const car = carQuery.data.car;
  const canRequest = status === 'authenticated' && hasRole('client');

  return (
    <article className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8">
      <p className="text-sm text-[var(--text-muted)]">
        <Link to="/catalog" className="hover:text-[var(--text-primary)]">
          Каталог
        </Link>
        {' · '}
        {car.brand}
      </p>

      <div className="mt-3 flex flex-col gap-3 sm:flex-row sm:items-start sm:justify-between">
        <div>
          <h1 className="text-2xl font-semibold text-balance md:text-3xl">{car.display_name}</h1>
          <div className="mt-3 flex flex-wrap gap-1.5">
            <Badge tone={originTone(car.origin)}>{originTitle(car.origin)}</Badge>
            {car.steering_right && <Badge>Правый руль</Badge>}
            {car.auction_grade && <Badge tone="harbor">Оценка {car.auction_grade}</Badge>}
            {car.interior_grade && <Badge>Салон {car.interior_grade}</Badge>}
          </div>
        </div>
        <div className="text-left sm:text-right">
          <p className="numeric text-2xl font-semibold md:text-3xl">{car.price_label}</p>
          {car.turnkey_label && (
            <p className="mt-1 text-sm text-[var(--text-muted)]">Под ключ {car.turnkey_label}</p>
          )}
        </div>
      </div>

      <div className="mt-8 grid gap-8 lg:grid-cols-[1.4fr_1fr]">
        <div>
          <CarGallery
            photos={car.photos}
            title={car.display_name}
            {...(car.cover_url ? { coverUrl: car.cover_url } : {})}
          />

          {car.description && (
            <div className="mt-8">
              <h2 className="text-lg font-semibold">Описание</h2>
              <p className="mt-3 whitespace-pre-wrap text-sm leading-relaxed text-[var(--text-secondary)]">
                {car.description}
              </p>
            </div>
          )}

          {car.equipment.length > 0 && (
            <div className="mt-8">
              <h2 className="text-lg font-semibold">Комплектация</h2>
              <ul className="mt-3 grid gap-1 text-sm text-[var(--text-secondary)] sm:grid-cols-2">
                {car.equipment.map((item) => (
                  <li key={item}>{item}</li>
                ))}
              </ul>
            </div>
          )}
        </div>

        <aside className="flex flex-col gap-4">
          <div className="border border-[var(--border-hairline)] bg-[var(--surface-raised)] p-5">
            <dl className="space-y-3 text-sm">
              <Row label="Год" value={String(car.year)} />
              <Row
                label="Пробег"
                value={new Intl.NumberFormat('ru-RU').format(car.mileage_km) + ' км'}
              />
              {car.power_hp ? <Row label="Мощность" value={`${car.power_hp} л.с.`} /> : null}
              {car.engine_cc ? <Row label="Объём" value={`${car.engine_cc} см³`} /> : null}
              {car.vin ? <Row label="VIN" value={car.vin} /> : null}
              {car.delivery_days ? (
                <Row label="Срок доставки" value={`${car.delivery_days} дн.`} />
              ) : null}
            </dl>

            {user && (
              <Button
                className="mt-4"
                block
                loading={favorite.isPending}
                onClick={() => favorite.mutate(!car.is_favorite)}
              >
                {car.is_favorite ? 'Убрать из избранного' : 'В избранное'}
              </Button>
            )}
          </div>

          {canRequest ? (
            <RequestForm carId={car.id} />
          ) : (
            <LinkButton to={user ? '/app' : `/register?car=${car.id}`} variant="primary" block>
              {user ? 'Заявки доступны покупателю' : 'Оставить заявку'}
            </LinkButton>
          )}

          {pageBanner.data && pageBanner.data.items.length > 0 ? (
            <BannerSlot banners={pageBanner.data.items} compact />
          ) : null}
        </aside>
      </div>
    </article>
  );
}

function CarGallery({
  photos,
  coverUrl,
  title,
}: {
  photos: readonly { id: string; url: string; thumb_url?: string }[];
  coverUrl?: string;
  title: string;
}) {
  const urls = photos.length > 0 ? photos.map((item) => item.url) : coverUrl ? [coverUrl] : [];
  const [index, setIndex] = useState(0);
  const [open, setOpen] = useState(false);
  const current = urls[index] ?? urls[0];

  return (
    <>
      <button
        type="button"
        className="aspect-[16/10] w-full overflow-hidden rounded-[var(--radius-card)] border border-[var(--border-hairline)] bg-[var(--surface-sunken)]"
        onClick={() => {
          if (current) setOpen(true);
        }}
        disabled={!current}
      >
        {current ? (
          <img src={current} alt={title} className="size-full object-cover" />
        ) : (
          <div className="flex size-full items-center justify-center text-sm text-[var(--text-muted)]">
            Нет фото
          </div>
        )}
      </button>

      {urls.length > 1 && (
        <ul className="mt-2 grid grid-cols-5 gap-1 sm:grid-cols-6">
          {photos.map((photo, i) => (
            <li key={photo.id}>
              <button
                type="button"
                className={
                  i === index
                    ? 'aspect-[4/3] w-full overflow-hidden rounded-[var(--radius-sheet)] ring-2 ring-[var(--accent)]'
                    : 'aspect-[4/3] w-full overflow-hidden rounded-[var(--radius-sheet)]'
                }
                onClick={() => setIndex(i)}
                onDoubleClick={() => {
                  setIndex(i);
                  setOpen(true);
                }}
              >
                <img src={photo.thumb_url ?? photo.url} alt="" className="size-full object-cover" />
              </button>
            </li>
          ))}
        </ul>
      )}

      <PhotoLightbox
        photos={urls}
        index={index}
        open={open}
        title={title}
        onClose={() => setOpen(false)}
        onIndex={setIndex}
      />
    </>
  );
}

function RequestForm({ carId }: { carId: string }) {
  const toast = useToast();
  const [comment, setComment] = useState('');
  const [budget, setBudget] = useState('');

  const create = useMutation({
    mutationFn: () =>
      requestsApi.create({
        car_id: carId,
        comment,
        ...(budget ? { budget_to_rub: Number(budget) } : {}),
      }),
    onSuccess: () => {
      toast.success('Заявка в общем пуле', 'Любой дилер может взять её во вкладке «Пул».');
      setComment('');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    create.mutate();
  }

  return (
    <form
      onSubmit={(event) => void onSubmit(event)}
      className="panel flex flex-col gap-3 p-5"
    >
      <p className="text-sm font-medium text-[var(--accent)]">Заявка по лоту</p>
      <TextField
        label="Бюджет, ₽"
        numeric
        inputMode="numeric"
        value={budget}
        onChange={(event) => setBudget(event.target.value)}
      />
      <TextAreaField
        label="Комментарий"
        rows={3}
        value={comment}
        onChange={(event) => setComment(event.target.value)}
      />
      <Button type="submit" variant="primary" loading={create.isPending}>
        Отправить заявку
      </Button>
    </form>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between gap-4 border-b border-[var(--border-hairline)] pb-3">
      <dt className="text-[var(--text-muted)]">{label}</dt>
      <dd className="numeric font-medium">{value}</dd>
    </div>
  );
}
