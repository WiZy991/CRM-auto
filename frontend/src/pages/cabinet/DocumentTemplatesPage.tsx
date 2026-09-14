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

  const fields = list.data?.fields ?? [];

  const upload = useMutation({
    mutationFn: () => {
      if (!file) throw new Error('Выберите DOCX');
      return documentTemplatesApi.upload({ file, title, kind });
    },
    onSuccess: (data) => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.documentTemplates });
      const n = data.template.placeholders.length;
      toast.success(
        n > 0
          ? `Шаблон загружен, найдено маркеров: ${n}. При заполнении подставятся все {{поля}} разом.`
          : 'Шаблон загружен. Вставьте теги {{…}} из списка ниже — иначе в договоре нечего менять.',
      );
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
      toast.success('Сохранено');
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

  async function copyAllTags() {
    const text = fields.map((f) => `{{${f.key}}}`).join('\n');
    try {
      await navigator.clipboard.writeText(text);
      toast.success('Все теги скопированы — вставьте в Word в нужные места');
    } catch {
      toast.error('Не удалось скопировать');
    }
  }

  async function copyTag(key: string) {
    try {
      await navigator.clipboard.writeText(`{{${key}}}`);
      toast.success(`Скопировано {{${key}}}`);
    } catch {
      toast.error('Не удалось скопировать');
    }
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Документы"
        title="Шаблоны DOCX"
        description="В договор в Word вставляете теги вида {{client.name}}, {{deal.amount}}, {{car.vin}}. Кнопка «Заполнить» в сделке подставляет сразу все поля CRM — не по одному."
      />
      <PageGuide
        items={[
          {
            title: 'Теги в Word',
            text: 'Скопируйте теги из списка ниже и вставьте вместо пустых мест в договоре. Обычный текст «ФИО» без {{ }} система не угадает.',
          },
          {
            title: 'Заполнить',
            text: 'В карточке сделки → Документы → «Заполнить» у шаблона. Меняются все {{поля}} разом из данных сделки.',
          },
          {
            title: 'Маппинг',
            text: 'Нужен только если в файле свои маркеры вроде «ФИО клиента» — сопоставьте их с полем CRM.',
          },
        ]}
      />

      <section className="panel space-y-3 p-4">
        <div className="flex flex-wrap items-end justify-between gap-2">
          <div>
            <h2 className="text-sm font-semibold">Поля сделки (все сразу)</h2>
            <p className="mt-1 text-sm text-[var(--text-muted)]">
              Это не «выберите 2 поля». При заполнении подставляется весь каталог, где в DOCX есть
              соответствующий тег.
            </p>
          </div>
          <Button size="sm" onClick={() => void copyAllTags()} disabled={fields.length === 0}>
            Скопировать все теги
          </Button>
        </div>
        {list.isPending && <Spinner className="text-[var(--accent)]" />}
        <ul className="grid max-h-64 gap-1 overflow-y-auto sm:grid-cols-2">
          {fields.map((f) => (
            <li key={f.key} className="flex items-center justify-between gap-2 text-sm">
              <span className="min-w-0 truncate text-[var(--text-secondary)]">{f.title}</span>
              <button
                type="button"
                className="shrink-0 font-mono text-xs text-[var(--link)] underline underline-offset-2"
                onClick={() => void copyTag(f.key)}
              >
                {`{{${f.key}}}`}
              </button>
            </li>
          ))}
        </ul>
      </section>

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
          Загрузить
        </Button>
      </section>

      {list.isError && (
        <EmptyState title="Не удалось загрузить шаблоны" description={errorMessage(list.error)} />
      )}
      {list.data && list.data.items.length === 0 && (
        <EmptyState
          title="Шаблонов пока нет"
          description="Вставьте теги в договор, сохраните DOCX и загрузите файл."
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
                {item.kind_title} · своих маркеров {item.placeholders.length} ·{' '}
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
                Доп. маппинг
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
        title={edit ? `Шаблон: ${edit.title}` : 'Шаблон'}
        footer={
          <>
            <Button onClick={() => setEdit(null)}>Отмена</Button>
            <Button variant="primary" loading={saveMap.isPending} onClick={() => saveMap.mutate()}>
              Сохранить
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
            <p className="text-sm text-[var(--text-muted)]">
              Стандартные теги <code className="text-xs">{'{{client.name}}'}</code> и остальные из
              каталога подставляются сами. Ниже — только нестандартные маркеры из вашего файла.
            </p>
            {(edit.placeholders.length ? edit.placeholders : Object.keys(mapDraft)).length === 0 ? (
              <p className="text-sm text-[var(--text-muted)]">
                Нестандартных маркеров нет — достаточно тегов из каталога в Word.
              </p>
            ) : (
              (edit.placeholders.length ? edit.placeholders : Object.keys(mapDraft)).map((marker) => (
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
              ))
            )}
          </div>
        )}
      </Modal>
    </div>
  );
}
