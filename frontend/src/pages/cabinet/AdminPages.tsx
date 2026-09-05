import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';

import { adminApi, bannersApi, errorMessage, sellersApi } from '@/lib/api';
import type { AdminUserRow } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  PageHeader,
  SelectField,
  Spinner,
  TextField,
  useToast,
} from '@/ui';

export function AdminUsersPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [q, setQ] = useState('');
  const [role, setRole] = useState('');

  const filters = {
    ...(q ? { q } : {}),
    ...(role ? { role: [role] } : {}),
    limit: 50,
  };

  const list = useQuery({
    queryKey: queryKeys.adminUsers(filters),
    queryFn: ({ signal }) => adminApi.users(filters, signal),
  });

  const setStatus = useMutation({
    mutationFn: ({ id, status }: { id: string; status: string }) => adminApi.setUserStatus(id, status),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin'] });
      toast.success('Статус обновлён');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Админка"
        title="Пользователи"
        description="Роли, блокировка и активация. Покупатель, дилер, продавец и администратор."
      />
      <div className="grid gap-3 md:grid-cols-2">
        <TextField label="Поиск" value={q} onChange={(event) => setQ(event.target.value)} />
        <SelectField
          label="Роль"
          placeholder="Все"
          value={role}
          onChange={(event) => setRole(event.target.value)}
          options={[
            { value: 'client', title: 'Клиент' },
            { value: 'dealer', title: 'Дилер' },
            { value: 'seller', title: 'Продавец' },
            { value: 'admin', title: 'Админ' },
          ]}
        />
      </div>
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.data && (
        <DataTable
          columns={[
            { key: 'n', header: 'Имя', render: (row: AdminUserRow) => row.full_name },
            { key: 'e', header: 'Почта', render: (row) => row.email },
            { key: 'r', header: 'Роль', render: (row) => row.role },
            { key: 's', header: 'Статус', render: (row) => <Badge>{row.status}</Badge> },
            {
              key: 'a',
              header: '',
              render: (row) =>
                row.status === 'active' ? (
                  <Button size="sm" variant="danger" onClick={() => setStatus.mutate({ id: row.id, status: 'suspended' })}>
                    Заблокировать
                  </Button>
                ) : row.status === 'pending' ? (
                  <Button size="sm" onClick={() => setStatus.mutate({ id: row.id, status: 'active' })}>
                    Активировать
                  </Button>
                ) : row.status === 'suspended' ? (
                  <Button size="sm" onClick={() => setStatus.mutate({ id: row.id, status: 'active' })}>
                    Разблокировать
                  </Button>
                ) : null,
            },
          ]}
          rows={list.data.items}
          rowKey={(row) => row.id}
          empty={<EmptyState title="Никого не найдено" />}
        />
      )}
    </div>
  );
}

export function AdminModerationPage() {
  const toast = useToast();
  const queryClient = useQueryClient();

  const cars = useQuery({
    queryKey: queryKeys.adminCars({ status: ['moderation'] }),
    queryFn: ({ signal }) => adminApi.cars({ status: ['moderation'], limit: 50 }, signal),
  });
  const banners = useQuery({
    queryKey: queryKeys.bannersPending,
    queryFn: ({ signal }) => bannersApi.pending(signal),
  });
  const sellers = useQuery({
    queryKey: queryKeys.sellers({ verified: false, limit: 50 }),
    queryFn: ({ signal }) => sellersApi.list({ verified: false, limit: 50 }, signal),
  });

  const moderateCar = useMutation({
    mutationFn: ({ id, approve }: { id: string; approve: boolean }) =>
      adminApi.moderateCar(id, approve, approve ? '' : 'Отклонено модератором'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['admin'] });
      toast.success('Объявление обработано');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const moderate = useMutation({
    mutationFn: ({ id, approve }: { id: string; approve: boolean }) =>
      bannersApi.moderate(id, approve, approve ? '' : 'Не соответствует правилам размещения'),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.bannersPending });
      toast.success('Решение записано');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const verify = useMutation({
    mutationFn: (id: string) => adminApi.verifySeller(id, true),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['sellers'] });
      toast.success('Поставщик отмечен как проверенный');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        kicker="Админка"
        title="Модерация"
        description="Очередь на витрину: объявления дилеров, рекламные баннеры и непроверенные поставщики. Одобренное сразу видно гостям сайта."
      />
      <section>
        <h2 className="mb-3 text-sm font-medium">Объявления</h2>
        {cars.isPending && <Spinner className="text-[var(--accent)]" />}
        {cars.data && (
          <DataTable
            columns={[
              { key: 't', header: 'Лот', render: (row) => row.title },
              { key: 'b', header: 'Марка', render: (row) => `${row.brand} ${row.model}` },
              { key: 'p', header: 'Цена', numeric: true, render: (row) => row.price_label },
              {
                key: 'a',
                header: '',
                render: (row) => (
                  <div className="flex gap-1">
                    <Button size="sm" onClick={() => moderateCar.mutate({ id: row.id, approve: true })}>
                      Одобрить
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => moderateCar.mutate({ id: row.id, approve: false })}
                    >
                      В черновик
                    </Button>
                  </div>
                ),
              },
            ]}
            rows={cars.data.items}
            rowKey={(row) => row.id}
            empty={<EmptyState title="Очередь объявлений пуста" />}
          />
        )}
      </section>
      <section>
        <h2 className="mb-3 text-sm font-medium">Баннеры</h2>
        {banners.isPending && <Spinner className="text-[var(--accent)]" />}
        {banners.data && (
          <DataTable
            columns={[
              { key: 't', header: 'Заголовок', render: (row) => row.title },
              { key: 'p', header: 'Место', render: (row) => row.placement_title },
              {
                key: 'a',
                header: '',
                render: (row) => (
                  <div className="flex gap-1">
                    <Button size="sm" onClick={() => moderate.mutate({ id: row.id, approve: true })}>
                      Одобрить
                    </Button>
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => moderate.mutate({ id: row.id, approve: false })}
                    >
                      Отклонить
                    </Button>
                  </div>
                ),
              },
            ]}
            rows={banners.data.items}
            rowKey={(row) => row.id}
            empty={<EmptyState title="Очередь пуста" />}
          />
        )}
      </section>
      <section>
        <h2 className="mb-3 text-sm font-medium">Поставщики без отметки</h2>
        {sellers.data && (
          <DataTable
            columns={[
              { key: 'n', header: 'Название', render: (row) => row.display_name },
              { key: 'c', header: 'Страна', render: (row) => row.country_title },
              {
                key: 'a',
                header: '',
                render: (row) => (
                  <Button size="sm" onClick={() => verify.mutate(row.id)}>
                    Проверен
                  </Button>
                ),
              },
            ]}
            rows={sellers.data.items}
            rowKey={(row) => row.id}
            empty={<EmptyState title="Все отмечены" />}
          />
        )}
      </section>
    </div>
  );
}

export function AdminAuditPage() {
  const [entity, setEntity] = useState('');
  const audit = useQuery({
    queryKey: queryKeys.adminAudit({ entity }),
    queryFn: ({ signal }) =>
      adminApi.audit({ ...(entity ? { entity } : {}), limit: 100 }, signal),
  });
  const security = useQuery({
    queryKey: queryKeys.adminSecurity({}),
    queryFn: ({ signal }) => adminApi.securityEvents({ limit: 50 }, signal),
  });

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        kicker="Админка"
        title="Аудит"
        description="Кто что менял: пользователи, лоты, сделки. Ниже — события безопасности (входы, блокировки)."
      />
      <SelectField
        label="Сущность"
        placeholder="Все"
        value={entity}
        onChange={(event) => setEntity(event.target.value)}
        options={[
          { value: 'user', title: 'Пользователь' },
          { value: 'car', title: 'Авто' },
          { value: 'deal', title: 'Сделка' },
          { value: 'seller', title: 'Поставщик' },
          { value: 'banner', title: 'Баннер' },
        ]}
      />
      {audit.data && (
        <DataTable
          columns={[
            { key: 't', header: 'Когда', render: (row) => formatDateTime(row.created_at) },
            { key: 'a', header: 'Кто', render: (row) => row.actor_name || 'система' },
            { key: 'c', header: 'Действие', render: (row) => row.action },
            { key: 'e', header: 'Объект', render: (row) => `${row.entity} ${row.entity_id ?? ''}` },
          ]}
          rows={audit.data.items}
          rowKey={(row) => String(row.id)}
          empty={<EmptyState title="Журнал пуст" />}
        />
      )}
      <section>
        <h2 className="mb-3 text-sm font-medium">События безопасности</h2>
        {security.data && (
          <DataTable
            columns={[
              { key: 't', header: 'Когда', render: (row) => formatDateTime(row.created_at) },
              { key: 'k', header: 'Тип', render: (row) => row.kind },
              { key: 's', header: 'Уровень', numeric: true, render: (row) => String(row.severity) },
              { key: 'i', header: 'IP', render: (row) => row.ip || '—' },
            ]}
            rows={security.data.items}
            rowKey={(row) => String(row.id)}
            empty={<EmptyState title="Событий нет" />}
          />
        )}
      </section>
    </div>
  );
}
