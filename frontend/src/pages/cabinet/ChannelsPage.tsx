import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { useEffect, useMemo, useState } from 'react';
import { useSearchParams } from 'react-router-dom';

import { channelsApi, errorMessage } from '@/lib/api';
import type { SocialChannel, SocialNetwork } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import { Badge, Button, CheckField, EmptyState, PageGuide, PageHeader, Spinner, TextField, useToast } from '@/ui';
import type { BadgeTone } from '@/ui';

import { CHANNEL_GUIDES } from './channelGuides';

const STATUS_LABEL: Record<SocialChannel['status'], string> = {
  disconnected: 'Не подключено',
  connected: 'Связь есть',
  needs_reauth: 'Обновите ключ',
  error: 'Ошибка API',
};

const STATUS_TONE: Record<SocialChannel['status'], BadgeTone> = {
  disconnected: 'neutral',
  connected: 'jade',
  needs_reauth: 'amber',
  error: 'danger',
};

interface Draft {
  token: string;
  apiKey: string;
  chatId: string;
  ownerId: string;
  phoneNumberId: string;
  businessAccountId: string;
  destination: string;
  autoPost: boolean;
}

function draftFrom(channel: SocialChannel): Draft {
  return {
    token: '',
    apiKey: '',
    chatId: channel.chat_id ?? '',
    ownerId: channel.owner_id ?? '',
    phoneNumberId: channel.phone_number_id ?? '',
    businessAccountId: channel.business_account_id ?? '',
    destination: channel.destination ?? '',
    autoPost: channel.auto_post,
  };
}

export function ChannelsPage() {
  const toast = useToast();
  const queryClient = useQueryClient();
  const [params, setParams] = useSearchParams();
  const [drafts, setDrafts] = useState<Partial<Record<SocialNetwork, Draft>>>({});

  const list = useQuery({
    queryKey: queryKeys.dealerChannels,
    queryFn: ({ signal }) => channelsApi.list(signal),
  });

  useEffect(() => {
    if (!list.data) return;
    setDrafts((prev) => {
      const next = { ...prev };
      for (const channel of list.data.items) {
        if (!next[channel.network]) next[channel.network] = draftFrom(channel);
      }
      return next;
    });
  }, [list.data]);

  useEffect(() => {
    const connected = params.get('connected');
    const oauth = params.get('oauth');
    if (connected) {
      toast.success(`${connected}: аккаунт подключён`);
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerChannels });
    } else if (oauth === 'error') {
      toast.error('Не удалось завершить подключение. Проверьте, что Instagram — Business, а у Google есть канал YouTube.');
    }
    if (connected || oauth) {
      setParams({}, { replace: true });
    }
  }, [params, queryClient, setParams, toast]);

  const items = list.data?.items ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker="Дилер"
        title="Каналы"
        description="Соцсети, куда уходит лот после модерации: Telegram, ВКонтакте, WhatsApp, Instagram, YouTube и RuTube. Ключ или кнопка «Подключить», затем проверка связи."
      />
      <PageGuide
        items={[
          { title: 'Ключ или вход', text: 'Telegram, VK, WhatsApp, RuTube — вставить токен. Instagram и YouTube — «Подключить».' },
          { title: 'Проверить связь', text: 'Зелёный статус значит, что CRM ходит в ваш аккаунт. Инструкция — в «Как получить».' },
          { title: 'Пост с лота', text: 'Автопост после одобрения объявления. Вручную — кнопка «Опубликовать» в объявлениях.' },
        ]}
      />
      {list.isPending && <Spinner className="text-[var(--accent)]" />}
      {list.isError && (
        <EmptyState title="Не удалось загрузить каналы" description={errorMessage(list.error)} />
      )}
      {items.map((channel) => (
        <ChannelCard
          key={channel.network}
          channel={channel}
          draft={drafts[channel.network] ?? draftFrom(channel)}
          onDraft={(patch) =>
            setDrafts((prev) => ({
              ...prev,
              [channel.network]: { ...(prev[channel.network] ?? draftFrom(channel)), ...patch },
            }))
          }
        />
      ))}
    </div>
  );
}

function ChannelCard({
  channel,
  draft,
  onDraft,
}: {
  channel: SocialChannel;
  draft: Draft;
  onDraft: (patch: Partial<Draft>) => void;
}) {
  const toast = useToast();
  const queryClient = useQueryClient();
  const guide = CHANNEL_GUIDES[channel.network];

  const save = useMutation({
    mutationFn: () =>
      channelsApi.save({
        network: channel.network,
        ...(draft.token ? { token: draft.token } : {}),
        ...(draft.apiKey ? { api_key: draft.apiKey } : {}),
        chat_id: draft.chatId,
        owner_id: draft.ownerId,
        phone_number_id: draft.phoneNumberId,
        business_account_id: draft.businessAccountId,
        destination: draft.destination,
        auto_post: draft.autoPost,
      }),
    onSuccess: () => {
      onDraft({ token: '', apiKey: '' });
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerChannels });
      toast.success(`${channel.title}: ключ сохранён`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const test = useMutation({
    mutationFn: () => channelsApi.test(channel.network),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerChannels });
      toast.success(`${channel.title}: связь есть`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const disconnect = useMutation({
    mutationFn: () => channelsApi.save({ network: channel.network, disconnect: true }),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.dealerChannels });
      toast.success(`${channel.title}: отключён`);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const oauth = useMutation({
    mutationFn: () => channelsApi.oauthStart(channel.network),
    onSuccess: (data) => {
      window.location.assign(data.url);
    },
    onError: (error) => toast.error(errorMessage(error)),
  });

  const oauthLabel = useMemo(() => {
    if (channel.network === 'instagram') return 'Подключить Instagram';
    if (channel.network === 'youtube') return 'Подключить YouTube';
    return 'Подключить';
  }, [channel.network]);

  return (
    <section className="panel p-4">
      <div className="flex flex-wrap items-start justify-between gap-3">
        <div>
          <h2 className="text-base font-semibold">{channel.title}</h2>
          {channel.external_id ? (
            <p className="mt-0.5 text-xs text-[var(--text-muted)]">{channel.external_id}</p>
          ) : null}
        </div>
        <Badge tone={STATUS_TONE[channel.status]}>{STATUS_LABEL[channel.status]}</Badge>
      </div>
      {channel.platform_hint ? (
        <p className="mt-3 border border-[var(--border-hairline)] bg-[var(--surface-sunken)] px-3 py-2 text-sm">
          {channel.platform_hint}
        </p>
      ) : null}
      {channel.publish_hint ? (
        <p className="mt-2 text-sm text-[var(--text-secondary)]">{channel.publish_hint}</p>
      ) : null}
      {channel.last_error ? (
        <p className="mt-2 text-sm text-lacquer-600">{channel.last_error}</p>
      ) : null}
      {channel.token_mask ? (
        <p className="mt-2 text-xs text-[var(--text-muted)]">Ключ принят: {channel.token_mask}</p>
      ) : null}

      <div className="mt-4 grid gap-3 md:grid-cols-2">
        {channel.auth_kind === 'keys' && channel.network !== 'rutube' ? (
          <TextField
            label={channel.network === 'vk' ? 'Токен сообщества' : 'Токен'}
            type="password"
            autoComplete="off"
            value={draft.token}
            placeholder={channel.token_mask ? 'новый ключ, если меняете' : ''}
            onChange={(event) => onDraft({ token: event.target.value })}
          />
        ) : null}
        {channel.network === 'rutube' ? (
          <TextField
            label="API key / user token"
            type="password"
            autoComplete="off"
            value={draft.apiKey || draft.token}
            placeholder={channel.token_mask ? 'новый ключ, если меняете' : ''}
            onChange={(event) => onDraft({ apiKey: event.target.value, token: event.target.value })}
          />
        ) : null}
        {channel.network === 'telegram' ? (
          <TextField
            label="@канал или chat_id"
            value={draft.chatId}
            onChange={(event) => onDraft({ chatId: event.target.value })}
          />
        ) : null}
        {channel.network === 'vk' ? (
          <TextField
            label="owner_id группы"
            hint="Отрицательное число, например -123456"
            value={draft.ownerId}
            onChange={(event) => onDraft({ ownerId: event.target.value })}
          />
        ) : null}
        {channel.network === 'whatsapp' ? (
          <>
            <TextField
              label="phone_number_id"
              value={draft.phoneNumberId}
              onChange={(event) => onDraft({ phoneNumberId: event.target.value })}
            />
            <TextField
              label="business_account_id"
              value={draft.businessAccountId}
              onChange={(event) => onDraft({ businessAccountId: event.target.value })}
            />
            <TextField
              label="Куда постить лоты"
              hint="Channel id или номер чата. Пусто — лоты не отправлять."
              value={draft.destination}
              onChange={(event) => onDraft({ destination: event.target.value })}
            />
          </>
        ) : null}
      </div>

      <div className="mt-4">
        <CheckField
          label="Автопост лота после модерации"
          checked={draft.autoPost}
          onChange={(event) => onDraft({ autoPost: event.target.checked })}
        />
      </div>

      <div className="mt-4 flex flex-wrap gap-2">
        {channel.auth_kind === 'oauth' ? (
          <Button
            variant="primary"
            size="sm"
            disabled={!channel.platform_ready || oauth.isPending}
            onClick={() => oauth.mutate()}
          >
            {oauthLabel}
          </Button>
        ) : (
          <Button variant="primary" size="sm" disabled={save.isPending} onClick={() => save.mutate()}>
            Сохранить
          </Button>
        )}
        {channel.auth_kind === 'oauth' ? (
          <Button size="sm" disabled={save.isPending} onClick={() => save.mutate()}>
            Сохранить настройки
          </Button>
        ) : null}
        <Button size="sm" disabled={test.isPending} onClick={() => test.mutate()}>
          Проверить связь
        </Button>
        {channel.status !== 'disconnected' ? (
          <Button size="sm" disabled={disconnect.isPending} onClick={() => disconnect.mutate()}>
            Отключить
          </Button>
        ) : null}
      </div>

      <details className="mt-4 border-t border-[var(--border-hairline)] pt-3">
        <summary className="cursor-pointer text-sm font-medium">Как получить</summary>
        <p className="mt-2 text-sm text-[var(--text-secondary)]">{guide.why}</p>
        <ol className="mt-3 list-decimal space-y-1.5 pl-5 text-sm text-[var(--text-secondary)]">
          {guide.steps.map((step) => (
            <li key={step}>{step}</li>
          ))}
        </ol>
        <p className="mt-3 text-xs font-medium uppercase tracking-wide text-[var(--text-muted)]">Типичные ошибки</p>
        <ul className="mt-1 list-disc space-y-1 pl-5 text-sm text-[var(--text-secondary)]">
          {guide.errors.map((item) => (
            <li key={item}>{item}</li>
          ))}
        </ul>
      </details>
    </section>
  );
}
