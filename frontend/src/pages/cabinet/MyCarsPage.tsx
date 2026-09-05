import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';

import { carsApi, errorMessage, isApiError, uploadsApi } from '@/lib/api';
import type { CarForm, Origin } from '@/lib/api';
import { carStatusTone, carStatusTitle } from '@/lib/status';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  FullPageSpinner,
  Modal,
  PageHeader,
  PageGuide,
  SelectField,
  Spinner,
  TextAreaField,
  TextField,
  useToast,
} from '@/ui';

export function MyCarsPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [createOpen, setCreateOpen] = useState(false);
  const [editId, setEditId] = useState<string | null>(null);

  const list = useQuery({
    queryKey: queryKeys.myCars({ limit: 50 }),
    queryFn: ({ signal }) => carsApi.mine({ limit: 50 }, signal),
  });

  const status = useMutation({
    mutationFn: ({ id, next }: { id: string; next: string }) => carsApi.changeStatus(id, next),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['cars'] });
      toast.success('Статус обновлён');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const publish = useMutation({
    mutationFn: (id: string) => carsApi.publishSocial(id),
    onSuccess: () => toast.success('Лот поставлен в очередь каналов'),
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Дилер"
        title="Объявления"
        description="Лоты на витрине сайта. Черновик виден только вам, после проверки попадает в каталог. Из продажи можно отправить пост в подключённые каналы."
        actions={
          <Button variant="primary" size="sm" onClick={() => setCreateOpen(true)}>
            Новый лот
          </Button>
        }
      />
      <PageGuide
        items={[
          { title: 'Заполните лот', text: 'Марка, год, цена и фото. Без трёх фото на проверку не отправить.' },
          { title: 'Модерация', text: '«На проверку» — смотрит администратор. Отказ вернёт черновик.' },
          { title: 'В каталоге', text: 'Статус «В продаже» — лот видят гости. «Опубликовать» шлёт его в соцсети.' },
        ]}
      />
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.data && (
        <DataTable
          columns={[
            {
              key: 'title',
              header: 'Лот',
              render: (row) =>
                row.status === 'draft' || row.status === 'archived' ? (
                  <button
                    type="button"
                    className="text-left text-sm font-medium text-[var(--link)] underline-offset-2 hover:underline"
                    onClick={() => setEditId(row.id)}
                  >
                    {row.title || `${row.brand} ${row.model}`}
                  </button>
                ) : (
                  row.title || `${row.brand} ${row.model}`
                ),
            },
            { key: 'year', header: 'Год', numeric: true, render: (row) => String(row.year) },
            { key: 'price', header: 'Цена', numeric: true, render: (row) => row.price_label },
            {
              key: 'st',
              header: 'Статус',
              render: (row) => <Badge tone={carStatusTone(row.status)}>{carStatusTitle(row.status)}</Badge>,
            },
            {
              key: 'act',
              header: '',
              render: (row) => (
                <div className="flex flex-wrap gap-1">
                  {(row.status === 'draft' || row.status === 'archived') && (
                    <>
                      <Button size="sm" onClick={() => setEditId(row.id)}>
                        Изменить
                      </Button>
                      {row.status === 'draft' && (
                        <Button
                          size="sm"
                          variant="primary"
                          onClick={() => status.mutate({ id: row.id, next: 'moderation' })}
                        >
                          На проверку
                        </Button>
                      )}
                    </>
                  )}
                  {row.status === 'active' && (
                    <>
                      <Button size="sm" onClick={() => setEditId(row.id)}>
                        Изменить
                      </Button>
                      <Button size="sm" onClick={() => publish.mutate(row.id)}>
                        Опубликовать
                      </Button>
                      <Button size="sm" onClick={() => status.mutate({ id: row.id, next: 'archived' })}>
                        Снять
                      </Button>
                    </>
                  )}
                </div>
              ),
            },
          ]}
          rows={list.data.items}
          rowKey={(row) => row.id}
          empty={
            <EmptyState
              title="Пока нет лотов"
              description="Создайте черновик, добавьте фото и отправьте на проверку — после одобрения машина появится в каталоге."
              action={
                <Button variant="primary" size="sm" onClick={() => setCreateOpen(true)}>
                  Новый лот
                </Button>
              }
            />
          }
        />
      )}
      <CarFormModal open={createOpen} onClose={() => setCreateOpen(false)} />
      {editId ? (
        <CarFormModal open onClose={() => setEditId(null)} carId={editId} />
      ) : null}
    </div>
  );
}

function CarFormModal({
  open,
  onClose,
  carId,
}: {
  open: boolean;
  onClose: () => void;
  carId?: string;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const editing = Boolean(carId);

  const dict = useQuery({
    queryKey: queryKeys.carDictionaries(),
    queryFn: ({ signal }) => carsApi.dictionaries(undefined, signal),
    enabled: open,
  });

  const existing = useQuery({
    queryKey: queryKeys.car(carId ?? ''),
    queryFn: ({ signal }) => carsApi.get(carId!, signal),
    enabled: open && Boolean(carId),
  });

  const [origin, setOrigin] = useState<Origin>('cn');
  const [brand, setBrand] = useState('');
  const [model, setModel] = useState('');
  const [year, setYear] = useState('2022');
  const [mileage, setMileage] = useState('10000');
  const [price, setPrice] = useState('');
  const [fuel, setFuel] = useState('petrol');
  const [gearbox, setGearbox] = useState('at');
  const [drive, setDrive] = useState('fwd');
  const [body, setBody] = useState('sedan');
  const [description, setDescription] = useState('');
  const [photoUrls, setPhotoUrls] = useState<string[]>([]);
  const [uploading, setUploading] = useState(false);
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (!open) return;
    if (!editing) {
      setOrigin('cn');
      setBrand('');
      setModel('');
      setYear('2022');
      setMileage('10000');
      setPrice('');
      setFuel('petrol');
      setGearbox('at');
      setDrive('fwd');
      setBody('sedan');
      setDescription('');
      setPhotoUrls([]);
      setFieldErrors({});
      return;
    }
    const car = existing.data?.car;
    if (!car) return;
    setOrigin(car.origin);
    setBrand(car.brand);
    setModel(car.model);
    setYear(String(car.year));
    setMileage(String(car.mileage_km));
    setPrice(String(Math.round(car.price_rub_minor / 100)));
    setFuel(car.fuel);
    setGearbox(car.gearbox);
    setDrive(car.drive);
    setBody(car.body);
    setDescription(car.description ?? '');
    setPhotoUrls(car.photos?.map((p) => p.url) ?? car.photo_urls ?? []);
    setFieldErrors({});
  }, [open, editing, existing.data?.car]);

  const save = useMutation({
    mutationFn: () => {
      const yearNum = Number(year);
      const title = `${brand.trim()} ${model.trim()} ${year}`.trim();
      const form: CarForm = {
        origin,
        brand: brand.trim(),
        model: model.trim(),
        year: yearNum,
        mileage_km: Number(mileage),
        fuel,
        gearbox,
        drive,
        body,
        price_minor: Math.round(Number(price) * 100),
        currency: 'rub',
        title,
        description,
        photo_urls: photoUrls,
      };
      return editing && carId ? carsApi.update(carId, form) : carsApi.create(form);
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['cars'] });
      toast.success(editing ? 'Лот сохранён' : 'Черновик создан');
      onClose();
    },
    onError: (error) => {
      if (isApiError(error) && Object.keys(error.details).length > 0) {
        setFieldErrors(error.details);
        toast.error(errorMessage(error), Object.values(error.details)[0]);
        return;
      }
      setFieldErrors({});
      toast.error(errorMessage(error));
    },
  });

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    save.mutate();
  }

  const dictionaries = dict.data?.dictionaries ?? {};
  const loadingExisting = editing && existing.isPending;

  return (
    <Modal
      open={open}
      onClose={onClose}
      title={editing ? 'Изменить лот' : 'Новый лот'}
      wide
      footer={
        <>
          <Button onClick={onClose}>Отмена</Button>
          <Button
            variant="primary"
            loading={save.isPending}
            disabled={loadingExisting}
            onClick={() => save.mutate()}
          >
            {editing ? 'Сохранить' : 'Сохранить черновик'}
          </Button>
        </>
      }
    >
      {loadingExisting ? (
        <FullPageSpinner label="Загружаем лот" />
      ) : existing.isError && editing ? (
        <EmptyState title="Не удалось открыть лот" description={errorMessage(existing.error)} />
      ) : (
        <form onSubmit={(event) => void onSubmit(event)} className="grid gap-3 sm:grid-cols-2">
          <SelectField
            label="Рынок"
            value={origin}
            onChange={(event) => setOrigin(event.target.value as Origin)}
            options={[
              { value: 'cn', title: 'Китай' },
              { value: 'jp', title: 'Япония' },
            ]}
          />
          <TextField
            label="Марка"
            required
            value={brand}
            onChange={(event) => setBrand(event.target.value)}
            {...(fieldErrors.brand ? { error: fieldErrors.brand } : {})}
          />
          <TextField
            label="Модель"
            required
            value={model}
            onChange={(event) => setModel(event.target.value)}
            {...(fieldErrors.model ? { error: fieldErrors.model } : {})}
          />
          <TextField
            label="Год"
            numeric
            value={year}
            onChange={(event) => setYear(event.target.value)}
            {...(fieldErrors.year ? { error: fieldErrors.year } : {})}
          />
          <TextField
            label="Пробег, км"
            numeric
            value={mileage}
            onChange={(event) => setMileage(event.target.value)}
            {...(fieldErrors.mileage_km ? { error: fieldErrors.mileage_km } : {})}
          />
          <TextField
            label="Цена, ₽"
            numeric
            required
            value={price}
            onChange={(event) => setPrice(event.target.value)}
            {...(fieldErrors.price_minor ? { error: fieldErrors.price_minor } : {})}
          />
          <SelectField
            label="Топливо"
            value={fuel}
            onChange={(event) => setFuel(event.target.value)}
            options={dictionaries.fuel ?? [{ value: 'petrol', title: 'Бензин' }]}
          />
          <SelectField
            label="КПП"
            value={gearbox}
            onChange={(event) => setGearbox(event.target.value)}
            options={dictionaries.transmission ?? [{ value: 'at', title: 'Автомат' }]}
          />
          <SelectField
            label="Привод"
            value={drive}
            onChange={(event) => setDrive(event.target.value)}
            options={dictionaries.drive ?? [{ value: 'fwd', title: 'Передний' }]}
          />
          <SelectField
            label="Кузов"
            value={body}
            onChange={(event) => setBody(event.target.value)}
            options={dictionaries.body ?? [{ value: 'sedan', title: 'Седан' }]}
          />
          <div className="sm:col-span-2">
            <TextAreaField
              label="Описание"
              value={description}
              onChange={(event) => setDescription(event.target.value)}
            />
          </div>
          <div className="sm:col-span-2">
            <label className="text-[13px] font-medium text-[var(--text-secondary)]">Фото</label>
            <p className="mt-1 text-xs text-[var(--text-muted)]">
              JPEG, PNG или WebP. До 30 снимков. Для публикации нужно минимум три. Файл с
              расширением .png иногда бывает WebP из мессенджера — такие тоже принимаются.
            </p>
            <input
              type="file"
              accept="image/jpeg,image/png,image/webp,.jpg,.jpeg,.png,.webp"
              multiple
              className="mt-2 block w-full text-sm"
              disabled={uploading || photoUrls.length >= 30}
              onChange={(event) => {
                const files = [...(event.target.files ?? [])];
                event.target.value = '';
                if (files.length === 0) return;
                const room = 30 - photoUrls.length;
                const batch = files.slice(0, room);
                if (batch.length === 0) {
                  toast.error('Не больше 30 фото на объявление');
                  return;
                }
                setUploading(true);
                void (async () => {
                  const next = [...photoUrls];
                  try {
                    for (const file of batch) {
                      const saved = await uploadsApi.image(file);
                      next.push(saved.url);
                    }
                    setPhotoUrls(next);
                    toast.success(
                      batch.length === 1 ? 'Фото загружено' : `Загружено фото: ${batch.length}`,
                    );
                  } catch (error: unknown) {
                    setPhotoUrls(next);
                    toast.error(errorMessage(error));
                  } finally {
                    setUploading(false);
                  }
                })();
              }}
            />
            {photoUrls.length > 0 && (
              <ul className="mt-3 grid grid-cols-4 gap-2 sm:grid-cols-6">
                {photoUrls.map((url, i) => (
                  <li key={`${url}-${i}`} className="relative">
                    <img
                      src={url}
                      alt=""
                      className="aspect-[4/3] w-full rounded-[var(--radius-sheet)] object-cover"
                    />
                    <button
                      type="button"
                      className="absolute top-1 right-1 rounded-md bg-ink-900/80 px-1.5 py-0.5 text-2xs text-paper-50"
                      onClick={() => setPhotoUrls((prev) => prev.filter((_, idx) => idx !== i))}
                    >
                      Убрать
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </div>
        </form>
      )}
    </Modal>
  );
}
