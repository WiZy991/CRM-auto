import { useId } from 'react';
import type {
  InputHTMLAttributes,
  ReactNode,
  Ref,
  SelectHTMLAttributes,
  TextareaHTMLAttributes,
} from 'react';

import { cn } from './cn';

/**
 * Поля ввода.
 *
 * Подпись, подсказка и ошибка связаны с полем через id: без aria-describedby
 * экранный диктор прочитает «поле, редактируемое» и умолчит о том, что
 * форма его отвергла. Ошибка от сервера приходит в details и попадает сюда
 * же — сообщение об ошибке всегда рядом с полем, а не общим списком сверху.
 */

const CONTROL_BASE = cn(
  'w-full rounded-[var(--radius-sheet)] border bg-[var(--surface-raised)]',
  'px-3.5 text-base text-[var(--text-primary)] transition-colors md:text-sm',
  'placeholder:text-[var(--text-muted)] outline-none',
  'disabled:cursor-not-allowed disabled:bg-[var(--surface-sunken)] disabled:opacity-60',
);

function controlTone(invalid: boolean): string {
  return invalid
    ? 'border-lacquer-500 focus:border-lacquer-500'
    : 'border-[var(--border-hairline)] hover:border-[var(--text-muted)] focus:border-[var(--accent)]';
}

interface FieldShellProps {
  label?: string;
  hint?: string;
  error?: string;
  required?: boolean;
  htmlFor: string;
  describedBy: string;
  children: ReactNode;
  className?: string;
}

function FieldShell({
  label,
  hint,
  error,
  required,
  htmlFor,
  describedBy,
  children,
  className,
}: FieldShellProps) {
  return (
    <div className={cn('flex flex-col gap-1.5', className)}>
      {label && (
        <label
          htmlFor={htmlFor}
          className="text-[13px] font-medium text-[var(--text-secondary)]"
        >
          {label}
          {required && <span className="ml-1 text-lacquer-500">*</span>}
        </label>
      )}

      {children}

      {(error ?? hint) && (
        <p
          id={describedBy}
          className={cn('text-xs', error ? 'text-lacquer-600' : 'text-[var(--text-muted)]')}
          // Появление ошибки должно быть озвучено сразу, подсказка — нет.
          role={error ? 'alert' : undefined}
        >
          {error ?? hint}
        </p>
      )}
    </div>
  );
}

export interface TextFieldProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'id'> {
  label?: string;
  hint?: string;
  error?: string;
  /** Моноширинный набор для сумм, VIN и номеров лотов. */
  numeric?: boolean;
  className?: string;
  fieldClassName?: string;
  ref?: Ref<HTMLInputElement>;
}

export function TextField({
  label,
  hint,
  error,
  numeric,
  className,
  fieldClassName,
  required,
  ...rest
}: TextFieldProps) {
  const id = useId();
  const describedBy = `${id}-help`;

  return (
    <FieldShell
      {...(label !== undefined ? { label } : {})}
      {...(hint !== undefined ? { hint } : {})}
      {...(error !== undefined ? { error } : {})}
      {...(required !== undefined ? { required } : {})}
      htmlFor={id}
      describedBy={describedBy}
      {...(fieldClassName !== undefined ? { className: fieldClassName } : {})}
    >
      <input
        id={id}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ?? hint ? describedBy : undefined}
        className={cn(CONTROL_BASE, controlTone(Boolean(error)), 'h-11 md:h-10', numeric && 'numeric', className)}
        {...rest}
      />
    </FieldShell>
  );
}

export interface TextAreaFieldProps
  extends Omit<TextareaHTMLAttributes<HTMLTextAreaElement>, 'className' | 'id'> {
  label?: string;
  hint?: string;
  error?: string;
  className?: string;
  ref?: Ref<HTMLTextAreaElement>;
}

export function TextAreaField({
  label,
  hint,
  error,
  className,
  required,
  rows = 4,
  ...rest
}: TextAreaFieldProps) {
  const id = useId();
  const describedBy = `${id}-help`;

  return (
    <FieldShell
      {...(label !== undefined ? { label } : {})}
      {...(hint !== undefined ? { hint } : {})}
      {...(error !== undefined ? { error } : {})}
      {...(required !== undefined ? { required } : {})}
      htmlFor={id}
      describedBy={describedBy}
    >
      <textarea
        id={id}
        rows={rows}
        required={required}
        aria-invalid={error ? true : undefined}
        aria-describedby={error ?? hint ? describedBy : undefined}
        className={cn(CONTROL_BASE, controlTone(Boolean(error)), 'resize-y py-2.5', className)}
        {...rest}
      />
    </FieldShell>
  );
}

export interface SelectFieldProps
  extends Omit<SelectHTMLAttributes<HTMLSelectElement>, 'className' | 'id'> {
  label?: string;
  hint?: string;
  error?: string;
  options: readonly { value: string; title: string }[];
  placeholder?: string;
  className?: string;
  ref?: Ref<HTMLSelectElement>;
}

export function SelectField({
  label,
  hint,
  error,
  options,
  placeholder,
  className,
  required,
  ...rest
}: SelectFieldProps) {
  const id = useId();
  const describedBy = `${id}-help`;

  return (
    <FieldShell
      {...(label !== undefined ? { label } : {})}
      {...(hint !== undefined ? { hint } : {})}
      {...(error !== undefined ? { error } : {})}
      {...(required !== undefined ? { required } : {})}
      htmlFor={id}
      describedBy={describedBy}
    >
      <div className="relative">
        <select
          id={id}
          required={required}
          aria-invalid={error ? true : undefined}
          aria-describedby={error ?? hint ? describedBy : undefined}
          className={cn(
            CONTROL_BASE,
            controlTone(Boolean(error)),
            'h-11 cursor-pointer appearance-none pr-9 md:h-10',
            className,
          )}
          {...rest}
        >
          {placeholder && <option value="">{placeholder}</option>}
          {options.map((option) => (
            <option key={option.value} value={option.value}>
              {option.title}
            </option>
          ))}
        </select>

        <svg
          className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-[var(--text-muted)]"
          width="10"
          height="6"
          viewBox="0 0 10 6"
          fill="none"
          aria-hidden="true"
        >
          <path d="M1 1l4 4 4-4" stroke="currentColor" strokeWidth="1.5" strokeLinecap="square" />
        </svg>
      </div>
    </FieldShell>
  );
}

export interface CheckFieldProps
  extends Omit<InputHTMLAttributes<HTMLInputElement>, 'className' | 'id' | 'type'> {
  label: string;
  hint?: string;
  ref?: Ref<HTMLInputElement>;
}

export function CheckField({ label, hint, ...rest }: CheckFieldProps) {
  const id = useId();

  return (
    <div className="flex items-start gap-2.5">
      <input
        id={id}
        type="checkbox"
        className={cn(
          'mt-0.5 size-4 shrink-0 cursor-pointer appearance-none rounded-[4px]',
          'border border-[var(--border-hairline)] bg-[var(--surface-raised)]',
          'checked:border-[var(--accent)] checked:bg-[var(--accent)]',
          "checked:after:block checked:after:text-center checked:after:text-[10px] checked:after:leading-[14px] checked:after:text-paper-50 checked:after:content-['✓']",
          'disabled:opacity-50',
        )}
        {...rest}
      />
      <label htmlFor={id} className="cursor-pointer text-sm leading-tight select-none">
        {label}
        {hint && <span className="mt-0.5 block text-xs text-[var(--text-muted)]">{hint}</span>}
      </label>
    </div>
  );
}
