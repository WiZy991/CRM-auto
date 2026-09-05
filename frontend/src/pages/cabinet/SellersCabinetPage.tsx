import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { errorMessage, isApiError, sellersApi } from '@/lib/api';
import type { Seller, SellerForm } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  CheckField,
  Combobox,
  DataTable,
  EmptyState,
  Modal,
  originTitle,
  originTone,
  PageHeader,
  SelectField,
  Spinner,
  TextAreaField,
  TextField,
  useToast,
} from '@/ui';

export function SellersCabinetPage() {
  const navigate = useNavigate();
  const [country, setCountry] = useState('');
  const [kind, setKind] = useState('');
  const [query, setQuery] = useState('');
  const [region, setRegion] = useState('');
  const [brand, setBrand] = useState('');
  const [createOpen, setCreateOpen] = useState(false);

  const filters = {
    ...(country ? { country: [country] } : {}),
    ...(kind ? { kind: [kind] } : {}),
    ...(region ? { region: [region] } : {}),
    ...(brand ? { brand: [brand] } : {}),
    ...(query ? { q: query } : {}),
    limit: 50,
  };

  const list = useQuery({
    queryKey: queryKeys.sellers(filters),
    queryFn: ({ signal }) => sellersApi.list(filters, signal),
  });
  const facets = useQuery({
    queryKey: queryKeys.sellerFacets(country ? [country] : []),
    queryFn: ({ signal }) => sellersApi.facets(country ? [country] : undefined, signal),
  });
  const brandOptions = (facets.data?.brands ?? []).map((item) => ({
    value: item.brand,
    title: item.brand,
  }));

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Дилер"
        title="Поставщики"
        description="Заводы, аукционы и экспортёры. Откройте карточку, чтобы увидеть контакты и оставить заметку."
        actions={
          <Button variant="primary" size="sm" onClick={() => setCreateOpen(true)}>
            Добавить поставщика
          </Button>
        }
      />
      <div className="grid gap-3 sm:grid-cols-2 lg:grid-cols-5">
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
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.data && (
        <DataTable
          columns={[
            { key: 'name', header: 'Название', render: (row: Seller) => row.display_name },
            {
              key: 'c',
              header: 'Страна',
              render: (row) => <Badge tone={originTone(row.country)}>{originTitle(row.country)}</Badge>,
            },
            { key: 'k', header: 'Тип', render: (row) => row.kind_title },
            { key: 'r', header: 'Регион', render: (row) => row.region },
            { key: 'b', header: 'Бренды', render: (row) => row.brands.slice(0, 3).join(', ') || '—' },
            {
              key: 'v',
              header: '',
              render: (row) => (row.is_verified ? <Badge tone="jade">Проверен</Badge> : null),
            },
          ]}
          rows={list.data.items}
          rowKey={(row) => row.id}
          onRowClick={(row) => void navigate(`/app/sellers/${row.id}`)}
          empty={<EmptyState title="Никого не найдено" />}
        />
      )}
      <Modal open={createOpen} onClose={() => setCreateOpen(false)} title="Новый поставщик">
        <SellerEditor
          onSaved={(id) => {
            setCreateOpen(false);
            void navigate(`/app/sellers/${id}`);
          }}
        />
      </Modal>
    </div>
  );
}

export function SellerCardPage() {
  const { id = '' } = useParams();
  const navigate = useNavigate();
  const toast = useToast();
  const queryClient = useQueryClient();
  const { hasRole } = useAuth();
  const [note, setNote] = useState('');
  const [trusted, setTrusted] = useState(false);

  const details = useQuery({
    queryKey: queryKeys.seller(id),
    queryFn: ({ signal }) => sellersApi.get(id, signal),
    enabled: Boolean(id),
  });

  useEffect(() => {
    if (!details.data) return;
    setNote(details.data.seller.note ?? '');
    setTrusted(Boolean(details.data.seller.is_trusted));
  }, [details.data]);

  const save = useMutation({
    mutationFn: () => sellersApi.saveNote(id, note, trusted),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.seller(id) });
      toast.success('Заметка сохранена');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const seller = details.data?.seller;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Поставщик"
        title={seller?.display_name ?? 'Карточка'}
        description="Карточка поставщика: тип, регион, бренды и ваши контакты к нему."
        actions={
          <Button size="sm" variant="ghost" onClick={() => void navigate('/app/sellers')}>
            К списку
          </Button>
        }
      />
      {details.isPending && <Spinner className="text-[var(--accent)]" />}
      {seller && (
        <>
          <dl className="panel grid gap-3 p-5 text-sm sm:grid-cols-2">
            <div>
              <dt className="text-[var(--text-muted)]">Тип</dt>
              <dd>{seller.kind_title}</dd>
            </div>
            <div>
              <dt className="text-[var(--text-muted)]">Регион</dt>
              <dd>
                {seller.country_title}, {seller.region}
              </dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-[var(--text-muted)]">Бренды</dt>
              <dd>{seller.brands.join(', ') || '—'}</dd>
            </div>
            <div className="sm:col-span-2">
              <dt className="text-[var(--text-muted)]">Контакты</dt>
              <dd className="mt-1 flex flex-col gap-1">
                {seller.country === 'cn' ? (
                  <span>WeChat: {contactOf(seller.contacts, 'wechat') || '—'}</span>
                ) : (
                  <>
                    {seller.contacts.phone || seller.contacts.tel ? (
                      <span>Телефон: {seller.contacts.phone || seller.contacts.tel}</span>
                    ) : (
                      <span>Телефон: —</span>
                    )}
                    {contactOf(seller.contacts, 'wechat') ? (
                      <span>WeChat: {contactOf(seller.contacts, 'wechat')}</span>
                    ) : null}
                  </>
                )}
                {seller.website ? (
                  <span>
                    Сайт:{' '}
                    <a href={seller.website} className="text-[var(--link)] underline underline-offset-2" rel="noreferrer">
                      {seller.website}
                    </a>
                  </span>
                ) : null}
              </dd>
            </div>
            {seller.description && (
              <div className="sm:col-span-2">
                <dt className="text-[var(--text-muted)]">Описание</dt>
                <dd>{seller.description}</dd>
              </div>
            )}
          </dl>
          {seller.can_edit && (
            <section>
              <h2 className="mb-3 text-sm font-medium">Редактирование</h2>
              <SellerEditor
                seller={seller}
                onSaved={() => {
                  void queryClient.invalidateQueries({ queryKey: queryKeys.seller(id) });
                  void queryClient.invalidateQueries({ queryKey: ['sellers'] });
                }}
              />
            </section>
          )}
          {hasRole('dealer') && (
          <section className="flex max-w-lg flex-col gap-3">
            <h2 className="text-sm font-medium">Заметка дилера</h2>
            <TextField label="Текст" value={note} onChange={(event) => setNote(event.target.value)} />
            <CheckField
              label="Свой проверенный"
              checked={trusted}
              onChange={(event) => setTrusted(event.target.checked)}
            />
            <Button onClick={() => save.mutate()} loading={save.isPending}>
              Сохранить заметку
            </Button>
          </section>
          )}
        </>
      )}
    </div>
  );
}

function SellerEditor({
  seller,
  onSaved,
}: {
  seller?: Seller;
  onSaved: (id: string) => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [country, setCountry] = useState<string>(seller?.country ?? 'cn');
  const [kind, setKind] = useState<string>(seller?.kind ?? 'exporter');
  const [name, setName] = useState(seller?.name ?? '');
  const [region, setRegion] = useState(seller?.region === 'не указан' ? '' : (seller?.region ?? ''));
  const [city, setCity] = useState(seller?.city ?? '');
  const [brands, setBrands] = useState(seller?.brands.join(', ') ?? '');
  const [description, setDescription] = useState(seller?.description ?? '');
  const [website, setWebsite] = useState(seller?.website ?? '');
  const [phone, setPhone] = useState(seller?.contacts?.phone ?? seller?.contacts?.tel ?? '');
  const [wechat, setWechat] = useState(contactOf(seller?.contacts ?? {}, 'wechat'));
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  const save = useMutation({
    mutationFn: () => {
      const payload: SellerForm = {
        country,
        kind,
        name: name.trim(),
        region: region.trim() || 'не указан',
        brands: brands
          .split(',')
          .map((item) => item.trim())
          .filter(Boolean),
        contacts: {
          ...(phone.trim() ? { phone: phone.trim() } : {}),
          ...(wechat.trim() ? { wechat: wechat.trim() } : {}),
        },
      };
      if (city.trim()) payload.city = city.trim();
      if (description.trim()) payload.description = description.trim();
      if (website.trim()) payload.website = website.trim();
      return seller ? sellersApi.update(seller.id, payload) : sellersApi.create(payload);
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: ['sellers'] });
      setFieldErrors({});
      toast.success(seller ? 'Карточка сохранена' : 'Поставщик добавлен');
      onSaved(data.seller.id);
    },
    onError: (error) => {
      if (isApiError(error) && Object.keys(error.details).length > 0) {
        setFieldErrors(error.details);
      }
      toast.error(errorMessage(error));
    },
  });

  return (
    <form
      className="flex max-w-md flex-col gap-3"
      onSubmit={(event) => {
        event.preventDefault();
        save.mutate();
      }}
    >
      <SelectField
        label="Страна"
        value={country}
        onChange={(event) => setCountry(event.target.value)}
        options={[
          { value: 'cn', title: 'Китай' },
          { value: 'jp', title: 'Япония' },
        ]}
      />
      <SelectField
        label="Тип"
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
      <TextField label="Название" value={name} onChange={(event) => setName(event.target.value)} required />
      <TextField label="Регион" value={region} onChange={(event) => setRegion(event.target.value)} />
      <TextField label="Город" value={city} onChange={(event) => setCity(event.target.value)} />
      <TextField
        label="Бренды"
        value={brands}
        onChange={(event) => setBrands(event.target.value)}
        hint="Через запятую"
      />
      {country === 'cn' ? (
        <TextField
          label="WeChat"
          value={wechat}
          onChange={(event) => setWechat(event.target.value)}
          required
          {...(fieldErrors.wechat ? { error: fieldErrors.wechat } : {})}
        />
      ) : (
        <TextField label="Телефон" value={phone} onChange={(event) => setPhone(event.target.value)} />
      )}
      {country === 'cn' ? (
        <TextField label="Телефон" value={phone} onChange={(event) => setPhone(event.target.value)} />
      ) : (
        <TextField label="WeChat" value={wechat} onChange={(event) => setWechat(event.target.value)} />
      )}
      <TextField label="Сайт" value={website} onChange={(event) => setWebsite(event.target.value)} />
      <TextAreaField
        label="Описание"
        value={description}
        onChange={(event) => setDescription(event.target.value)}
        rows={3}
      />
      <Button type="submit" variant="primary" loading={save.isPending}>
        Сохранить
      </Button>
    </form>
  );
}

function contactOf(contacts: Record<string, string>, name: string): string {
  const found = Object.entries(contacts).find(([key]) => key.toLowerCase() === name);
  return found?.[1] ?? '';
}
