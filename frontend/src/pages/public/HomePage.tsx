import { useQuery } from '@tanstack/react-query';
import { Link } from 'react-router-dom';

import { bannersApi, carsApi, dealersApi } from '@/lib/api';
import { dealerCityLabel } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { BannerSlot, LinkButton, LotCard } from '@/ui';

const STEPS = [
  {
    n: '01',
    title: 'Заявка',
    text: 'Опишите, что ищете: страну, бюджет, кузов. Можно сразу по объявлению из каталога.',
  },
  {
    n: '02',
    title: 'Подбор и договор',
    text: 'Дилер находит лот на аукционе или у экспортёра, фиксирует цену и сроки в сделке.',
  },
  {
    n: '03',
    title: 'Оплата и путь',
    text: 'Оплата, отгрузка, растаможка и выдача — семь этапов, каждый с датой и документами.',
  },
] as const;

export function HomePage() {
  const hero = useQuery({
    queryKey: queryKeys.bannersActive('home_hero'),
    queryFn: ({ signal }) => bannersApi.active('home_hero', 1, signal),
  });
  const inline = useQuery({
    queryKey: queryKeys.bannersActive('home_inline'),
    queryFn: ({ signal }) => bannersApi.active('home_inline', 2, signal),
  });
  const dealers = useQuery({
    queryKey: queryKeys.dealers({ limit: 6 }),
    queryFn: ({ signal }) => dealersApi.list({ limit: 6 }, signal),
  });
  const lots = useQuery({
    queryKey: queryKeys.catalog({ limit: 6, sort: 'fresh' }),
    queryFn: ({ signal }) => carsApi.list({ limit: 6, sort: 'fresh' }, signal),
  });

  return (
    <div>
      <section className="border-b border-[var(--border-hairline)]">
        <div className="mx-auto grid w-full max-w-[1600px] gap-10 px-4 py-12 sm:px-6 md:grid-cols-[1.4fr_1fr] md:py-20 lg:px-8">
          <div>
            <p className="text-sm font-medium text-[var(--accent)]">
              Китай · Япония · под ключ
            </p>
            <h1 className="mt-3 max-w-xl text-3xl leading-[1.15] font-semibold text-balance md:text-5xl">
              Автомобиль с аукциона — без сюрпризов на выдаче
            </h1>
            <p className="mt-5 max-w-lg text-base text-[var(--text-secondary)]">
              Площадка для частных покупателей и дилеров-импортёров. Каталог лотов, база
              поставщиков и сделка, в которой видны деньги, сроки и документы.
            </p>
            <div className="mt-8 flex flex-wrap gap-3">
              <LinkButton to="/catalog" variant="primary" size="lg">
                Смотреть каталог
              </LinkButton>
              <LinkButton to="/register" size="lg">
                Оставить заявку
              </LinkButton>
            </div>
          </div>

          <aside className="panel p-6">
            <p className="text-sm font-medium text-[var(--text-muted)]">
              Сделка на виду
            </p>
            <ul className="mt-4 space-y-3 text-sm">
              <li className="flex justify-between border-b border-[var(--border-hairline)] pb-3">
                <span className="text-[var(--text-secondary)]">Этапов воронки</span>
                <span className="numeric font-medium">7</span>
              </li>
              <li className="flex justify-between border-b border-[var(--border-hairline)] pb-3">
                <span className="text-[var(--text-secondary)]">Рынки</span>
                <span className="font-medium">CN и JP</span>
              </li>
              <li className="flex justify-between">
                <span className="text-[var(--text-secondary)]">Участники</span>
                <span className="font-medium">Клиент, дилер, продавец</span>
              </li>
            </ul>
          </aside>
        </div>
      </section>

      <section className="border-b border-[var(--border-hairline)]">
        <div className="mx-auto grid w-full max-w-[1600px] gap-3 px-4 py-8 sm:grid-cols-3 sm:px-6 lg:px-8">
          <Link to="/catalog" className="panel px-5 py-6">
            <p className="text-sm font-medium text-[var(--accent)]">Покупатель</p>
            <p className="mt-2 text-base font-semibold">Выбрать машину</p>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">Каталог, заявка дилеру, семь этапов до выдачи.</p>
          </Link>
          <Link to="/register" className="panel px-5 py-6">
            <p className="text-sm font-medium text-[var(--accent)]">Дилер</p>
            <p className="mt-2 text-base font-semibold">Вести сделки</p>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">Воронка, лоты, реклама и каналы в кабинете.</p>
          </Link>
          <Link to="/register" className="panel px-5 py-6">
            <p className="text-sm font-medium text-[var(--accent)]">Поставщик</p>
            <p className="mt-2 text-base font-semibold">Попасть в справочник</p>
            <p className="mt-1 text-sm text-[var(--text-secondary)]">Карточка завода или аукциона для дилеров.</p>
          </Link>
        </div>
      </section>

      {hero.data && hero.data.items.length > 0 && (
        <div className="mx-auto w-full max-w-[1600px] px-4 pt-8 sm:px-6 lg:px-8">
          <BannerSlot banners={hero.data.items} />
        </div>
      )}

      <section className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8">
        <h2 className="text-2xl font-semibold">Как проходит покупка</h2>
        <div className="mt-8 grid gap-3 md:grid-cols-3">
          {STEPS.map((step) => (
            <article key={step.n} className="panel p-6">
              <p className="numeric text-sm text-[var(--accent)]">{step.n}</p>
              <h3 className="mt-3 text-lg font-semibold">{step.title}</h3>
              <p className="mt-2 text-sm text-[var(--text-secondary)]">{step.text}</p>
            </article>
          ))}
        </div>
        <p className="mt-6 text-sm">
          <Link to="/how-it-works" className="text-[var(--link)] underline underline-offset-4">
            Семь этапов воронки подробно
          </Link>
        </p>
      </section>

      {lots.data && lots.data.items.length > 0 && (
        <section className="border-t border-[var(--border-hairline)]">
          <div className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8">
            <div className="flex flex-wrap items-end justify-between gap-4">
              <div>
                <p className="text-sm font-medium text-[var(--accent)]">Каталог</p>
                <h2 className="mt-2 text-2xl font-semibold">Доступные автомобили</h2>
              </div>
              <Link to="/catalog" className="text-sm text-[var(--link)] underline underline-offset-4">
                Все лоты
              </Link>
            </div>
            <div className="mt-8 grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
              {lots.data.items.map((car) => (
                <LotCard key={car.id} car={car} />
              ))}
            </div>
          </div>
        </section>
      )}

      <section className="border-t border-[var(--border-hairline)]">
        <div className="mx-auto w-full max-w-[1600px] px-4 py-16 sm:px-6 lg:px-8">
          <div className="flex flex-wrap items-end justify-between gap-4">
            <div>
              <p className="text-sm font-medium text-[var(--accent)]">Сеть</p>
              <h2 className="mt-2 text-2xl font-semibold">Дилеры-импортёры</h2>
            </div>
            <Link to="/dealers" className="text-sm text-[var(--link)] underline underline-offset-4">
              Все импортёры
            </Link>
          </div>
          {dealers.data && dealers.data.items.length === 0 && (
            <p className="mt-6 text-sm text-[var(--text-secondary)]">
              Пока нет активных карточек. После регистрации дилера учётка появляется в каталоге сразу.
            </p>
          )}
          {dealers.data && dealers.data.items.length > 0 && (
            <ul className="mt-8 grid gap-3 sm:grid-cols-3">
              {dealers.data.items.map((dealer) => (
                <li key={dealer.slug} className="panel p-5">
                  <Link to={`/dealers/${dealer.slug}`} className="block">
                    <p className="text-sm text-[var(--text-muted)]">
                      {dealerCityLabel(dealer.city)}
                    </p>
                    <h3 className="mt-2 text-lg font-semibold">{dealer.company_name}</h3>
                    <p className="numeric mt-3 text-xs text-[var(--text-muted)]">
                      {dealer.deals_won} выдач · {dealer.cars_active} лотов
                    </p>
                  </Link>
                </li>
              ))}
            </ul>
          )}
        </div>
      </section>

      {inline.data && inline.data.items.length > 0 && (
        <div className="mx-auto w-full max-w-[1600px] px-4 pb-16 sm:px-6 lg:px-8">
          <BannerSlot banners={inline.data.items} compact />
        </div>
      )}
    </div>
  );
}
