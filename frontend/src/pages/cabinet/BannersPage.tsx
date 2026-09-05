import { useEffect, useState, type DragEvent } from 'react';
import { Link } from 'react-router-dom';
import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';

import { bannersApi, errorMessage, uploadsApi } from '@/lib/api';
import type { Banner, BannerPlacement } from '@/lib/api';
import { formatPercent } from '@/lib/format';
import { bannerStatusTone } from '@/lib/status';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  LinkButton,
  Modal,
  PageGuide,
  PageHeader,
  Spinner,
  TextField,
  cn,
  useToast,
} from '@/ui';

const PLACES: readonly {
  id: BannerPlacement;
  page: string;
  href: string;
  where: string;
  hint: string;
}[] = [
  {
    id: 'home_hero',
    page: 'Главная',
    href: '/',
    where: 'Сразу под шапкой',
    hint: 'Первое, что видит гость сайта.',
  },
  {
    id: 'home_inline',
    page: 'Главная',
    href: '/',
    where: 'В ленте лотов',
    hint: 'Между блоками «как купить» и каталогом.',
  },
  {
    id: 'catalog_top',
    page: 'Каталог',
    href: '/catalog',
    where: 'Над фильтрами',
    hint: 'Видно, пока человек выбирает машину.',
  },
  {
    id: 'catalog_sidebar',
    page: 'Каталог',
    href: '/catalog',
    where: 'Справа от сетки',
    hint: 'На широком экране, рядом с объявлениями.',
  },
  {
    id: 'car_page',
    page: 'Карточка лота',
    href: '/catalog',
    where: 'Справа от фото',
    hint: 'Рядом с ценой и заявкой на конкретный автомобиль.',
  },
];

const STATUS_HINT: Record<string, string> = {
  draft: 'Черновик. Сайт его не показывает — нажмите «На проверку».',
  moderation: 'Администратор смотрит объявление. На сайте ещё нет.',
  active: 'Сейчас на сайте.',
  paused: 'Снят с показа. Можно вернуть без новой проверки.',
  rejected: 'Не прошёл проверку. Исправьте и отправьте снова.',
  expired: 'Срок размещения закончился.',
};

export function BannersPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);
  const [preset, setPreset] = useState<BannerPlacement>('home_hero');

  const mine = useQuery({
    queryKey: queryKeys.bannersMine,
    queryFn: ({ signal }) => bannersApi.mine(signal),
  });

  const submit = useMutation({
    mutationFn: (id: string) => bannersApi.submit(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.bannersMine });
      toast.success('Отправлено на проверку. После одобрения баннер появится на сайте.');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const pause = useMutation({
    mutationFn: ({ id, paused }: { id: string; paused: boolean }) => bannersApi.setPaused(id, paused),
    onSuccess: () => void queryClient.invalidateQueries({ queryKey: queryKeys.bannersMine }),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const stats = mine.data?.stats;
  const items = mine.data?.items ?? [];

  function openCreate(placement: BannerPlacement) {
    setPreset(placement);
    setOpen(true);
  }

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        kicker="Реклама"
        title="Баннеры"
        description="Картинка вашей компании на витрине: главная, каталог или карточка автомобиля. Гость кликает — уходит по вашей ссылке. Показы считаются сами."
        actions={
          <Button size="sm" variant="primary" onClick={() => openCreate('home_hero')}>
            Новый баннер
          </Button>
        }
      />

      <PageGuide
        items={[
          { title: 'Место и картинка', text: 'Выберите слот на схеме и загрузите JPEG или PNG.' },
          { title: 'Проверка', text: 'Черновик виден только вам. «На проверку» смотрит администратор.' },
          { title: 'На сайте', text: 'После одобрения баннер видят гости. Пауза снимает его с витрины.' },
        ]}
      />

      <section>
        <div className="mb-3 flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold">Где показывать</h2>
            <p className="mt-0.5 text-sm text-[var(--text-secondary)]">
              Нажмите на место — откроется форма уже с этим слотом.
            </p>
          </div>
          <LinkButton to="/" size="sm">
            Открыть витрину
          </LinkButton>
        </div>
        <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
          {PLACES.map((place) => (
            <button
              key={place.id}
              type="button"
              onClick={() => openCreate(place.id)}
              className="panel p-3 text-left transition-colors hover:border-[var(--accent)]"
            >
              <PlacementSketch id={place.id} />
              <p className="mt-3 text-sm font-semibold">{place.page}</p>
              <p className="text-sm text-[var(--text-secondary)]">{place.where}</p>
              <p className="mt-1 text-xs text-[var(--text-muted)]">{place.hint}</p>
            </button>
          ))}
        </div>
      </section>

      {stats ? (
        <div className="panel flex flex-wrap items-end gap-8 p-5">
          <div>
            <p className="text-sm text-[var(--text-muted)]">Показы на сайте</p>
            <p className="numeric text-xl">{stats.impressions}</p>
          </div>
          <div>
            <p className="text-sm text-[var(--text-muted)]">Клики по ссылке</p>
            <p className="numeric text-xl">{stats.clicks}</p>
          </div>
          <div>
            <p className="text-sm text-[var(--text-muted)]">Доля кликов</p>
            <p className="numeric text-xl">{formatPercent(stats.ctr)}</p>
          </div>
        </div>
      ) : null}

      {mine.isPending && <Spinner className="text-[var(--accent)]" />}

      {mine.data && items.length === 0 ? (
        <p className="text-sm text-[var(--text-secondary)]">
          Пока ничего не размещено. Выберите место выше или нажмите «Новый баннер».
        </p>
      ) : null}

      {items.length > 0 ? (
        <ul className="grid gap-3">
          {items.map((row) => (
            <BannerCard
              key={row.id}
              banner={row}
              onSubmit={() => submit.mutate(row.id)}
              onPause={(paused) => pause.mutate({ id: row.id, paused })}
              busy={submit.isPending || pause.isPending}
            />
          ))}
        </ul>
      ) : null}

      <BannerCreateModal
        open={open}
        placement={preset}
        onClose={() => setOpen(false)}
      />
    </div>
  );
}

function BannerCard({
  banner,
  onSubmit,
  onPause,
  busy,
}: {
  banner: Banner;
  onSubmit: () => void;
  onPause: (paused: boolean) => void;
  busy: boolean;
}) {
  const place = PLACES.find((item) => item.id === banner.placement);
  const hint = banner.reject_reason || STATUS_HINT[banner.status] || banner.status_title;

  return (
    <li className="panel grid gap-4 p-3 sm:grid-cols-[11rem_1fr_auto] sm:items-stretch">
      <div className="aspect-[16/10] overflow-hidden bg-[var(--surface-sunken)] sm:aspect-auto sm:min-h-28">
        {banner.image_url ? (
          <img src={banner.image_url} alt="" className="size-full object-cover" />
        ) : (
          <div className="bg-hatch size-full" />
        )}
      </div>
      <div className="flex min-w-0 flex-col justify-center gap-2">
        <div className="flex flex-wrap items-center gap-2">
          <h3 className="text-base font-semibold">{banner.title}</h3>
          <Badge tone={bannerStatusTone(banner.status)}>{banner.status_title}</Badge>
        </div>
        <p className="text-sm text-[var(--text-secondary)]">
          {place ? (
            <>
              {place.page}
              {' · '}
              {place.where}
              {' · '}
              <Link to={place.href} className="text-[var(--link)] underline underline-offset-2">
                страница на сайте
              </Link>
            </>
          ) : (
            banner.placement_title
          )}
        </p>
        <p className="text-sm text-[var(--text-secondary)]">{hint}</p>
        <p className="numeric text-xs text-[var(--text-muted)]">
          {banner.impressions} показов · {banner.clicks} кликов
        </p>
      </div>
      <div className="flex flex-wrap items-end gap-2 sm:flex-col sm:justify-center sm:items-stretch">
        {banner.status === 'draft' || banner.status === 'rejected' ? (
          <Button size="sm" variant="primary" disabled={busy} onClick={onSubmit}>
            На проверку
          </Button>
        ) : null}
        {banner.status === 'active' ? (
          <Button size="sm" disabled={busy} onClick={() => onPause(true)}>
            Снять с сайта
          </Button>
        ) : null}
        {banner.status === 'paused' ? (
          <Button size="sm" variant="primary" disabled={busy} onClick={() => onPause(false)}>
            Вернуть на сайт
          </Button>
        ) : null}
      </div>
    </li>
  );
}

function PlacementSketch({ id }: { id: BannerPlacement }) {
  const mark = (slot: BannerPlacement) =>
    cn('rounded-[1px]', slot === id ? 'bg-[var(--accent)]' : 'bg-[var(--surface-sunken)]');

  if (id === 'catalog_sidebar') {
    return (
      <div className="grid aspect-[5/4] grid-cols-[1fr_0.45fr] gap-1 border border-[var(--border-hairline)] bg-[var(--surface)] p-1.5">
        <div className="flex flex-col gap-1">
          <div className="h-2 bg-[var(--surface-sunken)]" />
          <div className="grid flex-1 grid-cols-2 gap-1">
            <div className="bg-[var(--surface-sunken)]" />
            <div className="bg-[var(--surface-sunken)]" />
          </div>
        </div>
        <div className={mark('catalog_sidebar')} />
      </div>
    );
  }

  if (id === 'car_page') {
    return (
      <div className="grid aspect-[5/4] grid-cols-[1.2fr_0.8fr] gap-1 border border-[var(--border-hairline)] bg-[var(--surface)] p-1.5">
        <div className="bg-[var(--surface-sunken)]" />
        <div className="flex flex-col gap-1">
          <div className="h-3 bg-[var(--surface-sunken)]" />
          <div className="h-3 bg-[var(--surface-sunken)]" />
          <div className={cn('flex-1', mark('car_page'))} />
        </div>
      </div>
    );
  }

  return (
    <div className="flex aspect-[5/4] flex-col gap-1 border border-[var(--border-hairline)] bg-[var(--surface)] p-1.5">
      <div className="h-2 bg-[var(--surface-sunken)]" />
      {id === 'home_hero' ? <div className={cn('h-7', mark('home_hero'))} /> : <div className="h-4 bg-[var(--surface-sunken)]" />}
      {id === 'catalog_top' ? <div className={cn('h-4', mark('catalog_top'))} /> : null}
      <div className="grid flex-1 grid-cols-3 gap-1">
        <div className="bg-[var(--surface-sunken)]" />
        <div className="bg-[var(--surface-sunken)]" />
        <div className="bg-[var(--surface-sunken)]" />
      </div>
      {id === 'home_inline' ? <div className={cn('h-4', mark('home_inline'))} /> : null}
    </div>
  );
}

function BannerCreateModal({
  open,
  onClose,
  placement,
}: {
  open: boolean;
  onClose: () => void;
  placement: BannerPlacement;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState('');
  const [image, setImage] = useState('');
  const [url, setUrl] = useState('');
  const [slot, setSlot] = useState<BannerPlacement>(placement);
  const [uploading, setUploading] = useState(false);
  const [dragOver, setDragOver] = useState(false);
  const starts = new Date().toISOString();
  const ends = new Date(Date.now() + 30 * 86400000).toISOString();
  const current = PLACES.find((item) => item.id === slot) ?? {
    id: 'home_hero' as const,
    page: 'Главная',
    href: '/',
    where: 'Сразу под шапкой',
    hint: 'Первое, что видит гость сайта.',
  };

  useEffect(() => {
    if (open) setSlot(placement);
  }, [open, placement]);

  function resetAndClose() {
    setTitle('');
    setImage('');
    setUrl('');
    setDragOver(false);
    onClose();
  }

  function upload(file: File) {
    setUploading(true);
    void uploadsApi
      .image(file)
      .then((saved) => {
        setImage(saved.url);
        toast.success('Картинка загружена');
      })
      .catch((error: unknown) => toast.error(errorMessage(error)))
      .finally(() => setUploading(false));
  }

  function onDrop(event: DragEvent<HTMLLabelElement>) {
    event.preventDefault();
    setDragOver(false);
    const file = event.dataTransfer.files[0];
    if (file) upload(file);
  }

  const create = useMutation({
    mutationFn: () =>
      bannersApi.create({
        placement: slot,
        title,
        image_url: image,
        target_url: url,
        starts_at: starts,
        ends_at: ends,
        cta_label: 'Подробнее',
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.bannersMine });
      toast.success('Черновик сохранён. Отправьте его на проверку в списке ниже.');
      resetAndClose();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <Modal
      open={open}
      onClose={resetAndClose}
      title="Новый баннер"
      wide
      footer={
        <>
          <Button onClick={resetAndClose}>Отмена</Button>
          <Button
            variant="primary"
            loading={create.isPending}
            disabled={!image || uploading}
            onClick={() => create.mutate()}
          >
            Сохранить черновик
          </Button>
        </>
      }
    >
      <div className="grid gap-6 md:grid-cols-2">
        <div className="flex flex-col gap-3">
          <p className="text-[13px] font-medium text-[var(--text-secondary)]">Место на сайте</p>
          <div className="grid grid-cols-2 gap-2">
            {PLACES.map((place) => (
              <button
                key={place.id}
                type="button"
                onClick={() => setSlot(place.id)}
                className={cn(
                  'border px-2 py-2 text-left text-xs transition-colors',
                  slot === place.id
                    ? 'border-[var(--accent)] bg-[var(--surface-sunken)]'
                    : 'border-[var(--border-hairline)] hover:border-[var(--text-muted)]',
                )}
              >
                <span className="block font-medium">{place.page}</span>
                <span className="text-[var(--text-secondary)]">{place.where}</span>
              </button>
            ))}
          </div>
          <TextField label="Заголовок на баннере" value={title} onChange={(event) => setTitle(event.target.value)} />
          <label
            onDragOver={(event) => {
              event.preventDefault();
              setDragOver(true);
            }}
            onDragLeave={() => setDragOver(false)}
            onDrop={onDrop}
            className={cn(
              'cursor-pointer border border-dashed px-3 py-6 text-center transition-colors',
              dragOver || uploading
                ? 'border-[var(--accent)] bg-[var(--surface-sunken)]'
                : 'border-[var(--border-hairline)] hover:border-[var(--text-muted)]',
            )}
          >
            <input
              type="file"
              accept="image/jpeg,image/png"
              className="sr-only"
              disabled={uploading}
              onChange={(event) => {
                const file = event.target.files?.[0];
                event.target.value = '';
                if (file) upload(file);
              }}
            />
            <span className="text-sm font-medium">
              {uploading ? 'Загрузка…' : 'Перетащите картинку или нажмите, чтобы выбрать'}
            </span>
            <span className="mt-1 block text-xs text-[var(--text-muted)]">JPEG или PNG</span>
          </label>
          <TextField
            label="Куда вести по клику"
            hint="Адрес сайта, лота или мессенджера"
            value={url}
            onChange={(event) => setUrl(event.target.value)}
          />
        </div>
        <div>
          <p className="text-[13px] font-medium text-[var(--text-secondary)]">Как это выглядит</p>
          <p className="mt-1 text-sm text-[var(--text-secondary)]">
            {current.page}, {current.where.toLowerCase()}. {current.hint}
          </p>
          <div className="mt-3 border border-[var(--border-hairline)]">
            <PlacementSketch id={slot} />
            <div className="grid overflow-hidden border-t border-[var(--border-hairline)] md:grid-cols-[1.2fr_1fr]">
              <div className="min-h-28 bg-[var(--surface-sunken)]">
                {image ? <img src={image} alt="" className="size-full max-h-40 object-cover" /> : <div className="bg-hatch size-full min-h-28" />}
              </div>
              <div className="flex flex-col justify-end p-4">
                <p className="text-xs font-medium text-[var(--accent)]">Подробнее</p>
                <p className="mt-1 text-base font-semibold">{title || 'Заголовок баннера'}</p>
              </div>
            </div>
          </div>
        </div>
      </div>
    </Modal>
  );
}
