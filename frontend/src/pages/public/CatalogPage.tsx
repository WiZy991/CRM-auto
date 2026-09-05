import { useInfiniteQuery, useQuery } from '@tanstack/react-query';
import { useMemo, useState } from 'react';

import { bannersApi, carsApi } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import { BannerSlot, Button, Combobox, EmptyState, LotCard, SelectField, Spinner, TextField } from '@/ui';

const SORTS = [
  { value: 'fresh', title: 'Сначала новые' },
  { value: 'price_asc', title: 'Дешевле' },
  { value: 'price_desc', title: 'Дороже' },
  { value: 'year_desc', title: 'Год выпуска' },
  { value: 'mileage_asc', title: 'Меньше пробег' },
] as const;

export function CatalogPage() {
  const [origin, setOrigin] = useState('');
  const [brand, setBrand] = useState('');
  const [body, setBody] = useState('');
  const [yearFrom, setYearFrom] = useState('');
  const [yearTo, setYearTo] = useState('');
  const [priceTo, setPriceTo] = useState('');
  const [mileageTo, setMileageTo] = useState('');
  const [steering, setSteering] = useState('');
  const [query, setQuery] = useState('');
  const [sort, setSort] = useState('fresh');

  const filters = useMemo(
    () => ({
      ...(origin ? { origin: [origin] } : {}),
      ...(brand ? { brand: [brand] } : {}),
      ...(body ? { body: [body] } : {}),
      ...(yearFrom ? { year_from: Number(yearFrom) } : {}),
      ...(yearTo ? { year_to: Number(yearTo) } : {}),
      ...(priceTo ? { price_to: Number(priceTo) } : {}),
      ...(mileageTo ? { mileage_to: Number(mileageTo) } : {}),
      ...(steering === 'right' ? { steering_right: true } : {}),
      ...(steering === 'left' ? { steering_right: false } : {}),
      ...(query ? { q: query } : {}),
      sort,
      limit: 24,
    }),
    [origin, brand, body, yearFrom, yearTo, priceTo, mileageTo, steering, query, sort],
  );

  const catalog = useInfiniteQuery({
    queryKey: queryKeys.catalog(filters),
    queryFn: ({ signal, pageParam }) =>
      carsApi.list(pageParam ? { ...filters, cursor: pageParam } : filters, signal),
    initialPageParam: '',
    getNextPageParam: (last) => last.next_cursor || undefined,
  });

  const topBanner = useQuery({
    queryKey: queryKeys.bannersActive('catalog_top'),
    queryFn: ({ signal }) => bannersApi.active('catalog_top', 1, signal),
  });
  const sidebarBanner = useQuery({
    queryKey: queryKeys.bannersActive('catalog_sidebar'),
    queryFn: ({ signal }) => bannersApi.active('catalog_sidebar', 2, signal),
  });

  const dictionaries = useQuery({
    queryKey: queryKeys.carDictionaries(origin ? [origin] : []),
    queryFn: ({ signal }) => carsApi.dictionaries(origin ? [origin] : undefined, signal),
  });

  const items = catalog.data?.pages.flatMap((page) => page.items) ?? [];
  const total = catalog.data?.pages[0]?.total;

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8">
      <header className="flex flex-col gap-2 md:flex-row md:items-end md:justify-between">
        <div>
          <p className="text-sm font-medium text-[var(--accent)]">Каталог</p>
          <h1 className="text-2xl font-semibold md:text-3xl">Автомобили из Китая и Японии</h1>
          <p className="mt-2 max-w-2xl text-sm text-[var(--text-secondary)]">
            Откройте карточку, оставьте заявку дилеру. Если лот забронирован — его уже ведут. Сделка с этапами появится в кабинете.
          </p>
        </div>
        {typeof total === 'number' && (
          <p className="numeric text-sm text-[var(--text-muted)]">{total} лотов</p>
        )}
      </header>

      {topBanner.data && topBanner.data.items.length > 0 && (
        <BannerSlot className="mt-6" banners={topBanner.data.items} compact />
      )}

      <div className="mt-8 grid gap-3 md:grid-cols-4">
        <SelectField
          label="Страна"
          placeholder="Все рынки"
          value={origin}
          onChange={(event) => setOrigin(event.target.value)}
          options={[
            { value: 'cn', title: 'Китай' },
            { value: 'jp', title: 'Япония' },
          ]}
        />
        <Combobox
          label="Марка"
          value={brand}
          onChange={setBrand}
          options={dictionaries.data?.brands ?? []}
          emptyTitle="Все марки"
        />
        <TextField
          label="Поиск"
          placeholder="Марка, модель, VIN"
          value={query}
          onChange={(event) => setQuery(event.target.value)}
        />
        <SelectField
          label="Сортировка"
          value={sort}
          onChange={(event) => setSort(event.target.value)}
          options={SORTS}
        />
      </div>
      <div className="mt-3 grid gap-3 sm:grid-cols-2 md:grid-cols-3 lg:grid-cols-6">
        <SelectField
          label="Кузов"
          placeholder="Любой"
          value={body}
          onChange={(event) => setBody(event.target.value)}
          options={dictionaries.data?.dictionaries.body ?? []}
        />
        <TextField
          label="Год от"
          inputMode="numeric"
          value={yearFrom}
          onChange={(event) => setYearFrom(event.target.value)}
        />
        <TextField
          label="Год до"
          inputMode="numeric"
          value={yearTo}
          onChange={(event) => setYearTo(event.target.value)}
        />
        <TextField
          label="Цена до, ₽"
          inputMode="numeric"
          value={priceTo}
          onChange={(event) => setPriceTo(event.target.value)}
        />
        <TextField
          label="Пробег до, км"
          inputMode="numeric"
          value={mileageTo}
          onChange={(event) => setMileageTo(event.target.value)}
        />
        <SelectField
          label="Руль"
          placeholder="Любой"
          value={steering}
          onChange={(event) => setSteering(event.target.value)}
          options={[
            { value: 'left', title: 'Левый' },
            { value: 'right', title: 'Правый' },
          ]}
        />
      </div>

      {sidebarBanner.data && sidebarBanner.data.items.length > 0 ? (
        <BannerSlot className="mt-6" banners={sidebarBanner.data.items} compact />
      ) : null}

      <div className="mt-8">
        {catalog.isPending && (
          <div className="flex justify-center py-16">
            <Spinner size={28} className="text-[var(--accent)]" />
          </div>
        )}

        {catalog.isError && (
          <EmptyState
            title="Каталог сейчас недоступен"
            description="Сервер не ответил. Если база ещё не запущена, список будет пустым до подъёма Postgres."
          />
        )}

        {catalog.data && items.length === 0 && (
          <EmptyState
            title="Нет объявлений по этим условиям"
            description="Снимите фильтры или оставьте заявку на подбор — дилер подберёт лот вне каталога."
          />
        )}

        {items.length > 0 && (
          <>
            <div className="grid grid-cols-2 gap-4 md:grid-cols-3 lg:grid-cols-5">
              {items.map((car) => (
                <LotCard key={car.id} car={car} />
              ))}
            </div>
            {catalog.hasNextPage && (
              <div className="mt-8 flex justify-center">
                <Button
                  loading={catalog.isFetchingNextPage}
                  onClick={() => void catalog.fetchNextPage()}
                >
                  Показать ещё
                </Button>
              </div>
            )}
          </>
        )}
      </div>
    </div>
  );
}
