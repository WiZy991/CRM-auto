import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState, type FormEvent } from 'react';
import { useParams } from 'react-router-dom';

import { carsApi, dealsApi, errorMessage, getSession, uploadsApi } from '@/lib/api';
import type { Deal, Stage } from '@/lib/api';
import { formatDateTime } from '@/lib/format';
import { queryKeys } from '@/lib/query';
import {
  Badge,
  Button,
  EmptyState,
  FullPageSpinner,
  LinkButton,
  PageHeader,
  SelectField,
  StageBar,
  TextAreaField,
  TextField,
  cn,
  useToast,
} from '@/ui';

const DOCUMENT_KINDS = [
  { value: 'contract', title: 'Договор' },
  { value: 'invoice', title: 'Инвойс' },
  { value: 'payment_order', title: 'Платёжное поручение' },
  { value: 'customs_declaration', title: 'Таможенная декларация' },
  { value: 'vehicle_certificate', title: 'Документ на автомобиль' },
  { value: 'acceptance_act', title: 'Акт приёма-передачи' },
  { value: 'passport', title: 'Паспорт' },
  { value: 'other', title: 'Прочее' },
] as const;

const PRINTABLE_KINDS = DOCUMENT_KINDS.filter((item) => item.value !== 'other');

type DealTab = 'stage' | 'docs' | 'chat' | 'more';

function dateInput(value?: string): string {
  return value ? value.slice(0, 10) : '';
}

function timelineItem(label: string, value?: string) {
  return { label, value: value ? formatDateTime(value) : '—' };
}

function stageFacts(deal: Deal): string {
  const bits: string[] = [];
  if (deal.amount_label) bits.push(deal.amount_label);
  if (deal.paid_share > 0) bits.push(`оплачено ${Math.round(deal.paid_share * 100)}%`);
  if (deal.destination_port) bits.push(deal.destination_port);
  if (deal.sbkts_number) bits.push(`СБКТС ${deal.sbkts_number}`);
  return bits.join(' · ');
}

export function DealDetailPage() {
  const { id = '' } = useParams();
  const toast = useToast();
  const queryClient = useQueryClient();
  const [tab, setTab] = useState<DealTab>('stage');
  const [message, setMessage] = useState('');
  const [stage, setStage] = useState('');
  const [comment, setComment] = useState('');
  const [taskTitle, setTaskTitle] = useState('');
  const [rating, setRating] = useState('5');
  const [reviewText, setReviewText] = useState('');
  const [lostReason, setLostReason] = useState('');
  const [docKind, setDocKind] = useState('contract');
  const [docTitle, setDocTitle] = useState('');
  const [docVisible, setDocVisible] = useState(true);
  const [amountRub, setAmountRub] = useState('');
  const [paidRub, setPaidRub] = useState('');
  const [handover, setHandover] = useState('');
  const [servicesNote, setServicesNote] = useState('');
  const [carId, setCarId] = useState('');
  const [destinationPort, setDestinationPort] = useState('');
  const [shippingTracking, setShippingTracking] = useState('');
  const [arrivedAt, setArrivedAt] = useState('');
  const [customsDuties, setCustomsDuties] = useState('');
  const [sbktsNumber, setSbktsNumber] = useState('');
  const [sbktsIssuedAt, setSbktsIssuedAt] = useState('');
  const [markContacted, setMarkContacted] = useState(false);

  const stages = useQuery({
    queryKey: queryKeys.stages,
    queryFn: ({ signal }) => dealsApi.stages(signal),
  });

  const myCars = useQuery({
    queryKey: ['cars', 'my', 'deal-picker'],
    queryFn: ({ signal }) => carsApi.mine({ limit: 50 }, signal),
    enabled: Boolean(id),
  });

  const details = useQuery({
    queryKey: queryKeys.deal(id),
    queryFn: ({ signal }) => dealsApi.get(id, signal),
    enabled: Boolean(id),
  });

  const messages = useQuery({
    queryKey: queryKeys.dealMessages(id),
    queryFn: ({ signal }) => dealsApi.messages(id, signal),
    enabled: Boolean(id),
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: queryKeys.deal(id) });
    void queryClient.invalidateQueries({ queryKey: queryKeys.dealMessages(id) });
    void queryClient.invalidateQueries({ queryKey: ['deals'] });
  };

  const send = useMutation({
    mutationFn: () => dealsApi.sendMessage(id, message),
    onSuccess: () => {
      setMessage('');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const changeStage = useMutation({
    mutationFn: () => dealsApi.changeStage(id, stage as Stage, comment),
    onSuccess: () => {
      setComment('');
      toast.success('Этап обновлён');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const closeDeal = useMutation({
    mutationFn: (input: { outcome: 'won' | 'lost'; reason: string }) =>
      dealsApi.close(id, input.outcome, input.reason),
    onSuccess: () => {
      setLostReason('');
      toast.success('Сделка закрыта');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const saveFinance = useMutation({
    mutationFn: () => {
      const amount = Number(amountRub.replace(/\s/g, '').replace(',', '.'));
      const paid = Number(paidRub.replace(/\s/g, '').replace(',', '.'));
      const duties = Number(customsDuties.replace(/\s/g, '').replace(',', '.'));
      return dealsApi.update(id, {
        ...(Number.isFinite(amount) && amount > 0 ? { amount_minor: Math.round(amount * 100) } : {}),
        ...(Number.isFinite(paid) && paid >= 0 ? { paid_rub_minor: Math.round(paid * 100) } : {}),
        ...(handover ? { expected_handover_at: `${handover}T00:00:00Z` } : {}),
        services_note: servicesNote,
        destination_port: destinationPort,
        shipping_tracking: shippingTracking,
        sbkts_number: sbktsNumber,
        ...(Number.isFinite(duties) && duties >= 0
          ? { customs_duties_rub_minor: Math.round(duties * 100) }
          : {}),
        ...(arrivedAt ? { arrived_at: `${arrivedAt}T12:00:00Z` } : { clear_arrived_at: true }),
        ...(sbktsIssuedAt
          ? { sbkts_issued_at: `${sbktsIssuedAt}T12:00:00Z` }
          : { clear_sbkts_issued_at: true }),
        ...(carId ? { car_id: carId } : { car_id: '' }),
        ...(markContacted && !details.data?.deal.first_contacted_at
          ? { first_contacted_at: new Date().toISOString() }
          : {}),
      });
    },
    onSuccess: () => {
      toast.success('Данные сделки сохранены');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const attachDoc = useMutation({
    mutationFn: async (file: File) => {
      const uploaded = await uploadsApi.image(file);
      return dealsApi.addDocument(id, {
        kind: docKind,
        title: docTitle.trim() || file.name,
        file_url: uploaded.url,
        mime_type: 'image/jpeg',
        visible_to_client: docVisible,
      });
    },
    onSuccess: () => {
      setDocTitle('');
      toast.success('Документ прикреплён');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const generateDocs = useMutation({
    mutationFn: (input: { kind?: string } = {}) =>
      dealsApi.generateDocuments(id, {
        visible_to_client: docVisible,
        ...(input.kind ? { kind: input.kind } : {}),
      }),
    onSuccess: (data, input) => {
      invalidate();
      const title = input.kind ? 'Документ сформирован' : 'Пакет документов сформирован';
      if (data.warnings.length > 0) {
        toast.info(title, data.warnings.join('. '));
      } else {
        toast.success(title);
      }
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const addTask = useMutation({
    mutationFn: () => dealsApi.createTask(id, { title: taskTitle }),
    onSuccess: () => {
      setTaskTitle('');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const toggleTask = useMutation({
    mutationFn: ({ taskId, done }: { taskId: string; done: boolean }) =>
      dealsApi.toggleTask(id, taskId, done),
    onSuccess: invalidate,
    onError: (error) => toast.error(errorMessage(error)),
  });

  const review = useMutation({
    mutationFn: () => dealsApi.leaveReview(id, Number(rating), reviewText),
    onSuccess: () => {
      toast.success('Отзыв опубликован');
      invalidate();
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  useEffect(() => {
    if (details.data?.next_stages[0]) {
      setStage(details.data.next_stages[0].value);
    } else {
      setStage('');
    }
    const deal = details.data?.deal;
    if (!deal) return;
    setAmountRub(deal.amount_minor != null ? String(Math.round(deal.amount_minor / 100)) : '');
    setPaidRub(deal.paid_rub_minor ? String(Math.round(deal.paid_rub_minor / 100)) : '');
    setHandover(deal.expected_handover_at ? deal.expected_handover_at.slice(0, 10) : '');
    setServicesNote(deal.services_note ?? '');
    setCarId(deal.car_id ?? '');
    setDestinationPort(deal.destination_port ?? '');
    setShippingTracking(deal.shipping_tracking ?? '');
    setArrivedAt(dateInput(deal.arrived_at));
    setCustomsDuties(
      deal.customs_duties_rub_minor != null
        ? String(Math.round(deal.customs_duties_rub_minor / 100))
        : '',
    );
    setSbktsNumber(deal.sbkts_number ?? '');
    setSbktsIssuedAt(dateInput(deal.sbkts_issued_at));
    setMarkContacted(Boolean(deal.first_contacted_at));
  }, [details.data?.deal.stage, details.data?.deal.updated_at, details.data?.next_stages]);

  if (details.isPending) return <FullPageSpinner label="Открываем сделку" />;
  if (details.isError || !details.data) {
    return (
      <EmptyState
        title="Сделка не найдена"
        description={errorMessage(details.error)}
        action={<LinkButton to="/app">К списку</LinkButton>}
      />
    );
  }

  const { deal, history, tasks, documents, next_stages, can_manage, can_review, review: existingReview } =
    details.data;
  const visibleDocs = documents.filter((doc) => can_manage || doc.visible_to_client);
  const facts = stageFacts(deal);

  const tabs: { id: DealTab; label: string }[] = [
    { id: 'stage', label: 'Этап' },
    { id: 'docs', label: 'Документы' },
    { id: 'chat', label: 'Переписка' },
    { id: 'more', label: 'Ещё' },
  ];

  function onSend(event: FormEvent) {
    event.preventDefault();
    if (message.trim()) send.mutate();
  }

  async function openAuthed(url: string) {
    const token = getSession()?.accessToken;
    const headers = new Headers();
    if (token) headers.set('Authorization', `Bearer ${token}`);
    try {
      const response = await fetch(url, { credentials: 'include', headers });
      if (!response.ok) {
        toast.error('Не удалось открыть документ');
        return;
      }
      const blob = await response.blob();
      const objectUrl = URL.createObjectURL(blob);
      window.open(objectUrl, '_blank', 'noopener');
    } catch {
      toast.error('Не удалось открыть документ');
    }
  }

  const lotOptions = [
    { value: '', title: 'Без привязки' },
    ...(myCars.data?.items ?? []).map((car) => ({
      value: car.id,
      title: car.title || `${car.brand ?? ''} ${car.model ?? ''}`.trim() || car.id,
    })),
  ];

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-4">
      <PageHeader
        kicker={`Сделка № ${deal.number}`}
        title={deal.title}
        description={`${deal.stage_title}${deal.amount_label ? ` · ${deal.amount_label}` : ''}`}
        actions={
          <div className="flex flex-wrap items-center gap-2">
            {deal.is_stale ? <Badge tone="amber">Зависла</Badge> : null}
            <LinkButton to="/app" size="sm">
              К воронке
            </LinkButton>
          </div>
        }
      />

      {stages.data && (
        <div className="panel space-y-2 p-3 sm:p-4">
          <StageBar stages={stages.data.items} current={deal.stage} stale={deal.is_stale} />
          {deal.normative_days != null && (
            <p className="text-xs text-[var(--text-muted)]">
              На этапе {deal.days_on_stage} из {deal.normative_days} дн. (ориентир)
            </p>
          )}
          <details className="text-sm">
            <summary className="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]">
              Даты этапов
            </summary>
            <ol className="mt-2 grid gap-2 sm:grid-cols-2 lg:grid-cols-3">
              {[
                timelineItem('Первый контакт', deal.first_contacted_at),
                timelineItem('Договор', deal.contract_signed_at),
                timelineItem('Оплата', deal.paid_at),
                timelineItem('Отгрузка', deal.shipped_at),
                timelineItem('Прибытие в порт', deal.arrived_at),
                timelineItem('Растаможка', deal.customs_cleared_at),
                timelineItem('Выдача', deal.handed_over_at),
              ].map((item) => (
                <li key={item.label} className="rounded-md bg-[var(--surface-muted)] px-3 py-2">
                  <p className="text-xs text-[var(--text-muted)]">{item.label}</p>
                  <p className="mt-0.5 font-medium">{item.value}</p>
                </li>
              ))}
            </ol>
          </details>
        </div>
      )}

      {can_manage && next_stages.length > 0 && (
        <section className="panel flex flex-wrap items-end gap-3 p-3 sm:p-4">
          <div className="w-full sm:w-48">
            <SelectField
              label="Следующий этап"
              value={stage}
              onChange={(event) => setStage(event.target.value)}
              placeholder="Выберите"
              options={next_stages}
            />
          </div>
          <div className="min-w-0 flex-1 sm:min-w-40">
            <TextField
              label="Комментарий"
              value={comment}
              onChange={(event) => setComment(event.target.value)}
            />
          </div>
          <Button
            variant="primary"
            className="w-full sm:w-auto"
            disabled={!stage}
            loading={changeStage.isPending}
            onClick={() => changeStage.mutate()}
          >
            Перевести
          </Button>
          <p className="w-full text-xs text-[var(--text-muted)]">
            К оплате — сумма. К привозу — оплата или платёжка. К выдаче — СБКТС или декларация.
          </p>
        </section>
      )}

      <div className="-mx-1 flex gap-1 overflow-x-auto px-1 pb-1 [scrollbar-width:thin]">
        {tabs.map((item) => (
          <button
            key={item.id}
            type="button"
            onClick={() => setTab(item.id)}
            className={cn(
              'shrink-0 rounded-[var(--radius-sheet)] px-3 py-2 text-sm font-medium',
              tab === item.id
                ? 'bg-[var(--accent)] text-[var(--accent-contrast,white)]'
                : 'bg-[var(--surface-muted)] text-[var(--text-secondary)] hover:text-[var(--text-primary)]',
            )}
          >
            {item.label}
          </button>
        ))}
      </div>

      {tab === 'stage' && (
        <section className="panel space-y-4 p-3 sm:p-4">
          {facts && (
            <p className="text-sm text-[var(--text-muted)]">
              <span className="text-[var(--text-secondary)]">Сейчас: </span>
              {facts}
            </p>
          )}

          {can_manage ? (
            <div className="grid gap-3 sm:grid-cols-2">
              {(deal.stage === 'lead' || !deal.first_contacted_at) && (
                <label className="flex items-center gap-2 text-sm sm:col-span-2">
                  <input
                    type="checkbox"
                    checked={markContacted}
                    onChange={(event) => setMarkContacted(event.target.checked)}
                  />
                  Первый контакт с клиентом состоялся
                </label>
              )}

              {(deal.stage === 'needs' || deal.stage === 'lead') && (
                <div className="sm:col-span-2">
                  <SelectField
                    label="Лот из каталога"
                    value={carId}
                    onChange={(event) => setCarId(event.target.value)}
                    placeholder="Без привязки"
                    options={lotOptions}
                  />
                </div>
              )}

              {(deal.stage === 'contract' || deal.stage === 'payment') && (
                <>
                  {deal.stage === 'contract' && (
                    <div className="sm:col-span-2">
                      <TextAreaField
                        label="Состав услуг по договору"
                        rows={3}
                        value={servicesNote}
                        onChange={(event) => setServicesNote(event.target.value)}
                      />
                    </div>
                  )}
                  <TextField
                    label="Сумма договора, ₽"
                    inputMode="decimal"
                    value={amountRub}
                    onChange={(event) => setAmountRub(event.target.value)}
                  />
                  {deal.stage === 'payment' && (
                    <>
                      <TextField
                        label="Оплачено, ₽"
                        inputMode="decimal"
                        value={paidRub}
                        onChange={(event) => setPaidRub(event.target.value)}
                      />
                      <p className="text-xs text-[var(--text-muted)] sm:col-span-2">
                        Платёжку приложите во вкладке «Документы».
                      </p>
                    </>
                  )}
                </>
              )}

              {deal.stage === 'shipping' && (
                <>
                  <TextField
                    label="Порт назначения"
                    value={destinationPort}
                    onChange={(event) => setDestinationPort(event.target.value)}
                  />
                  <TextField
                    label="Трекинг / путь"
                    value={shippingTracking}
                    onChange={(event) => setShippingTracking(event.target.value)}
                  />
                  <TextField
                    label="Прибытие в порт"
                    type="date"
                    value={arrivedAt}
                    onChange={(event) => setArrivedAt(event.target.value)}
                  />
                </>
              )}

              {deal.stage === 'customs' && (
                <>
                  <TextField
                    label="Пошлины и сборы, ₽"
                    inputMode="decimal"
                    value={customsDuties}
                    onChange={(event) => setCustomsDuties(event.target.value)}
                  />
                  <TextField
                    label="Номер СБКТС"
                    value={sbktsNumber}
                    onChange={(event) => setSbktsNumber(event.target.value)}
                  />
                  <TextField
                    label="Дата СБКТС"
                    type="date"
                    value={sbktsIssuedAt}
                    onChange={(event) => setSbktsIssuedAt(event.target.value)}
                  />
                  <p className="text-xs text-[var(--text-muted)] sm:col-span-2">
                    Декларацию загрузите во вкладке «Документы».
                  </p>
                </>
              )}

              {deal.stage === 'handover' && (
                <TextField
                  label="План выдачи"
                  type="date"
                  value={handover}
                  onChange={(event) => setHandover(event.target.value)}
                />
              )}

              <div className="flex justify-end sm:col-span-2">
                <Button
                  size="sm"
                  variant="primary"
                  loading={saveFinance.isPending}
                  onClick={() => saveFinance.mutate()}
                >
                  Сохранить
                </Button>
              </div>
            </div>
          ) : (
            <div className="grid gap-2 text-sm sm:grid-cols-2">
              {deal.services_note && (
                <p className="sm:col-span-2">
                  <span className="text-[var(--text-muted)]">Услуги: </span>
                  {deal.services_note}
                </p>
              )}
              {deal.destination_port && (
                <p>
                  <span className="text-[var(--text-muted)]">Порт: </span>
                  {deal.destination_port}
                </p>
              )}
              {deal.shipping_tracking && (
                <p className="sm:col-span-2">
                  <span className="text-[var(--text-muted)]">Путь: </span>
                  {deal.shipping_tracking}
                </p>
              )}
              {deal.sbkts_number && (
                <p>
                  <span className="text-[var(--text-muted)]">СБКТС: </span>
                  {deal.sbkts_number}
                </p>
              )}
              {!facts && !deal.services_note && (
                <p className="text-[var(--text-muted)] sm:col-span-2">Данных этапа пока нет.</p>
              )}
            </div>
          )}
        </section>
      )}

      {tab === 'docs' && (
        <section className="panel space-y-4 p-3 sm:p-4">
          <ul className="space-y-2 text-sm">
            {visibleDocs.map((doc) => (
              <li key={doc.id} className="flex flex-wrap items-baseline gap-2">
                <button
                  type="button"
                  className="text-[var(--link)] underline underline-offset-4"
                  onClick={() => void openAuthed(doc.url)}
                >
                  {doc.title}
                </button>
                <span className="text-xs text-[var(--text-muted)]">{doc.size_label}</span>
              </li>
            ))}
            {visibleDocs.length === 0 && (
              <li className="text-[var(--text-muted)]">Документов пока нет</li>
            )}
          </ul>

          {can_manage && (
            <div className="space-y-3 border-t border-[var(--border-hairline)] pt-3">
              <label className="flex items-center gap-2 text-sm">
                <input
                  type="checkbox"
                  checked={docVisible}
                  onChange={(event) => setDocVisible(event.target.checked)}
                />
                Показать клиенту
              </label>
              <Button
                variant="primary"
                size="sm"
                loading={generateDocs.isPending && !generateDocs.variables?.kind}
                onClick={() => generateDocs.mutate({})}
              >
                Сформировать пакет
              </Button>

              <details className="text-sm">
                <summary className="cursor-pointer text-[var(--text-muted)] hover:text-[var(--text-primary)]">
                  Отдельные формы
                </summary>
                <ul className="mt-2 flex flex-col gap-2">
                  {PRINTABLE_KINDS.map((kind) => {
                    const exists = documents.some((doc) => doc.kind === kind.value);
                    return (
                      <li
                        key={kind.value}
                        className="flex flex-wrap items-center justify-between gap-2 border border-[var(--border-hairline)] px-3 py-2"
                      >
                        <span>
                          {kind.title}
                          {exists ? (
                            <span className="ml-2 text-xs text-[var(--text-muted)]">есть</span>
                          ) : null}
                        </span>
                        <span className="flex flex-wrap gap-1">
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() =>
                              void openAuthed(
                                `/api/v1/deals/${id}/documents/preview?kind=${kind.value}`,
                              )
                            }
                          >
                            Просмотр
                          </Button>
                          <Button
                            size="sm"
                            loading={
                              generateDocs.isPending && generateDocs.variables?.kind === kind.value
                            }
                            onClick={() => generateDocs.mutate({ kind: kind.value })}
                          >
                            Заполнить
                          </Button>
                        </span>
                      </li>
                    );
                  })}
                </ul>
              </details>

              <form className="grid gap-2 sm:grid-cols-2" onSubmit={(event) => event.preventDefault()}>
                <SelectField
                  label="Скан или фото"
                  value={docKind}
                  onChange={(event) => setDocKind(event.target.value)}
                  options={DOCUMENT_KINDS}
                />
                <TextField
                  label="Название скана"
                  value={docTitle}
                  onChange={(event) => setDocTitle(event.target.value)}
                  placeholder="Необязательно"
                />
                <input
                  type="file"
                  accept="image/jpeg,image/png,image/webp"
                  className="sm:col-span-2 text-sm"
                  disabled={attachDoc.isPending}
                  onChange={(event) => {
                    const file = event.target.files?.[0];
                    if (file) attachDoc.mutate(file);
                    event.target.value = '';
                  }}
                />
              </form>
            </div>
          )}
        </section>
      )}

      {tab === 'chat' && (
        <section className="panel space-y-3 p-3 sm:p-4">
          <ul className="max-h-[22rem] space-y-3 overflow-y-auto">
            {(messages.data?.items ?? []).map((item) => (
              <li key={item.id} className={item.is_system ? 'text-xs text-[var(--text-muted)]' : ''}>
                <p className="text-2xs text-[var(--text-muted)]">
                  {item.author_name || 'Система'} · {formatDateTime(item.created_at)}
                </p>
                <p className="mt-1 text-sm whitespace-pre-wrap">{item.body}</p>
              </li>
            ))}
            {(messages.data?.items ?? []).length === 0 && (
              <li className="text-sm text-[var(--text-muted)]">Сообщений пока нет.</li>
            )}
          </ul>
          <form
            onSubmit={(event) => void onSend(event)}
            className="flex flex-col gap-2 border-t border-[var(--border-hairline)] pt-3 sm:flex-row sm:items-end"
          >
            <div className="flex-1">
              <TextAreaField
                rows={2}
                value={message}
                onChange={(event) => setMessage(event.target.value)}
                placeholder="Сообщение"
              />
            </div>
            <Button type="submit" variant="primary" loading={send.isPending}>
              Отправить
            </Button>
          </form>
        </section>
      )}

      {tab === 'more' && (
        <div className="flex flex-col gap-4">
          <section className="panel space-y-3 p-3 sm:p-4">
            <h2 className="text-sm font-medium">Задачи</h2>
            <ul className="space-y-2">
              {tasks.map((task) => (
                <li key={task.id} className="flex items-center justify-between gap-2 text-sm">
                  <span className={task.done_at ? 'text-[var(--text-muted)] line-through' : ''}>
                    {task.title}
                    {task.overdue && !task.done_at && (
                      <Badge className="ml-2" tone="danger">
                        Просрочена
                      </Badge>
                    )}
                  </span>
                  {can_manage && (
                    <Button
                      size="sm"
                      variant="ghost"
                      onClick={() => toggleTask.mutate({ taskId: task.id, done: !task.done_at })}
                    >
                      {task.done_at ? 'Вернуть' : 'Готово'}
                    </Button>
                  )}
                </li>
              ))}
              {tasks.length === 0 && (
                <li className="text-sm text-[var(--text-muted)]">Задач нет</li>
              )}
            </ul>
            {can_manage && (
              <form
                className="flex flex-col gap-2 sm:flex-row sm:items-end"
                onSubmit={(event) => {
                  event.preventDefault();
                  if (taskTitle.trim()) addTask.mutate();
                }}
              >
                <TextField
                  placeholder="Новая задача"
                  value={taskTitle}
                  onChange={(event) => setTaskTitle(event.target.value)}
                  fieldClassName="flex-1"
                />
                <Button type="submit" size="sm">
                  Добавить
                </Button>
              </form>
            )}
          </section>

          <section className="panel space-y-2 p-3 sm:p-4">
            <h2 className="text-sm font-medium">История этапов</h2>
            <ol className="max-h-48 space-y-2 overflow-y-auto text-sm">
              {history.map((entry, index) => (
                <li key={`${entry.created_at}-${index}`}>
                  <p className="font-medium">{entry.stage_title}</p>
                  <p className="text-xs text-[var(--text-muted)]">
                    {entry.changed_by} · {formatDateTime(entry.created_at)}
                    {entry.comment ? ` · ${entry.comment}` : ''}
                  </p>
                </li>
              ))}
            </ol>
          </section>

          {can_manage && deal.outcome === 'open' && (
            <section className="panel flex flex-wrap items-end gap-3 p-3 sm:p-4">
              <div className="min-w-0 flex-1 sm:min-w-56">
                <TextField
                  label="Причина отказа"
                  value={lostReason}
                  onChange={(event) => setLostReason(event.target.value)}
                />
              </div>
              <Button
                className="w-full sm:w-auto"
                loading={closeDeal.isPending}
                onClick={() => closeDeal.mutate({ outcome: 'lost', reason: lostReason })}
              >
                Закрыть как отказ
              </Button>
              {deal.stage === 'handover' && (
                <Button
                  variant="primary"
                  className="w-full sm:w-auto"
                  loading={closeDeal.isPending}
                  onClick={() => closeDeal.mutate({ outcome: 'won', reason: '' })}
                >
                  Выдана, закрыть
                </Button>
              )}
            </section>
          )}

          {(existingReview || can_review) && (
            <section className="panel space-y-3 p-3 sm:p-4">
              <h2 className="text-sm font-medium">Отзыв</h2>
              {existingReview && (
                <p className="text-sm">
                  Оценка {existingReview.rating} из 5
                  {existingReview.text ? ` — ${existingReview.text}` : ''}
                </p>
              )}
              {can_review && (
                <form
                  className="flex flex-col gap-2"
                  onSubmit={(event) => {
                    event.preventDefault();
                    review.mutate();
                  }}
                >
                  <SelectField
                    label="Оценка"
                    value={rating}
                    onChange={(event) => setRating(event.target.value)}
                    options={[
                      { value: '5', title: '5 — отлично' },
                      { value: '4', title: '4' },
                      { value: '3', title: '3' },
                      { value: '2', title: '2' },
                      { value: '1', title: '1' },
                    ]}
                  />
                  <TextAreaField
                    label="Комментарий"
                    value={reviewText}
                    onChange={(event) => setReviewText(event.target.value)}
                  />
                  <Button type="submit" size="sm" loading={review.isPending}>
                    Оставить отзыв
                  </Button>
                </form>
              )}
            </section>
          )}
        </div>
      )}
    </div>
  );
}
