import { NavLink, Outlet, useLocation } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { cn, PageHeader } from '@/ui';

export function AnalyticsLayout() {
  const { hasRole } = useAuth();
  const location = useLocation();
  const reportsOpen = location.pathname.includes('/analytics/reports');

  return (
    <div className="flex flex-col gap-8">
      <PageHeader
        kicker={hasRole('dealer') ? 'Дилер' : 'Кабинет'}
        title="Аналитика"
        description="Сводка по воронке, рынкам и рекламе. Отдельные таблицы и выгрузка в CSV — во вкладке «Отчёты»."
      />
      <div className="flex flex-wrap gap-2" role="tablist" aria-label="Разделы аналитики">
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
    'inline-flex h-8 items-center justify-center box-border rounded-[var(--radius-sheet)] px-3 text-xs font-medium leading-none',
    active
      ? 'bg-[var(--accent)] text-white border border-transparent'
      : 'bg-[var(--surface-raised)] text-[var(--text-primary)] border border-[var(--border-hairline)] hover:bg-[var(--surface-sunken)]',
  );
}
