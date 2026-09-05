import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, useParams } from 'react-router-dom';
import { useState, type FormEvent } from 'react';

import { useAuth } from '@/features/auth/auth-context';
import { dealersApi, errorMessage, requestsApi, sellersApi } from '@/lib/api';
import type { PublicDealer, Seller } from '@/lib/api';
import { dealerCityLabel } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  Combobox,
  EmptyState,
  LinkButton,
  LotCard,
  originTitle,
  originTone,
  SelectField,
  Spinner,
  TextAreaField,
  TextField,
  useToast,
} from '@/ui';

export function DealersCatalog({ items }: { items: readonly PublicDealer[] }) {
  if (items.length === 0) {
    return (
      <EmptyState
        className="mt-8"
        title="Каталог дилеров пуст"
        description="Когда появится активный импортёр, он отобразится здесь. После регистрации дилера войдите ещё раз — карточка создаётся автоматически."
        action={<LinkButton to="/register">Стать дилером</LinkButton>}
      />
    );
  }

  return (
    <ul className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
      {items.map((dealer) => (
        <li key={dealer.slug} className="panel p-5">
          <Link to={`/dealers/${dealer.slug}`} className="block">
            <p className="text-sm text-[var(--text-muted)]">
              {dealerCityLabel(dealer.city)}
              {dealer.verified ? ' · проверен' : ''}
            </p>
            <h2 className="mt-2 text-lg font-semibold">{dealer.company_name}</h2>
            <p className="mt-2 line-clamp-3 text-sm text-[var(--text-secondary)]">
              {dealer.description || 'Импорт автомобилей из Китая и Японии.'}
            </p>
            <p className="numeric mt-4 text-xs text-[var(--text-muted)]">
              {dealer.deals_won} выдач · {dealer.cars_active} лотов
              {dealer.rating_count > 0 ? ` · ${dealer.rating_avg.toFixed(1)}` : ''}
            </p>
          </Link>
        </li>
      ))}
    </ul>
  );
}

export function DealersPage() {
  const list = useQuery({
    queryKey: queryKeys.dealers({ limit: 48 }),
    queryFn: ({ signal }) => dealersApi.list({ limit: 48 }, signal),
  });

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8">
      <p className="text-sm font-medium text-[var(--accent)]">Сеть</p>
      <h1 className="mt-2 text-2xl font-semibold md:text-3xl">Дилеры-импортёры</h1>
      <p className="mt-3 max-w-2xl text-sm text-[var(--text-secondary)]">
        Российские компании, которые ведут сделки на площадке: подбор, выкуп, доставка и
        растаможка.
      </p>

      {list.isPending && (
        <div className="mt-10 flex justify-center">
          <Spinner className="text-[var(--accent)]" />
        </div>
      )}
      {list.isError && (
        <EmptyState className="mt-8" title="Не удалось загрузить" description={errorMessage(list.error)} />
      )}
      {list.data && <DealersCatalog items={list.data.items} />}
    </div>
  );
}

export function DealerPublicPage() {
  const { slug = '' } = useParams();
  const { status, hasRole } = useAuth();
  const dealer = useQuery({
    queryKey: queryKeys.dealer(slug),
    queryFn: ({ signal }) => dealersApi.get(slug, signal),
    enabled: slug.length > 0,
  });
  const cars = useQuery({
    queryKey: queryKeys.dealerCars(slug),
    queryFn: ({ signal }) => dealersApi.cars(slug, signal),
    enabled: slug.length > 0,
  });
  const reviews = useQuery({
    queryKey: queryKeys.dealerReviews(slug),
    queryFn: ({ signal }) => dealersApi.reviews(slug, signal),
    enabled: slug.length > 0,
  });

  if (dealer.isPending) {
    return (
      <div className="flex justify-center py-20">
        <Spinner className="text-[var(--accent)]" />
      </div>
    );
  }
  if (dealer.isError || !dealer.data) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-10">
        <EmptyState title="Дилер не найден" action={<LinkButton to="/dealers">К списку</LinkButton>} />
      </div>
    );
  }

  const item = dealer.data.dealer;
  const canRequest = status === 'authenticated' && hasRole('client');

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8">
      <div className="grid gap-8 lg:grid-cols-[1.4fr_1fr]">
        <div>
          <p className="text-sm text-[var(--text-muted)]">
            {dealerCityLabel(item.city)}
            {item.verified ? ' · проверен' : ''}
          </p>
          <h1 className="mt-2 text-2xl font-semibold md:text-3xl">{item.company_name}</h1>
          <p className="mt-3 max-w-2xl text-sm text-[var(--text-secondary)]">
            {item.description || 'Импорт автомобилей из Китая и Японии.'}
          </p>
          <p className="numeric mt-4 text-sm text-[var(--text-muted)]">
            {item.deals_won} выдач · {item.deals_total} сделок · {item.cars_active} лотов в каталоге
          </p>
          {item.services.length > 0 && (
            <ul className="mt-4 flex flex-wrap gap-2">
              {item.services.map((service) => (
                <li
                  key={service}
                  className="border border-[var(--border-hairline)] px-2 py-1 text-2xs tracking-[0.08em] uppercase"
                >
                  {service}
                </li>
              ))}
            </ul>
          )}
        </div>
        <aside>
          {canRequest ? (
            <DealerRequestForm dealerId={item.user_id} company={item.company_name} />
          ) : (
            <LinkButton to={status === 'authenticated' ? '/app' : '/register'} variant="primary" block>
              {status === 'authenticated' ? 'Заявку оставляет покупатель' : 'Оставить заявку этому дилеру'}
            </LinkButton>
          )}
        </aside>
      </div>

      <h2 className="mt-10 text-lg font-semibold">Отзывы</h2>
      {reviews.data && reviews.data.items.length === 0 && (
        <p className="mt-3 text-sm text-[var(--text-muted)]">Отзывов пока нет. Их оставляют после выдачи автомобиля.</p>
      )}
      {reviews.data && reviews.data.items.length > 0 && (
        <ul className="mt-4 divide-y divide-[var(--border-hairline)] border border-[var(--border-hairline)]">
          {reviews.data.items.map((item) => (
            <li key={item.id} className="px-4 py-3">
              <p className="text-sm font-medium">
                {item.author_name || 'Клиент'} · {item.rating} из 5
              </p>
              {item.text && <p className="mt-1 text-sm text-[var(--text-secondary)]">{item.text}</p>}
              {item.dealer_reply && (
                <p className="mt-2 text-xs text-[var(--text-muted)]">Ответ дилера: {item.dealer_reply}</p>
              )}
            </li>
          ))}
        </ul>
      )}

      <h2 className="mt-10 text-lg font-semibold">Лоты</h2>
      {cars.data && cars.data.items.length === 0 && (
        <EmptyState className="mt-4" title="Сейчас нет открытых лотов" />
      )}
      {cars.data && cars.data.items.length > 0 && (
        <div className="mt-4 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {cars.data.items.map((car) => (
            <LotCard key={car.id} car={car} />
          ))}
        </div>
      )}
    </div>
  );
}

function DealerRequestForm({ dealerId, company }: { dealerId: string; company: string }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [brand, setBrand] = useState('');
  const [model, setModel] = useState('');
  const [comment, setComment] = useState('');

  const create = useMutation({
    mutationFn: () =>
      requestsApi.create({
        dealer_id: dealerId,
        ...(brand ? { desired_brand: brand } : {}),
        ...(model ? { desired_model: model } : {}),
        ...(comment ? { comment } : {}),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Заявка отправлена', `${company} увидит её в своём списке.`);
      setBrand('');
      setModel('');
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
      <p className="text-sm font-medium text-[var(--accent)]">Заявка дилеру</p>
      <TextField label="Марка" value={brand} onChange={(event) => setBrand(event.target.value)} />
      <TextField label="Модель" value={model} onChange={(event) => setModel(event.target.value)} />
      <TextAreaField
        label="Что нужно"
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

export function SellersPublicPage() {
  const { status, hasRole } = useAuth();
  const [country, setCountry] = useState('');
  const [kind, setKind] = useState('');
  const [region, setRegion] = useState('');
  const [brand, setBrand] = useState('');
  const [query, setQuery] = useState('');

  const filters = {
    ...(country ? { country: [country] } : {}),
    ...(kind ? { kind: [kind] } : {}),
    ...(region ? { region: [region] } : {}),
    ...(brand ? { brand: [brand] } : {}),
    ...(query ? { q: query } : {}),
    limit: 48,
  };

  const list = useQuery({
    queryKey: queryKeys.sellerDirectory(filters),
    queryFn: ({ signal }) => sellersApi.directory(filters, signal),
  });
  const facets = useQuery({
    queryKey: queryKeys.sellerFacets(country ? [country] : []),
    queryFn: ({ signal }) => sellersApi.directoryFacets(country ? [country] : undefined, signal),
  });
  const brandOptions = (facets.data?.brands ?? []).map((item) => ({
    value: item.brand,
    title: item.brand,
  }));

  return (
    <div className="mx-auto w-full max-w-[1600px] px-4 py-10 sm:px-6 lg:px-8">
      <p className="text-sm font-medium text-[var(--accent)]">Справочник</p>
      <h1 className="mt-2 text-2xl font-semibold md:text-3xl">Поставщики Китая и Японии</h1>
      <p className="mt-3 max-w-2xl text-sm text-[var(--text-secondary)]">
        Аукционы, экспортёры, заводы и брокеры. Контакты открываются только дилеру после входа.
      </p>

      <div className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
        <SelectField
          label="Страна"
          placeholder="Все"
          value={country}
          onChange={(event) => setCountry(event.target.value)}
          options={[
            { value: 'cn', title: 'Китай' },
            { value: 'jp', title: 'Япония' },
          ]}
        />
        <SelectField
          label="Тип"
          placeholder="Все"
          value={kind}
          onChange={(event) => setKind(event.target.value)}
          options={[
            { value: 'auction', title: 'Аукцион' },
            { value: 'exporter', title: 'Экспортёр' },
            { value: 'dealership', title: 'Автосалон' },
            { value: 'factory', title: 'Завод' },
            { value: 'broker', title: 'Брокер' },
          ]}
        />
        <Combobox
          label="Марка"
          value={brand}
          onChange={setBrand}
          options={brandOptions}
          emptyTitle="Все марки"
        />
        <TextField label="Регион" value={region} onChange={(event) => setRegion(event.target.value)} />
        <TextField label="Поиск" value={query} onChange={(event) => setQuery(event.target.value)} />
      </div>

      {list.isPending && (
        <div className="mt-10 flex justify-center">
          <Spinner className="text-[var(--accent)]" />
        </div>
      )}
      {list.isError && (
        <EmptyState className="mt-8" title="Не удалось загрузить" description={errorMessage(list.error)} />
      )}
      {list.data && list.data.items.length === 0 && (
        <EmptyState
          className="mt-8"
          title="Пока никого нет"
          description="Карточки появятся, когда продавец зарегистрируется или дилер занесёт поставщика."
        />
      )}
      {list.data && list.data.items.length > 0 && (
        <ul className="mt-8 grid gap-3 sm:grid-cols-2 lg:grid-cols-3">
          {list.data.items.map((seller) => (
            <li key={seller.id} className="panel p-5">
              <Link to={`/sellers/${seller.id}`} className="block">
                <p className="text-sm text-[var(--text-muted)]">
                  {seller.country_title} · {seller.kind_title}
                </p>
                <h2 className="mt-2 text-lg font-semibold">{seller.display_name}</h2>
                <p className="mt-2 text-sm text-[var(--text-secondary)]">{seller.region}</p>
                <p className="mt-3 text-xs text-[var(--text-muted)]">
                  {seller.brands.slice(0, 4).join(', ') || 'марки не указаны'}
                </p>
              </Link>
            </li>
          ))}
        </ul>
      )}
      {status === 'authenticated' && hasRole('dealer') ? (
        <p className="mt-8 text-sm">
          <Link to="/app/sellers" className="text-[var(--link)] underline underline-offset-2">
            Открыть полную базу с контактами
          </Link>
        </p>
      ) : (
        <p className="mt-8 text-sm text-[var(--text-muted)]">
          Телефоны и WeChat — в кабинете дилера.{' '}
          <Link to="/login" className="text-[var(--link)] underline underline-offset-2">
            Войти
          </Link>
        </p>
      )}
    </div>
  );
}

export function SellerPublicPage() {
  const { id = '' } = useParams();
  const { status, hasRole } = useAuth();
  const details = useQuery({
    queryKey: queryKeys.sellerPublic(id),
    queryFn: ({ signal }) => sellersApi.directoryGet(id, signal),
    enabled: Boolean(id),
  });

  if (details.isPending) {
    return (
      <div className="flex justify-center py-20">
        <Spinner className="text-[var(--accent)]" />
      </div>
    );
  }
  if (details.isError || !details.data) {
    return (
      <div className="mx-auto max-w-3xl px-4 py-10">
        <EmptyState title="Поставщик не найден" action={<LinkButton to="/sellers">К списку</LinkButton>} />
      </div>
    );
  }

  const seller = details.data.seller;
  return <SellerPublicCard seller={seller} canSeeContacts={status === 'authenticated' && hasRole('dealer', 'admin')} />;
}

function SellerPublicCard({ seller, canSeeContacts }: { seller: Seller; canSeeContacts: boolean }) {
  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      <p className="text-sm text-[var(--text-muted)]">
        <Link to="/sellers" className="hover:text-[var(--text-primary)]">
          Поставщики
        </Link>
        {' · '}
        {seller.country_title}
      </p>
      <div className="mt-3 flex flex-wrap items-center gap-2">
        <h1 className="text-2xl font-semibold md:text-3xl">{seller.display_name}</h1>
        <Badge tone={originTone(seller.country)}>{originTitle(seller.country)}</Badge>
      </div>
      <p className="mt-2 text-sm text-[var(--text-secondary)]">
        {seller.kind_title} · {seller.region}
        {seller.city ? ` · ${seller.city}` : ''}
      </p>
      {seller.description && (
        <p className="mt-6 text-sm leading-relaxed text-[var(--text-secondary)]">{seller.description}</p>
      )}
      <dl className="mt-8 grid gap-3 border border-[var(--border-hairline)] p-5 text-sm sm:grid-cols-2">
        <div>
          <dt className="text-[var(--text-muted)]">Бренды</dt>
          <dd>{seller.brands.join(', ') || '—'}</dd>
        </div>
        <div>
          <dt className="text-[var(--text-muted)]">Мин. партия</dt>
          <dd>{seller.min_order_qty > 0 ? seller.min_order_qty : 'не указана'}</dd>
        </div>
        {canSeeContacts && seller.country === 'cn' ? (
          <div>
            <dt className="text-[var(--text-muted)]">WeChat</dt>
            <dd>{contactOf(seller.contacts, 'wechat') || '—'}</dd>
          </div>
        ) : null}
        {canSeeContacts && seller.country !== 'cn' && (seller.contacts.phone || seller.contacts.tel) ? (
          <div>
            <dt className="text-[var(--text-muted)]">Телефон</dt>
            <dd>{seller.contacts.phone || seller.contacts.tel}</dd>
          </div>
        ) : null}
      </dl>
      {!canSeeContacts && (
        <p className="mt-6 text-sm text-[var(--text-muted)]">
          Контакты скрыты. Их видит дилер после входа.
        </p>
      )}
      <div className="mt-6">
        {canSeeContacts ? (
          <LinkButton to={`/app/sellers/${seller.id}`}>Карточка в кабинете</LinkButton>
        ) : (
          <LinkButton to="/login">Войти как дилер</LinkButton>
        )}
      </div>
    </div>
  );
}

function contactOf(contacts: Record<string, string>, name: string): string {
  const found = Object.entries(contacts).find(([key]) => key.toLowerCase() === name);
  return found?.[1] ?? '';
}

const STAGES = [
  { n: '01', title: 'Лид', text: 'Заявка принята. Дилер связывается с клиентом — первый контакт фиксируется в карточке.' },
  { n: '02', title: 'Потребность', text: 'Согласовываются рынок, бюджет и сроки. К сделке можно привязать лот из каталога.' },
  { n: '03', title: 'Договор', text: 'Фиксируются цена и состав услуг. Договор генерируется или загружается в документы.' },
  { n: '04', title: 'Оплата', text: 'Задаток или полная сумма. Без оплаты или платёжки нельзя перейти к привозу.' },
  { n: '05', title: 'Привоз', text: 'Порт назначения, трекинг и дата прибытия — в карточке сделки.' },
  { n: '06', title: 'Растаможка', text: 'Декларация, пошлины и СБКТС. Для выдачи нужен номер СБКТС или декларация.' },
  { n: '07', title: 'Выдача', text: 'Автомобиль передаётся клиенту, сделка закрывается, можно оставить отзыв.' },
] as const;

export function HowItWorksPage() {
  return (
    <div className="mx-auto max-w-3xl px-4 py-10">
      <p className="text-sm font-medium text-[var(--accent)]">Воронка</p>
      <h1 className="mt-2 text-2xl font-semibold md:text-3xl">Как это работает</h1>
      <p className="mt-3 text-sm text-[var(--text-secondary)]">
        Одна сделка — семь этапов. И покупатель, и дилер видят один и тот же ход: от заявки до выдачи ключей.
      </p>
      <ol className="panel mt-8 divide-y divide-[var(--border-hairline)]">
        {STAGES.map((stage) => (
          <li key={stage.n} className="grid grid-cols-[2.5rem_1fr] gap-3 px-4 py-4 sm:grid-cols-[3rem_1fr] sm:gap-4 sm:px-5 sm:py-5">
            <span className="numeric text-sm text-[var(--text-muted)]">{stage.n}</span>
            <div>
              <h2 className="text-base font-semibold">{stage.title}</h2>
              <p className="mt-1 text-sm text-[var(--text-secondary)]">{stage.text}</p>
            </div>
          </li>
        ))}
      </ol>
    </div>
  );
}
