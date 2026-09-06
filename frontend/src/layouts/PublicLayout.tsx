import { Link, NavLink, Outlet } from 'react-router-dom';
import { useState } from 'react';

import { useAuth } from '@/features/auth/auth-context';
import { BrandLogo, Button, Drawer, LinkButton, cn } from '@/ui';

const NAV = [
  { to: '/catalog', label: 'Каталог' },
  { to: '/dealers', label: 'Дилеры' },
  { to: '/sellers', label: 'Поставщики' },
  { to: '/how-it-works', label: 'Как это работает' },
] as const;

export function PublicLayout() {
  const { status, user, logout } = useAuth();
  const [menuOpen, setMenuOpen] = useState(false);
  const authed = status === 'authenticated' && user;

  return (
    <div className="flex min-h-dvh flex-col">
      <header className="sticky top-0 z-20 border-b border-[var(--border-hairline)] bg-[var(--surface-raised)] pt-[env(safe-area-inset-top)]">
        <div className="mx-auto flex h-16 w-full max-w-[1600px] items-center justify-between gap-4 px-4 sm:px-6 lg:px-8">
          <BrandLogo
            className="min-w-0"
            subtitle="Китай · Япония"
            hideSubtitleOnMobile
          />

          <nav className="hidden items-center gap-5 md:flex" aria-label="Основное меню">
            {NAV.map((item) => (
              <NavLink
                key={item.to}
                to={item.to}
                className={({ isActive }) =>
                  cn(
                    'text-sm',
                    isActive ? 'text-[var(--text-primary)]' : 'text-[var(--text-secondary)]',
                    'hover:text-[var(--text-primary)]',
                  )
                }
              >
                {item.label}
              </NavLink>
            ))}
          </nav>

          <div className="flex shrink-0 items-center gap-1 sm:gap-2">
            <div className="md:hidden">
              <button
                type="button"
                className="inline-flex h-11 min-w-11 items-center justify-center px-2 text-sm text-[var(--text-secondary)]"
                onClick={() => setMenuOpen(true)}
              >
                Меню
              </button>
            </div>
            {authed ? (
              <>
                <div className="hidden md:block">
                  <Button variant="ghost" size="sm" onClick={() => void logout()}>
                    Выйти
                  </Button>
                </div>
                <LinkButton size="sm" variant="primary" to="/app">
                  <span className="md:hidden">Кабинет</span>
                  <span className="hidden md:inline">{cabinetLabel(user.role)}</span>
                </LinkButton>
              </>
            ) : (
              <>
                <div className="hidden items-center gap-2 md:flex">
                  <LinkButton variant="ghost" size="sm" to="/login">
                    Войти
                  </LinkButton>
                  <LinkButton size="sm" variant="primary" to="/register">
                    Регистрация
                  </LinkButton>
                </div>
                <div className="md:hidden">
                  <LinkButton size="sm" variant="primary" to="/login">
                    Войти
                  </LinkButton>
                </div>
              </>
            )}
          </div>
        </div>
      </header>

      <Drawer open={menuOpen} onClose={() => setMenuOpen(false)} title="Разделы">
        <nav className="flex flex-col gap-1" aria-label="Мобильное меню">
          {NAV.map((item) => (
            <NavLink
              key={item.to}
              to={item.to}
              onClick={() => setMenuOpen(false)}
              className={({ isActive }) =>
                cn(
                  'min-h-11 border border-[var(--border-hairline)] px-3 py-3 text-sm',
                  isActive ? 'bg-[var(--surface-sunken)]' : 'bg-[var(--surface)]',
                )
              }
            >
              {item.label}
            </NavLink>
          ))}
          {authed ? (
            <>
              <NavLink
                to="/app"
                onClick={() => setMenuOpen(false)}
                className="min-h-11 border border-[var(--border-hairline)] bg-[var(--surface)] px-3 py-3 text-sm"
              >
                {cabinetLabel(user.role)}
              </NavLink>
              <button
                type="button"
                className="min-h-11 border border-[var(--border-hairline)] bg-[var(--surface)] px-3 py-3 text-left text-sm"
                onClick={() => {
                  setMenuOpen(false);
                  void logout();
                }}
              >
                Выйти
              </button>
            </>
          ) : (
            <>
              <NavLink
                to="/login"
                onClick={() => setMenuOpen(false)}
                className="min-h-11 border border-[var(--border-hairline)] bg-[var(--surface)] px-3 py-3 text-sm"
              >
                Войти
              </NavLink>
              <NavLink
                to="/register"
                onClick={() => setMenuOpen(false)}
                className="min-h-11 border border-[var(--border-hairline)] bg-[var(--surface)] px-3 py-3 text-sm"
              >
                Регистрация
              </NavLink>
            </>
          )}
        </nav>
      </Drawer>

      <main className="flex-1">
        <Outlet />
      </main>

      <footer className="border-t border-[var(--border-hairline)] pb-[env(safe-area-inset-bottom)]">
        <div className="mx-auto flex w-full max-w-[1600px] flex-col gap-2 px-4 py-8 text-xs text-[var(--text-muted)] sm:px-6 md:flex-row md:items-center md:justify-between lg:px-8">
          <p>Площадка ввоза: каталог, дилеры, сделка с документами.</p>
          <nav className="flex flex-wrap gap-4">
            <Link to="/catalog" className="hover:text-[var(--text-primary)]">Каталог</Link>
            <Link to="/how-it-works" className="hover:text-[var(--text-primary)]">Как купить</Link>
            <Link to="/dealers" className="hover:text-[var(--text-primary)]">Дилеры</Link>
          </nav>
        </div>
      </footer>
    </div>
  );
}

function cabinetLabel(role: string): string {
  switch (role) {
    case 'dealer':
      return 'Кабинет дилера';
    case 'seller':
      return 'Кабинет продавца';
    case 'admin':
      return 'Админка';
    default:
      return 'Мои заявки';
  }
}
