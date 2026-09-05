import { useMutation, useQuery } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';
import { useNavigate } from 'react-router-dom';

import { carsApi, dealsApi, errorMessage, isApiError, sellersApi } from '@/lib/api';
import type { DealClientMatch, Stage } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import { Button, Modal, SelectField, TextField, useToast } from '@/ui';

export function CreateDealModal({
  open,
  onClose,
  defaultStage = 'lead',
}: {
  open: boolean;
  onClose: () => void;
  defaultStage?: Stage;
}) {
  const toast = useToast();
  const navigate = useNavigate();

  const [title, setTitle] = useState('');
  const [amount, setAmount] = useState('');
  const [stage, setStage] = useState<Stage>(defaultStage);
  const [clientQuery, setClientQuery] = useState('');
  const [debouncedQuery, setDebouncedQuery] = useState('');
  const [picked, setPicked] = useState<DealClientMatch | null>(null);
  const [clientName, setClientName] = useState('');
  const [clientPhone, setClientPhone] = useState('');
  const [clientEmail, setClientEmail] = useState('');
  const [carId, setCarId] = useState('');
  const [sellerId, setSellerId] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});

  useEffect(() => {
    if (open) setStage(defaultStage);
  }, [defaultStage, open]);

  useEffect(() => {
    const timer = window.setTimeout(() => setDebouncedQuery(clientQuery.trim()), 250);
    return () => window.clearTimeout(timer);
  }, [clientQuery]);

  const stages = useQuery({
    queryKey: queryKeys.stages,
    queryFn: ({ signal }) => dealsApi.stages(signal),
    enabled: open,
  });
  const clients = useQuery({
    queryKey: queryKeys.dealClients(debouncedQuery),
    queryFn: ({ signal }) => dealsApi.searchClients(debouncedQuery, signal),
    enabled: open && !picked,
  });
  const lots = useQuery({
    queryKey: queryKeys.myCars({ limit: 50 }),
    queryFn: ({ signal }) => carsApi.mine({ limit: 50 }, signal),
    enabled: open,
  });
  const sellers = useQuery({
    queryKey: queryKeys.sellers({ limit: 50 }),
    queryFn: ({ signal }) => sellersApi.list({ limit: 50 }, signal),
    enabled: open,
  });

  const create = useMutation({
    mutationFn: () => {
      const amountRub = Number(amount.replace(/\s/g, '').replace(',', '.'));
      return dealsApi.create({
        title: title.trim(),
        stage,
        ...(picked
          ? { client_id: picked.id }
          : {
              client_name: clientName.trim(),
              client_phone: clientPhone.trim(),
              ...(clientEmail.trim() ? { client_email: clientEmail.trim() } : {}),
            }),
        ...(Number.isFinite(amountRub) && amountRub > 0
          ? { amount_minor: Math.round(amountRub * 100), currency: 'rub' }
          : {}),
        ...(carId ? { car_id: carId } : {}),
        ...(sellerId ? { seller_id: sellerId } : {}),
      });
    },
    onSuccess: (data) => {
      toast.success('Сделка создана');
      onClose();
      reset();
      void navigate(`/app/deals/${data.deal.id}`);
    },
    onError: (error) => {
      if (isApiError(error) && Object.keys(error.details).length > 0) {
        setFieldErrors(error.details);
      }
      toast.error(errorMessage(error));
    },
  });

  function reset() {
    setTitle('');
    setAmount('');
    setStage(defaultStage);
    setClientQuery('');
    setPicked(null);
    setClientName('');
    setClientPhone('');
    setClientEmail('');
    setCarId('');
    setSellerId('');
    setFieldErrors({});
  }

  function onSubmit(event: FormEvent) {
    event.preventDefault();
    setFieldErrors({});
    create.mutate();
  }

  const matches = clients.data?.items ?? [];

  return (
    <Modal
      open={open}
      onClose={() => {
        reset();
        onClose();
      }}
      title="Новая сделка"
      wide
      footer={
        <>
          <Button
            onClick={() => {
              reset();
              onClose();
            }}
          >
            Отмена
          </Button>
          <Button variant="primary" loading={create.isPending} onClick={() => create.mutate()}>
            Создать
          </Button>
        </>
      }
    >
      <form onSubmit={(event) => void onSubmit(event)} className="grid gap-3 sm:grid-cols-2">
        <div className="sm:col-span-2">
          <TextField
            label="Название"
            required
            value={title}
            onChange={(event) => setTitle(event.target.value)}
            placeholder="Toyota Camry, подбор под ключ"
            {...(fieldErrors.title ? { error: fieldErrors.title } : {})}
          />
        </div>
        <SelectField
          label="Этап воронки"
          value={stage}
          onChange={(event) => setStage(event.target.value as Stage)}
          options={(stages.data?.items ?? []).map((item) => ({
            value: item.stage,
            title: item.title,
          }))}
          {...(fieldErrors.stage ? { error: fieldErrors.stage } : {})}
        />
        <TextField
          label="Сумма, ₽"
          numeric
          value={amount}
          onChange={(event) => setAmount(event.target.value)}
          placeholder="Необязательно"
          {...(fieldErrors.amount_minor ? { error: fieldErrors.amount_minor } : {})}
        />

        <div className="sm:col-span-2 border-t border-[var(--border-hairline)] pt-3">
          <p className="mb-3 text-[13px] font-medium text-[var(--text-secondary)]">Клиент</p>
          {picked ? (
            <div className="flex items-center justify-between gap-3 border border-[var(--border-hairline)] px-3 py-2">
              <div>
                <p className="text-sm font-medium">{picked.full_name}</p>
                <p className="text-xs text-[var(--text-muted)]">
                  {picked.phone}
                  {picked.email ? ` · ${picked.email}` : ''}
                </p>
              </div>
              <Button
                size="sm"
                onClick={() => {
                  setPicked(null);
                  setClientQuery('');
                }}
              >
                Сменить
              </Button>
            </div>
          ) : (
            <div className="grid gap-3 sm:grid-cols-2">
              <div className="sm:col-span-2">
                <TextField
                  label="Найти среди покупателей"
                  value={clientQuery}
                  onChange={(event) => setClientQuery(event.target.value)}
                  placeholder="Имя, телефон или почта"
                />
                {matches.length > 0 && (
                  <ul className="mt-2 divide-y divide-[var(--border-hairline)] border border-[var(--border-hairline)]">
                    {matches.map((item) => (
                      <li key={item.id}>
                        <button
                          type="button"
                          className="flex w-full flex-col items-start px-3 py-2 text-left hover:bg-[var(--surface-sunken)]"
                          onClick={() => setPicked(item)}
                        >
                          <span className="text-sm font-medium">{item.full_name}</span>
                          <span className="text-xs text-[var(--text-muted)]">
                            {item.phone} · {item.email}
                          </span>
                        </button>
                      </li>
                    ))}
                  </ul>
                )}
              </div>
              <TextField
                label="Имя"
                required
                value={clientName}
                onChange={(event) => setClientName(event.target.value)}
                {...(fieldErrors.client_name ? { error: fieldErrors.client_name } : {})}
              />
              <TextField
                label="Телефон"
                required
                value={clientPhone}
                onChange={(event) => setClientPhone(event.target.value)}
                placeholder="+7 900 000-00-00"
                {...(fieldErrors.client_phone ? { error: fieldErrors.client_phone } : {})}
              />
              <div className="sm:col-span-2">
                <TextField
                  label="Почта"
                  value={clientEmail}
                  onChange={(event) => setClientEmail(event.target.value)}
                  placeholder="Если нет — заведём карточку по телефону"
                  {...(fieldErrors.client_email ? { error: fieldErrors.client_email } : {})}
                />
              </div>
            </div>
          )}
        </div>

        <SelectField
          label="Лот из объявлений"
          placeholder="Без лота"
          value={carId}
          onChange={(event) => setCarId(event.target.value)}
          options={(lots.data?.items ?? []).map((car) => ({
            value: car.id,
            title: car.title || `${car.brand} ${car.model}`,
          }))}
        />
        <SelectField
          label="Поставщик"
          placeholder="Не выбран"
          value={sellerId}
          onChange={(event) => setSellerId(event.target.value)}
          options={(sellers.data?.items ?? []).map((seller) => ({
            value: seller.id,
            title: seller.display_name,
          }))}
        />
      </form>
    </Modal>
  );
}
