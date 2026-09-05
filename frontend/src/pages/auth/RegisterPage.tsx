import { useState, type FormEvent } from 'react';
import { Link, useNavigate } from 'react-router-dom';

import { useAuth } from '@/features/auth/auth-context';
import { errorMessage, isApiError } from '@/lib/api';
import type { Role } from '@/lib/api';
import { Button, TextField, cn } from '@/ui';

const ROLES: { value: Exclude<Role, 'admin'>; title: string; hint: string }[] = [
  { value: 'client', title: 'Покупатель', hint: 'Каталог, заявка, этапы сделки до выдачи' },
  { value: 'dealer', title: 'Дилер-импортёр', hint: 'Воронка, лоты, реклама, поставщики' },
  { value: 'seller', title: 'Зарубежный продавец', hint: 'Карточка завода или аукциона для дилеров' },
];

export function RegisterPage() {
  const { register } = useAuth();
  const navigate = useNavigate();

  const [role, setRole] = useState<Exclude<Role, 'admin'>>('client');
  const [fullName, setFullName] = useState('');
  const [email, setEmail] = useState('');
  const [phone, setPhone] = useState('');
  const [password, setPassword] = useState('');
  const [error, setError] = useState('');
  const [fieldErrors, setFieldErrors] = useState<Record<string, string>>({});
  const [pending, setPending] = useState(false);

  async function onSubmit(event: FormEvent) {
    event.preventDefault();
    setError('');
    setFieldErrors({});
    setPending(true);
    try {
      await register({
        role,
        email: email.trim(),
        phone: phone.trim(),
        password,
        full_name: fullName.trim(),
      });
      void navigate(role === 'seller' ? '/app' : role === 'dealer' ? '/app/profile' : '/app', { replace: true });
    } catch (caught) {
      if (isApiError(caught) && Object.keys(caught.details).length > 0) {
        setFieldErrors(caught.details);
      } else {
        setError(errorMessage(caught));
      }
    } finally {
      setPending(false);
    }
  }

  return (
    <div className="mx-auto max-w-md px-4 py-10 md:py-16">
      <p className="text-sm font-medium text-[var(--accent)]">Площадка</p>
      <h1 className="mt-2 text-2xl font-semibold md:text-3xl">Регистрация</h1>
      <p className="mt-3 text-sm text-[var(--text-secondary)]">
        Сначала выберите роль — от неё зависит, что откроется в кабинете.
      </p>

      <form onSubmit={(event) => void onSubmit(event)} className="mt-8 flex flex-col gap-4">
        <fieldset className="grid gap-2">
          <legend className="mb-1 text-[13px] font-medium text-[var(--text-secondary)]">Кто вы</legend>
          {ROLES.map((item) => (
            <button
              key={item.value}
              type="button"
              onClick={() => setRole(item.value)}
              className={cn(
                'rounded-[var(--radius-sheet)] border px-3 py-3 text-left transition-colors',
                role === item.value
                  ? 'border-[var(--accent)] bg-[var(--surface-sunken)]'
                  : 'border-[var(--border-hairline)] hover:border-[var(--text-muted)]',
              )}
            >
              <span className="block text-sm font-medium">{item.title}</span>
              <span className="mt-0.5 block text-xs text-[var(--text-secondary)]">{item.hint}</span>
            </button>
          ))}
        </fieldset>
        <TextField
          label="Имя"
          autoComplete="name"
          required
          value={fullName}
          onChange={(event) => setFullName(event.target.value)}
          {...(fieldErrors.full_name ? { error: fieldErrors.full_name } : {})}
        />
        <TextField
          label="Почта"
          type="email"
          autoComplete="email"
          required
          value={email}
          onChange={(event) => setEmail(event.target.value)}
          {...(fieldErrors.email ? { error: fieldErrors.email } : {})}
        />
        <TextField
          label="Телефон"
          type="tel"
          autoComplete="tel"
          required
          value={phone}
          onChange={(event) => setPhone(event.target.value)}
          hint="В международном формате, начиная с +"
          {...(fieldErrors.phone ? { error: fieldErrors.phone } : {})}
        />
        <TextField
          label="Пароль"
          type="password"
          autoComplete="new-password"
          required
          minLength={10}
          value={password}
          onChange={(event) => setPassword(event.target.value)}
          hint="Не меньше 10 символов"
          {...(fieldErrors.password ? { error: fieldErrors.password } : {})}
        />
        {error && (
          <p className="text-sm text-lacquer-600" role="alert">
            {error}
          </p>
        )}
        <Button type="submit" variant="primary" loading={pending} block>
          Создать учётку
        </Button>
      </form>

      <p className="mt-6 text-sm text-[var(--text-secondary)]">
        Уже есть доступ?{' '}
        <Link to="/login" className="text-[var(--link)] underline underline-offset-4">
          Войти
        </Link>
      </p>
    </div>
  );
}
