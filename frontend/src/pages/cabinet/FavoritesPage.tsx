import { useQuery } from '@tanstack/react-query';

import { carsApi, errorMessage } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import { EmptyState, LinkButton, LotCard, PageHeader, Spinner } from '@/ui';

export function FavoritesPage() {
  const list = useQuery({
    queryKey: queryKeys.catalog({ favorites: true, limit: 48 }),
    queryFn: ({ signal }) => carsApi.list({ favorites: true, limit: 48 }, signal),
  });

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Клиент"
        title="Избранное"
        description="Лоты, которые вы отметили сердечком в каталоге. Отсюда удобно сравнить и оставить заявку."
      />
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.isError && (
        <EmptyState title="Не удалось загрузить избранное" description={errorMessage(list.error)} />
      )}
      {list.data && list.data.items.length === 0 && (
        <EmptyState
          title="Пока пусто"
          description="Отметка ставится на карточке автомобиля в каталоге."
          action={<LinkButton to="/catalog">Открыть каталог</LinkButton>}
        />
      )}
      {list.data && list.data.items.length > 0 && (
        <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3">
          {list.data.items.map((car) => (
            <LotCard key={car.id} car={car} />
          ))}
        </div>
      )}
    </div>
  );
}
