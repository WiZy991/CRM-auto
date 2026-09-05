import { NavLink, Outlet, useLocation } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { cn, PageHeader } from '@/ui';

export function AnalyticsLayout() {
  const { hasRole } = useAuth();
  const location = useLocation();
  const reportsOpen = location.pathname.includes('/analytics/reports');

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        kicker={hasRole('dealer') ? 'Дилер' : 'Кабинет'}
        title="Аналитика"
        description={
          reportsOpen
            ? 'Таблицы по сделкам, воронке и рекламе. Из любого отчёта можно выгрузить CSV.'
            : 'Главные цифры по воронке. Подробные таблицы и CSV — во вкладке «Отчёты».'
        }
      />
      <div
        className="flex gap-1 border-b border-[var(--border-hairline)]"
        role="tablist"
        aria-label="Разделы аналитики"
      >
        <NavLink
          to="/app/analytics"
          end
          role="tab"
          aria-selected={!reportsOpen}
          className={({ isActive }) => tabClass(isActive)}
        >
          Сводка
        </NavLink>
        <NavLink
          to="/app/analytics/reports"
          role="tab"
          aria-selected={reportsOpen}
          className={({ isActive }) => tabClass(isActive)}
        >
          Отчёты
        </NavLink>
      </div>
      <Outlet />
    </div>
  );
}

function tabClass(active: boolean) {
  return cn(
    'relative -mb-px inline-flex h-10 items-center px-4 text-sm font-medium',
    active
      ? 'text-[var(--text-primary)] after:absolute after:inset-x-2 after:bottom-0 after:h-0.5 after:bg-[var(--accent)]'
      : 'text-[var(--text-muted)] hover:text-[var(--text-primary)]',
  );
}
