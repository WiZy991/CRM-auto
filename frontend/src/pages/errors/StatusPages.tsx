import { LinkButton } from '@/ui';

export function NotFoundPage() {
  return (
    <div className="mx-auto max-w-lg px-4 py-24">
      <p className="numeric text-sm text-[var(--text-muted)]">404</p>
      <h1 className="mt-2 text-3xl font-semibold">Страницы нет</h1>
      <p className="mt-3 text-sm text-[var(--text-secondary)]">
        Адрес не существует или объявление снято. Вернитесь в каталог.
      </p>
      <LinkButton to="/" variant="primary" className="mt-6">
        На главную
      </LinkButton>
    </div>
  );
}

export function ForbiddenPage() {
  return (
    <div className="mx-auto max-w-lg px-4 py-24">
      <p className="numeric text-sm text-[var(--text-muted)]">403</p>
      <h1 className="mt-2 text-3xl font-semibold">Нет доступа</h1>
      <p className="mt-3 text-sm text-[var(--text-secondary)]">
        Этот раздел открыт для другой роли. Если вы дилер, войдите под дилерской учёткой.
      </p>
      <LinkButton to="/app" className="mt-6">
        В кабинет
      </LinkButton>
    </div>
  );
}
