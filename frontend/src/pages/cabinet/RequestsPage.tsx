import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useState } from 'react';
import { useNavigate } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { dealsApi, errorMessage, requestsApi } from '@/lib/api';
import type { RequestListItem } from '@/lib/api';
import { formatDate } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import { requestTone } from '@/lib/status';
import {
  Badge,
  Button,
  DataTable,
  EmptyState,
  Modal,
  PageHeader,
  PageGuide,
  SelectField,
  Spinner,
  TextAreaField,
  TextField,
  useToast,
} from '@/ui';

export function RequestsPage() {
  const { hasRole } = useAuth();
  if (hasRole('dealer', 'admin')) return <DealerRequests />;
  return <ClientRequests />;
}

function ClientRequests() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const list = useQuery({
    queryKey: queryKeys.requestsMine({}),
    queryFn: ({ signal }) => requestsApi.mine({ limit: 50 }, signal),
  });

  const close = useMutation({
    mutationFn: (id: string) => requestsApi.close(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Заявка закрыта');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Клиент"
        title="Заявки"
        description="Запрос дилеру: «подберите такой автомобиль» или вопрос по лоту из каталога. Когда дилер возьмёт заявку, появится сделка."
        actions={
          <Button variant="primary" size="sm" onClick={() => setOpen(true)}>
            Новая заявка
          </Button>
        }
      />
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.isError && (
        <EmptyState title="Не удалось загрузить заявки" description={errorMessage(list.error)} />
      )}
      {list.data && (
        <DataTable
          columns={clientColumns((id) => close.mutate(id))}
          rows={list.data.items}
          rowKey={(row) => row.id}
          empty={
            <EmptyState
              title="Заявок нет"
              description="Опишите автомобиль — дилер заберёт заявку из пула."
            />
          }
        />
      )}
      <CreateRequestModal open={open} onClose={() => setOpen(false)} />
    </div>
  );
}

function clientColumns(onClose: (id: string) => void) {
  return [
    {
      key: 'n',
      header: '№',
      numeric: true,
      render: (row: RequestListItem) => String(row.number),
    },
    { key: 'sum', header: 'Суть', render: (row: RequestListItem) => row.summary },
    {
      key: 'st',
      header: 'Статус',
      render: (row: RequestListItem) => <Badge tone={requestTone(row.status)}>{row.status_title}</Badge>,
    },
    {
      key: 'reply',
      header: 'Ответ',
      render: (row: RequestListItem) => row.dealer_reply || '—',
    },
    { key: 'dt', header: 'Дата', render: (row: RequestListItem) => formatDate(row.created_at) },
    {
      key: 'act',
      header: '',
      render: (row: RequestListItem) =>
        row.status === 'new' || row.status === 'in_progress' || row.status === 'answered' ? (
          <Button size="sm" variant="ghost" onClick={() => onClose(row.id)}>
            Закрыть
          </Button>
        ) : null,
    },
  ];
}

function CreateRequestModal({ open, onClose }: { open: boolean; onClose: () => void }) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [brand, setBrand] = useState('');
  const [model, setModel] = useState('');
  const [origin, setOrigin] = useState('');
  const [budget, setBudget] = useState('');
  const [comment, setComment] = useState('');

  const create = useMutation({
    mutationFn: () =>
      requestsApi.create({
        ...(brand ? { desired_brand: brand } : {}),
        ...(model ? { desired_model: model } : {}),
        ...(origin ? { origin } : {}),
        ...(budget ? { budget_to_rub: Number(budget) } : {}),
        ...(comment ? { comment } : {}),
      }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Заявка создана');
      onClose();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  return (
    <Modal
      open={open}
      onClose={onClose}
      title="Заявка на подбор"
      footer={
        <>
          <Button onClick={onClose}>Отмена</Button>
          <Button variant="primary" loading={create.isPending} onClick={() => create.mutate()}>
            Отправить
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-3">
        <TextField label="Марка" value={brand} onChange={(event) => setBrand(event.target.value)} />
        <TextField label="Модель" value={model} onChange={(event) => setModel(event.target.value)} />
        <SelectField
          label="Рынок"
          placeholder="Не важно"
          value={origin}
          onChange={(event) => setOrigin(event.target.value)}
          options={[
            { value: 'cn', title: 'Китай' },
            { value: 'jp', title: 'Япония' },
          ]}
        />
        <TextField
          label="Бюджет до, ₽"
          numeric
          value={budget}
          onChange={(event) => setBudget(event.target.value)}
        />
        <TextAreaField
          label="Комментарий"
          value={comment}
          onChange={(event) => setComment(event.target.value)}
        />
      </div>
    </Modal>
  );
}

function DealerRequests() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const navigate = useNavigate();
  const [tab, setTab] = useState<'mine' | 'pool'>('pool');
  const [replyFor, setReplyFor] = useState<RequestListItem | null>(null);
  const [reply, setReply] = useState('');

  const mine = useQuery({
    queryKey: queryKeys.requestsDealer({}),
    queryFn: ({ signal }) => requestsApi.dealer({ limit: 50 }, signal),
  });
  const pool = useQuery({
    queryKey: queryKeys.requestsPool({}),
    queryFn: ({ signal }) => requestsApi.pool({ limit: 50 }, signal),
  });

  const claim = useMutation({
    mutationFn: (id: string) => requestsApi.claim(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Заявка взята в работу');
      setTab('mine');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const sendReply = useMutation({
    mutationFn: () => requestsApi.reply(replyFor?.id ?? '', reply),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      toast.success('Ответ отправлен');
      setReplyFor(null);
      setReply('');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const convert = useMutation({
    mutationFn: (row: RequestListItem) =>
      dealsApi.create({
        request_id: row.id,
        ...(row.car_id ? { car_id: row.car_id } : {}),
        title: row.summary,
      }),
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: ['requests'] });
      void queryClient.invalidateQueries({ queryKey: ['deals'] });
      void navigate(`/app/deals/${data.deal.id}`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const poolCount = pool.data?.total ?? pool.data?.items.length ?? 0;
  const mineCount = mine.data?.total ?? mine.data?.items.length ?? 0;
  const rows = tab === 'mine' ? (mine.data?.items ?? []) : (pool.data?.items ?? []);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Дилер"
        title="Заявки"
        description="Пул — свободные обращения покупателей, их можно взять в работу. «Мои» — уже ваши: из них открывается сделка в воронке."
      />
      <PageGuide
        items={[
          { title: 'Пул', text: 'Новые запросы без дилера. «Взять» закрепляет заявку за вами.' },
          { title: 'Мои', text: 'То, что уже ведёте. Ответ клиенту — в карточке заявки.' },
          { title: 'Сделка', text: 'Когда договорились — создайте сделку в воронке или из заявки.' },
        ]}
      />
      <div className="flex gap-2">
        <Button size="sm" variant={tab === 'pool' ? 'primary' : 'secondary'} onClick={() => setTab('pool')}>
          Пул{pool.data ? ` · ${poolCount}` : ''}
        </Button>
        <Button size="sm" variant={tab === 'mine' ? 'primary' : 'secondary'} onClick={() => setTab('mine')}>
          Мои{mine.data ? ` · ${mineCount}` : ''}
        </Button>
      </div>
      <DataTable
        columns={[
          { key: 'n', header: '№', numeric: true, render: (row) => String(row.number) },
          { key: 'client', header: 'Клиент', render: (row) => row.client_name || '—' },
          { key: 'sum', header: 'Суть', render: (row) => row.summary },
          {
            key: 'st',
            header: 'Статус',
            render: (row) => <Badge tone={requestTone(row.status)}>{row.status_title}</Badge>,
          },
          { key: 'dt', header: 'Дата', render: (row) => formatDate(row.created_at) },
          {
            key: 'act',
            header: '',
            render: (row) =>
              tab === 'pool' ? (
                <Button size="sm" onClick={() => claim.mutate(row.id)}>
                  Взять
                </Button>
              ) : (
                <div className="flex flex-wrap justify-end gap-1">
                  <Button size="sm" variant="ghost" onClick={() => setReplyFor(row)}>
                    Ответ
                  </Button>
                  {!row.has_deal && (
                    <Button size="sm" onClick={() => convert.mutate(row)}>
                      В сделку
                    </Button>
                  )}
                </div>
              ),
          },
        ]}
        rows={rows}
        rowKey={(row) => row.id}
        empty={
          <EmptyState
            title={tab === 'pool' ? 'Пул пуст' : 'Своих заявок нет'}
            description={
              tab === 'pool'
                ? 'Здесь появляются заявки покупателей по каталогу и без выбранного дилера. Адресные заявки с карточки импортёра — во вкладке «Мои».'
                : 'Возьмите заявку из пула кнопкой «Взять» — она окажется здесь, и из неё можно создать сделку.'
            }
          />
        }
      />
      <Modal
        open={Boolean(replyFor)}
        onClose={() => setReplyFor(null)}
        title={replyFor ? `Ответ по заявке № ${replyFor.number}` : 'Ответ'}
        footer={
          <>
            <Button onClick={() => setReplyFor(null)}>Отмена</Button>
            <Button variant="primary" loading={sendReply.isPending} onClick={() => sendReply.mutate()}>
              Отправить
            </Button>
          </>
        }
      >
        <TextAreaField
          label="Текст"
          value={reply}
          onChange={(event) => setReply(event.target.value)}
        />
      </Modal>
    </div>
  );
}
