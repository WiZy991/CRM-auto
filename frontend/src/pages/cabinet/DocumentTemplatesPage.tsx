import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useState } from 'react';

import { documentTemplatesApi, errorMessage } from '@/lib/api';
import type { DocumentTemplate } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import {
  Button,
  EmptyState,
  Modal,
  PageGuide,
  PageHeader,
  SelectField,
  Spinner,
  TextField,
  useToast,
} from '@/ui';

const KIND_OPTIONS = [
  { value: 'contract', title: 'Договор' },
  { value: 'invoice', title: 'Инвойс' },
  { value: 'acceptance_act', title: 'Акт' },
  { value: 'other', title: 'Прочее' },
];

export function DocumentTemplatesPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [title, setTitle] = useState('Шаблон договора');
  const [kind, setKind] = useState('contract');
  const [file, setFile] = useState<File | null>(null);
  const [edit, setEdit] = useState<DocumentTemplate | null>(null);
  const [mapDraft, setMapDraft] = useState<Record<string, string>>({});

  const list = useQuery({
    queryKey: queryKeys.documentTemplates,
    queryFn: ({ signal }) => documentTemplatesApi.list(signal),
  });

  const upload = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('Выберите DOCX');
      return documentTemplatesApi.upload({ file, title, kind });
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.documentTemplates });
      toast.success('Шаблон загружен — проверьте маппинг полей');
      setFile(null);
      setEdit(data.template);
      setMapDraft({ ...data.template.field_map });
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const saveMap = useMutation({
    mutationFn: () => {
      if (!edit) throw new Error('нет шаблона');
      return documentTemplatesApi.update(edit.id, {
        title: edit.title,
        kind: edit.kind,
        ...(edit.stage ? { stage: edit.stage } : {}),
        field_map: mapDraft,
      });
    },
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.documentTemplates });
      toast.success('Маппинг сохранён');
      setEdit(null);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const remove = useMutation({
    mutationFn: (id: string) => documentTemplatesApi.remove(id),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.documentTemplates });
      toast.success('Шаблон удалён');
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  useEffect(() => {
    if (edit) setMapDraft({ ...edit.field_map });
  }, [edit?.id]);

  const fields = list.data?.fields ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Документы"
        title="Шаблоны DOCX"
        description="Загрузите свой договор с плейсхолдерами {{client.name}} или «ФИО». При формировании по сделке поля подставятся автоматически."
      />
      <PageGuide
        items={[
          { title: 'Плейсхолдеры', text: 'В Word: {{deal.amount}}, {{car.vin}} или русские подписи в «ёлочках».' },
          { title: 'Маппинг', text: 'После загрузки сверьте автосопоставление и поправьте вручную.' },
          { title: 'Сделка', text: 'Во вкладке «Документы» появится кнопка «Заполнить» у каждого шаблона.' },
        ]}
      />

      <section className="panel space-y-3 p-4">
        <h2 className="text-sm font-semibold">Загрузить DOCX</h2>
        <div className="grid gap-3 sm:grid-cols-2">
          <TextField label="Название" value={title} onChange={(e) => setTitle(e.target.value)} />
          <SelectField
            label="Вид"
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            options={KIND_OPTIONS}
          />
        </div>
        <input
          type="file"
          accept=".docx,application/vnd.openxmlformats-officedocument.wordprocessingml.document"
          onChange={(e) => setFile(e.target.files?.[0] ?? null)}
        />
        <Button
          variant="primary"
          size="sm"
          disabled={!file}
          loading={upload.isPending}
          onClick={() => upload.mutate()}
        >
          Загрузить и разобрать
        </Button>
      </section>

      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.isError && (
        <EmptyState title="Не удалось загрузить шаблоны" description={errorMessage(list.error)} />
      )}
      {list.data && list.data.items.length === 0 && (
        <EmptyState
          title="Шаблонов пока нет"
          description="Подготовьте DOCX с плейсхолдерами и загрузите выше."
        />
      )}
      <ul className="space-y-2">
        {(list.data?.items ?? []).map((item) => (
          <li
            key={item.id}
            className="panel flex flex-wrap items-center justify-between gap-3 p-4"
          >
            <div>
              <p className="font-medium">{item.title}</p>
              <p className="text-xs text-[var(--text-muted)]">
                {item.kind_title} · маркеров {item.placeholders.length} ·{' '}
                {Math.round(item.bytes / 1024)} КБ
              </p>
            </div>
            <div className="flex gap-2">
              <Button
                size="sm"
                onClick={() => {
                  setEdit(item);
                  setMapDraft({ ...item.field_map });
                }}
              >
                Поля
              </Button>
              <Button size="sm" variant="ghost" onClick={() => remove.mutate(item.id)}>
                Удалить
              </Button>
            </div>
          </li>
        ))}
      </ul>

      <Modal
        open={Boolean(edit)}
        onClose={() => setEdit(null)}
        title={edit ? `Поля: ${edit.title}` : 'Поля'}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Отмена</Button>
            <Button variant="primary" loading={saveMap.isPending} onClick={() => saveMap.mutate()}>
              Сохранить маппинг
            </Button>
          </>
        }
      >
        {edit && (
          <div className="max-h-[60vh] space-y-3 overflow-y-auto">
            <TextField
              label="Название шаблона"
              value={edit.title}
              onChange={(e) => setEdit({ ...edit, title: e.target.value })}
            />
            {(edit.placeholders.length ? edit.placeholders : Object.keys(mapDraft)).map((marker) => (
              <SelectField
                key={marker}
                label={marker}
                value={mapDraft[marker] ?? ''}
                onChange={(e) => setMapDraft((prev) => ({ ...prev, [marker]: e.target.value }))}
                options={[
                  { value: '', title: '— не заполнять —' },
                  ...fields.map((f) => ({ value: f.key, title: `${f.title} (${f.key})` })),
                ]}
              />
            ))}
            {edit.placeholders.length === 0 && (
              <p className="text-sm text-[var(--text-muted)]">
                Маркеры не найдены. Вставьте в DOCX вид `{'{{client.name}}'}` и загрузите снова.
              </p>
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
