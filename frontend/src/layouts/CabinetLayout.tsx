import { useMutation, useQuery, useQueryClient } from '@tanstack/react-query';
import { Link, NavLink, Outlet } from 'react-router-dom';
import { useState } from 'react';

import { useAuth } from '@/features/auth/auth-context';
import { notificationsApi } from '@/lib/api';
import type { Role } from '@/lib/api';
import { queryKeys } from '@/lib/query';
import { formatDateTime } from '@/lib/format';
import { Button, Drawer, cn } from '@/ui';

interface NavItem {
  to: string;
  label: string;
  hint: string;
  short?: string;
  end?: boolean;
}

interface NavGroup {
  label: string;
  items: readonly NavItem[];
}

const NAV_BY_ROLE: Record<Role, readonly NavGroup[]> = {
  client: [
    {
      label: 'Покупка',
      items: [
        { to: '/catalog', label: 'Каталог', hint: 'Машины с аукционов' },
        { to: '/app', label: 'Сделки', hint: 'Этапы вашей покупки', end: true },
        { to: '/app/requests', label: 'Заявки', hint: 'Запросы дилеру' },
        { to: '/app/favorites', label: 'Избранное', short: 'Лоты', hint: 'Отмеченные лоты' },
      ],
    },
    {
      label: 'Кабинет',
      items: [
        { to: '/app/help', label: 'Справка', hint: 'Как пользоваться' },
        { to: '/app/profile', label: 'Профиль', hint: 'Паспорт и вход' },
      ],
    },
  ],
  dealer: [
    {
      label: 'Клиенты',
      items: [
        { to: '/app', label: 'Воронка', hint: 'Сделки по этапам', end: true },
        { to: '/app/requests', label: 'Заявки', hint: 'Пул покупателей' },
      ],
    },
    {
      label: 'Витрина',
      items: [
        { to: '/app/cars', label: 'Объявления', short: 'Лоты', hint: 'Лоты в каталоге' },
        { to: '/app/banners', label: 'Реклама', hint: 'Баннеры на сайте' },
        { to: '/app/channels', label: 'Каналы', hint: 'Посты в соцсети' },
      ],
    },
    {
      label: 'Работа',
      items: [
        { to: '/app/sellers', label: 'Поставщики', hint: 'Китай и Япония' },
        { to: '/app/analytics', label: 'Аналитика', hint: 'Сводка и отчёты' },
      ],
    },
    {
      label: 'Кабинет',
      items: [
        { to: '/app/help', label: 'Справка', hint: 'Как пользоваться' },
        { to: '/app/profile', label: 'Профиль', hint: 'Компания и вход' },
      ],
    },
  ],
  seller: [
    {
      label: 'Кабинет',
      items: [
        { to: '/app', label: 'Карточка', hint: 'Ваш профиль для дилеров', end: true },
        { to: '/app/help', label: 'Справка', hint: 'Как пользоваться' },
        { to: '/app/profile', label: 'Профиль', hint: 'Контакты и вход' },
      ],
    },
  ],
  admin: [
    {
      label: 'Площадка',
      items: [
        { to: '/app', label: 'Сводка', hint: 'Цифры за день', end: true },
        { to: '/app/moderation', label: 'Модерация', short: 'Лоты', hint: 'Лоты и баннеры' },
        { to: '/app/users', label: 'Пользователи', short: 'Люди', hint: 'Роли и доступ' },
        { to: '/app/audit', label: 'Аудит', hint: 'Журнал действий' },
      ],
    },
    {
      label: 'Кабинет',
      items: [
        { to: '/app/help', label: 'Справка', hint: 'Как пользоваться' },
        { to: '/app/profile', label: 'Профиль', hint: 'Вход и сессии' },
      ],
    },
  ],
};

function flatten(groups: readonly NavGroup[]): NavItem[] {
  return groups.flatMap((group) => [...group.items]);
}

export function CabinetLayout() {
  const { user, logout } = useAuth();
  const groups = user ? NAV_BY_ROLE[user.role] : [];
  const items = flatten(groups);
  const [moreOpen, setMoreOpen] = useState(false);
  const primary = items.slice(0, 3);
  const extra = items.slice(3);

  return (
    <div className="flex min-h-dvh bg-[var(--surface)]">
      <aside className="hidden w-64 shrink-0 flex-col bg-[var(--nav)] text-[var(--nav-text)] lg:flex">
        <Link to="/" className="px-5 py-5">
          <span className="block font-display text-[17px] font-semibold tracking-tight">
            Импорт
          </span>
          <span className="mt-1 block text-xs text-[var(--nav-muted)]">Кабинет · Китай и Япония</span>
        </Link>
        <nav className="flex flex-1 flex-col gap-6 overflow-y-auto px-3 pb-4" aria-label="Кабинет">
          {groups.map((group) => (
            <div key={group.label}>
              <p className="px-2.5 pb-1.5 text-[11px] font-medium text-[var(--nav-muted)]">{group.label}</p>
              <div className="flex flex-col gap-0.5">
                {group.items.map((item) => (
                  <SideLink key={item.to} item={item} />
                ))}
              </div>
            </div>
          ))}
        </nav>
        {user && (
          <div className="border-t border-white/8 px-5 py-4">
            <p className="truncate text-sm font-medium">{user.full_name}</p>
            <p className="mt-0.5 text-xs text-[var(--nav-muted)]">{user.role_title}</p>
          </div>
        )}
      </aside>

      <div className="flex min-w-0 flex-1 flex-col">
        <header className="flex h-14 items-center justify-between gap-2 border-b border-[var(--border-hairline)] bg-[var(--surface-raised)] px-4 pt-[env(safe-area-inset-top)] sm:px-6 lg:h-16 lg:px-8">
          <Link to="/" className="font-display text-sm font-semibold lg:hidden">
            Импорт
          </Link>
          <div className="ml-auto flex items-center gap-1 sm:gap-2">
            <NotificationBell />
            <Button variant="ghost" size="sm" onClick={() => void logout()}>
              Выход
            </Button>
          </div>
        </header>

        <main className="w-full min-w-0 flex-1 px-4 py-6 pb-[calc(5rem+env(safe-area-inset-bottom))] sm:px-6 lg:px-8 lg:py-7 lg:pb-8">
          {user && !user.email_verified && (
            <p className="mb-5 rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] px-4 py-3 text-sm">
              Подтвердите почту в{' '}
              <Link to="/app/profile" className="text-[var(--link)] underline underline-offset-2">
                профиле
              </Link>
              , чтобы оставлять заявки и объявления.
            </p>
          )}
          <Outlet />
        </main>
      </div>

      <nav
        className="fixed inset-x-0 bottom-0 z-30 flex border-t border-[var(--border-hairline)] bg-[var(--surface-raised)] pb-[env(safe-area-inset-bottom)] lg:hidden"
        aria-label="Кабинет"
      >
        {primary.map((item) => (
          <NavLink
            key={item.to}
            to={item.to}
            {...(item.end ? { end: true } : {})}
            className={({ isActive }) =>
              cn(
                'flex min-h-12 flex-1 flex-col items-center justify-center px-1 py-2 text-center leading-tight',
                isActive ? 'text-[var(--text-primary)]' : 'text-[var(--text-muted)]',
              )
            }
          >
            <span className="text-xs">{item.short ?? item.label}</span>
          </NavLink>
        ))}
        {extra.length > 0 && (
          <button
            type="button"
            className="flex min-h-12 flex-1 items-center justify-center px-1 py-2 text-center text-xs leading-tight text-[var(--text-muted)]"
            onClick={() => setMoreOpen(true)}
          >
            Ещё
          </button>
        )}
      </nav>
      <Drawer open={moreOpen} onClose={() => setMoreOpen(false)} title="Разделы кабинета">
        <nav className="flex flex-col gap-4">
          {groups.map((group) => (
            <div key={group.label}>
              <p className="mb-1.5 text-xs font-medium text-[var(--text-muted)]">{group.label}</p>
              <div className="flex flex-col gap-1.5">
                {group.items.map((item) => (
                  <NavLink
                    key={item.to}
                    to={item.to}
                    {...(item.end ? { end: true } : {})}
                    onClick={() => setMoreOpen(false)}
                    className={({ isActive }) =>
                      cn(
                        'rounded-[var(--radius-sheet)] border border-[var(--border-hairline)] px-3 py-3',
                        isActive ? 'bg-[var(--surface-sunken)]' : 'bg-[var(--surface-raised)]',
                      )
                    }
                  >
                    <span className="block text-sm">{item.label}</span>
                    <span className="mt-0.5 block text-xs text-[var(--text-muted)]">{item.hint}</span>
                  </NavLink>
                ))}
              </div>
            </div>
          ))}
        </nav>
      </Drawer>
    </div>
  );
}

function SideLink({ item }: { item: NavItem }) {
  return (
    <NavLink
      to={item.to}
      {...(item.end ? { end: true } : {})}
      title={item.hint}
      className={({ isActive }) =>
        cn(
          'rounded-md px-2.5 py-2',
          isActive
            ? 'bg-[var(--nav-active)] text-[var(--nav-text)] shadow-[inset_2px_0_0_var(--accent)]'
            : 'text-[var(--nav-muted)] hover:bg-[var(--nav-raised)] hover:text-[var(--nav-text)]',
        )
      }
    >
      <span className="block text-sm font-medium">{item.label}</span>
      <span className="mt-0.5 block text-[11px] leading-snug opacity-70">{item.hint}</span>
    </NavLink>
  );
}

function NotificationBell() {
  const queryClient = useQueryClient();
  const [open, setOpen] = useState(false);

  const unread = useQuery({
    queryKey: queryKeys.unreadCount,
    queryFn: ({ signal }) => notificationsApi.unreadCount(signal),
    refetchInterval: 60_000,
  });

  const list = useQuery({
    queryKey: queryKeys.notifications(true),
    queryFn: ({ signal }) => notificationsApi.list({ unread: true, limit: 8 }, signal),
    enabled: open,
  });

  const markAll = useMutation({
    mutationFn: () => notificationsApi.markAllRead(),
    onSuccess: () => {
      void queryClient.invalidateQueries({ queryKey: queryKeys.unreadCount });
      void queryClient.invalidateQueries({ queryKey: ['notifications'] });
    },
  });

  const count = unread.data?.unread ?? 0;

  return (
    <div className="relative">
      <button
        type="button"
        onClick={() => setOpen((value) => !value)}
        className="relative h-11 min-w-11 px-2 text-xs text-[var(--text-secondary)] hover:text-[var(--text-primary)]"
        aria-expanded={open}
        aria-haspopup="true"
        aria-label={count > 0 ? `Уведомления, непрочитанных ${count}` : 'Уведомления'}
      >
        <span className="hidden sm:inline">Уведомления</span>
        <span className="sm:hidden">Увед.</span>
        {count > 0 && (
          <span className="ml-1 numeric text-[var(--accent)]">{count > 99 ? '99+' : count}</span>
        )}
      </button>
      {open && (
        <div className="absolute right-0 z-40 mt-1 w-[min(20rem,calc(100vw-1.5rem))] overflow-hidden rounded-[var(--radius-card)] border border-[var(--border-hairline)] bg-[var(--surface-raised)] shadow-[var(--shadow-raise)] max-md:fixed max-md:top-[calc(3.5rem+env(safe-area-inset-top))] max-md:right-3 max-md:left-3 max-md:w-auto">
          <div className="flex items-center justify-between border-b border-[var(--border-hairline)] px-3 py-2.5">
            <p className="text-xs font-medium text-[var(--text-muted)]">
              Непрочитанные
            </p>
            <button
              type="button"
              className="text-2xs text-[var(--link)] underline underline-offset-2"
              onClick={() => markAll.mutate()}
            >
              Прочитать все
            </button>
          </div>
          <ul className="max-h-80 overflow-y-auto">
            {(list.data?.items ?? []).length === 0 && (
              <li className="px-3 py-6 text-sm text-[var(--text-muted)]">Новых нет</li>
            )}
            {(list.data?.items ?? []).map((item) => (
              <li key={item.id} className="border-b border-[var(--border-hairline)] px-3 py-2 last:border-0">
                {item.link ? (
                  <Link to={item.link} className="block" onClick={() => setOpen(false)}>
                    <p className="text-sm font-medium">{item.title}</p>
                    {item.body && (
                      <p className="mt-0.5 text-xs text-[var(--text-secondary)]">{item.body}</p>
                    )}
                    <p className="mt-1 text-2xs text-[var(--text-muted)]">
                      {formatDateTime(item.created_at)}
                    </p>
                  </Link>
                ) : (
                  <>
                    <p className="text-sm font-medium">{item.title}</p>
                    {item.body && (
                      <p className="mt-0.5 text-xs text-[var(--text-secondary)]">{item.body}</p>
                    )}
                  </>
                )}
              </li>
            ))}
          </ul>
        </div>
      )}
    </div>
  );
}
