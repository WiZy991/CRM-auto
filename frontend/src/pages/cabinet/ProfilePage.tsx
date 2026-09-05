import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';

import { useAuth } from '@/features/auth/auth-context';
import { authApi, dealersApi, errorMessage } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { Badge, Button, CheckField, DataTable, PageHeader, SelectField, TextAreaField, TextField, useToast } from '@/ui';

export function ProfilePage() {
  const { user, applyUser, logout, hasRole } = useAuth();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [oldPassword, setOldPassword] = useState('');
  const [newPassword, setNewPassword] = useState('');
  const [fullName, setFullName] = useState(user?.full_name ?? '');
  const [passport, setPassport] = useState('');
  const [address, setAddress] = useState('');
  const [deletePassword, setDeletePassword] = useState('');

  const me = useQuery({
    queryKey: queryKeys.currentUser,
    queryFn: ({ signal }) => authApi.me(signal),
  });

  useEffect(() => {
    if (!me.data) return;
    setFullName(me.data.user.full_name);
    setPassport(me.data.user.passport ?? '');
    setAddress(me.data.user.address ?? '');
  }, [me.data]);

  const sessions = useQuery({
    queryKey: queryKeys.sessions,
    queryFn: () => authApi.sessions(),
  });

  const saveProfile = useMutation({
    mutationFn: () => authApi.updateProfile({ full_name: fullName, passport, address }),
    onSuccess: (data) => {
      applyUser(data.user);
      void queryClient.invalidateQueries({ queryKey: queryKeys.currentUser });
      toast.success('Профиль сохранён');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const change = useMutation({
    mutationFn: () => authApi.changePassword(oldPassword, newPassword),
    onSuccess: () => {
      toast.success('Пароль изменён');
      setOldPassword('');
      setNewPassword('');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const revoke = useMutation({
    mutationFn: (id: string) => authApi.revokeSession(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.sessions });
      toast.success('Сессия отозвана');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const remove = useMutation({
    mutationFn: () => authApi.deleteAccount(deletePassword),
    onSuccess: () => {
      toast.success('Учётная запись удалена');
      void logout();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  function onPassword(event: FormEvent) {
    event.preventDefault();
    change.mutate();
  }

  function onProfile(event: FormEvent) {
    event.preventDefault();
    saveProfile.mutate();
  }

  return (
    <div className="flex flex-col gap-10">
      <PageHeader
        kicker="Кабинет"
        title="Профиль"
        description="Имя, паспорт и адрес подставляются в договор. Здесь же смена пароля и список сессий."
      />

      <section className="panel grid gap-3 p-5 sm:grid-cols-2">
        <Field label="Роль" value={user?.role_title ?? ''} />
        <Field label="Почта" value={user?.email ?? ''} extra={user?.email_verified ? 'подтверждена' : 'не подтверждена'} />
        <Field label="Телефон" value={user?.phone ?? ''} extra={user?.phone_verified ? 'подтверждён' : 'не подтверждён'} />
      </section>

      {user && (!user.email_verified || !user.phone_verified) && <VerifyContacts />}

      <section>
        <h2 className="mb-4 text-sm font-medium">Персональные данные</h2>
        <p className="mb-4 max-w-xl text-xs text-[var(--text-muted)]">
          Паспорт и адрес шифруются на сервере. В логах и в чужих ответах их нет.
        </p>
        <form onSubmit={(event) => void onProfile(event)} className="flex max-w-md flex-col gap-3">
          <TextField
            label="Имя"
            value={fullName}
            onChange={(event) => setFullName(event.target.value)}
            required
          />
          <TextField
            label="Паспорт"
            value={passport}
            onChange={(event) => setPassport(event.target.value)}
            hint="Серия и номер, только для сделки"
          />
          <TextAreaField
            label="Адрес"
            value={address}
            onChange={(event) => setAddress(event.target.value)}
            rows={3}
          />
          <Button type="submit" loading={saveProfile.isPending}>
            Сохранить данные
          </Button>
        </form>
      </section>

      {hasRole('dealer', 'admin') && <DealerProfileForm />}

      <section>
        <h2 className="mb-4 text-sm font-medium">Смена пароля</h2>
        <form onSubmit={(event) => void onPassword(event)} className="flex max-w-md flex-col gap-3">
          <TextField
            label="Текущий пароль"
            type="password"
            autoComplete="current-password"
            value={oldPassword}
            onChange={(event) => setOldPassword(event.target.value)}
            required
          />
          <TextField
            label="Новый пароль"
            type="password"
            autoComplete="new-password"
            minLength={10}
            value={newPassword}
            onChange={(event) => setNewPassword(event.target.value)}
            required
          />
          <Button type="submit" loading={change.isPending}>
            Сохранить пароль
          </Button>
        </form>
      </section>

      <section>
        <div className="mb-4 flex items-center justify-between">
          <h2 className="text-sm font-medium">Сессии</h2>
          <Button
            size="sm"
            variant="danger"
            onClick={() => void authApi.logoutEverywhere().then(() => logout())}
          >
            Выйти везде
          </Button>
        </div>
        <DataTable
          columns={[
            { key: 'device', header: 'Устройство', render: (row) => row.device || 'Неизвестно' },
            { key: 'ip', header: 'IP', numeric: true, render: (row) => row.ip ?? '—' },
            {
              key: 'used',
              header: 'Активность',
              render: (row) => formatDateTime(row.last_used_at ?? row.created_at),
            },
            {
              key: 'flag',
              header: '',
              render: (row) =>
                row.current ? (
                  <Badge tone="jade">Эта вкладка</Badge>
                ) : (
                  <Button size="sm" variant="ghost" onClick={() => revoke.mutate(row.id)}>
                    Отозвать
                  </Button>
                ),
            },
          ]}
          rows={sessions.data?.items ?? []}
          rowKey={(row) => row.id}
          empty={<p className="text-sm text-[var(--text-muted)]">Список сессий недоступен.</p>}
        />
      </section>

      <section>
        <h2 className="mb-4 text-sm font-medium">Удаление аккаунта</h2>
        <p className="mb-3 max-w-xl text-xs text-[var(--text-muted)]">
          Имя и контакты обезличиваются. Сделки и документы сохраняются, потому что дилер обязан их хранить.
        </p>
        <form
          className="flex max-w-md flex-col gap-3"
          onSubmit={(event) => {
            event.preventDefault();
            if (window.confirm('Удалить учётную запись безвозвратно?')) remove.mutate();
          }}
        >
          <TextField
            label="Пароль для подтверждения"
            type="password"
            value={deletePassword}
            onChange={(event) => setDeletePassword(event.target.value)}
            required
          />
          <Button type="submit" variant="danger" loading={remove.isPending}>
            Удалить аккаунт
          </Button>
        </form>
      </section>
    </div>
  );
}

function VerifyContacts() {
  const { applyUser, reload } = useAuth();
  const toast = useToast();
  const [channel, setChannel] = useState<'email' | 'phone'>('email');
  const [code, setCode] = useState('');

  const send = useMutation({
    mutationFn: () => authApi.sendCode(channel),
    onSuccess: () => toast.success('Код отправлен', 'Если почта/SMS не настроены, код в логе API.'),
    onError: (error) => toast.error(errorMessage(error)),
  });

  const confirm = useMutation({
    mutationFn: () => authApi.verify(channel, code.trim()),
    onSuccess: async (data) => {
      applyUser(data.user);
      setCode('');
      toast.success('Контакт подтверждён');
      await reload();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <section>
      <h2 className="mb-4 text-sm font-medium">Подтверждение контактов</h2>
      <p className="mb-4 max-w-xl text-xs text-[var(--text-muted)]">
        Почта и телефон нужны для заявок и объявлений. В локальной среде коды пишутся в лог API.
      </p>
      <form
        className="flex max-w-md flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          if (code.trim()) confirm.mutate();
        }}
      >
        <SelectField
          label="Канал"
          value={channel}
          onChange={(event) => setChannel(event.target.value as 'email' | 'phone')}
          options={[
            { value: 'email', title: 'Электронная почта' },
            { value: 'phone', title: 'Телефон' },
          ]}
        />
        <div className="flex gap-2">
          <Button type="button" size="sm" onClick={() => send.mutate()} loading={send.isPending}>
            Выслать код
          </Button>
        </div>
        <TextField
          label="Код"
          value={code}
          onChange={(event) => setCode(event.target.value)}
          inputMode="numeric"
        />
        <Button type="submit" loading={confirm.isPending}>
          Подтвердить
        </Button>
      </form>
    </section>
  );
}

const DEALER_SERVICES = [
  { value: 'подбор', title: 'Подбор' },
  { value: 'выкуп', title: 'Выкуп' },
  { value: 'доставка', title: 'Доставка' },
  { value: 'растаможка', title: 'Растаможка' },
  { value: 'СБКТС', title: 'СБКТС' },
  { value: 'постановка на учёт', title: 'Постановка на учёт' },
] as const;

function DealerProfileForm() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const mine = useQuery({
    queryKey: queryKeys.dealerMine,
    queryFn: ({ signal }) => dealersApi.mine(signal),
  });
  const [slug, setSlug] = useState('');
  const [company, setCompany] = useState('');
  const [city, setCity] = useState('');
  const [description, setDescription] = useState('');
  const [inn, setInn] = useState('');
  const [services, setServices] = useState<string[]>([]);

  useEffect(() => {
    const dealer = mine.data?.dealer;
    if (!dealer) return;
    setSlug(dealer.slug);
    setCompany(dealer.company_name);
    setCity(dealer.city);
    setDescription(dealer.description);
    setInn(dealer.inn ?? '');
    setServices(dealer.services ?? []);
  }, [mine.data]);

  const save = useMutation({
    mutationFn: () =>
      dealersApi.saveMine({
        slug,
        company_name: company,
        city,
        description,
        inn,
        services,
        work_countries: mine.data?.dealer.work_countries ?? ['cn', 'jp'],
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerMine });
      toast.success('Карточка дилера обновлена');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  if (mine.isError) return null;

  return (
    <section>
      <h2 className="mb-4 text-sm font-medium">Карточка импортёра</h2>
      <form
        className="flex max-w-md flex-col gap-3"
        onSubmit={(event) => {
          event.preventDefault();
          save.mutate();
        }}
      >
        <TextField label="Название" value={company} onChange={(event) => setCompany(event.target.value)} required />
        <TextField
          label="Город"
          value={city}
          onChange={(event) => setCity(event.target.value)}
          required
          hint="Показывается в каталоге /dealers"
        />
        <TextField label="Адрес карточки" value={slug} onChange={(event) => setSlug(event.target.value)} hint="Латиница и дефис, как в ссылке /dealers/…" />
        <TextField label="ИНН" value={inn} onChange={(event) => setInn(event.target.value)} />
        <TextAreaField label="Описание" value={description} onChange={(event) => setDescription(event.target.value)} rows={3} />
        <fieldset className="flex flex-col gap-2">
          <legend className="text-[13px] font-medium text-[var(--text-secondary)]">Услуги</legend>
          {DEALER_SERVICES.map((item) => (
            <CheckField
              key={item.value}
              label={item.title}
              checked={services.includes(item.value)}
              onChange={(event) => {
                setServices((current) =>
                  event.target.checked
                    ? [...current, item.value]
                    : current.filter((value) => value !== item.value),
                );
              }}
            />
          ))}
        </fieldset>
        <Button type="submit" loading={save.isPending}>
          Сохранить карточку
        </Button>
      </form>
    </section>
  );
}

function Field({ label, value, extra }: { label: string; value: string; extra?: string }) {
  return (
    <div>
      <p className="text-xs font-medium text-[var(--text-muted)]">{label}</p>
      <p className="mt-1 text-sm">{value}</p>
      {extra && <p className="text-xs text-[var(--text-secondary)]">{extra}</p>}
    </div>
  );
}
