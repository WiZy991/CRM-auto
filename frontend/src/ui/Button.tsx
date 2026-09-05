import type { ButtonHTMLAttributes, ReactNode, Ref } from 'react';
import { Link } from 'react-router-dom';
import type { LinkProps } from 'react-router-dom';

import { cn } from './cn';
import { Spinner } from './Spinner';

export type ButtonVariant = 'primary' | 'secondary' | 'ghost' | 'danger' | 'link';
export type ButtonSize = 'sm' | 'md' | 'lg';

const VARIANTS: Record<ButtonVariant, string> = {
  primary:
    'bg-[var(--accent)] text-white border border-transparent hover:bg-[var(--accent-hover)] shadow-[0_1px_0_rgb(20_18_14/0.12)]',
  secondary:
    'bg-[var(--surface-raised)] text-[var(--text-primary)] border border-[var(--border-hairline)] hover:bg-[var(--surface-sunken)]',
  ghost:
    'bg-transparent text-[var(--text-secondary)] border border-transparent hover:bg-[var(--surface-sunken)] hover:text-[var(--text-primary)]',
  danger:
    'bg-transparent text-lacquer-600 border border-lacquer-500/35 hover:bg-lacquer-500 hover:text-white',
  link: 'bg-transparent border-0 p-0 h-auto text-[var(--link)] underline underline-offset-4 decoration-1 hover:decoration-2',
};

const SIZES: Record<ButtonSize, string> = {
  sm: 'h-8 px-3 text-xs gap-1.5',
  md: 'h-10 px-4 text-sm gap-2',
  lg: 'h-12 px-6 text-base gap-2.5',
};

export function buttonClassName(
  variant: ButtonVariant = 'secondary',
  size: ButtonSize = 'md',
  extra?: string,
): string {
  return cn(
    'inline-flex items-center justify-center box-border rounded-[var(--radius-sheet)]',
    'font-medium leading-none whitespace-nowrap transition-colors duration-150',
    'touch-manipulation disabled:pointer-events-none disabled:opacity-45',
    variant !== 'link' && SIZES[size],
    VARIANTS[variant],
    extra,
  );
}

export interface ButtonProps extends Omit<ButtonHTMLAttributes<HTMLButtonElement>, 'className'> {
  variant?: ButtonVariant;
  size?: ButtonSize;
  loading?: boolean;
  block?: boolean;
  iconLeft?: ReactNode;
  iconRight?: ReactNode;
  className?: string;
  ref?: Ref<HTMLButtonElement>;
}

export function Button({
  variant = 'secondary',
  size = 'md',
  loading = false,
  block = false,
  iconLeft,
  iconRight,
  className,
  children,
  disabled,
  type = 'button',
  ...rest
}: ButtonProps) {
  return (
    <button
      type={type}
      disabled={disabled || loading}
      aria-busy={loading || undefined}
      className={buttonClassName(variant, size, cn(block && 'w-full', className))}
      {...rest}
    >
      {loading ? <Spinner size={size === 'lg' ? 18 : 14} /> : iconLeft}
      {children}
      {!loading && iconRight}
    </button>
  );
}

/** Ссылка в облике кнопки: кнопка с вложенным `<a>` — невалидный HTML. */
export function LinkButton({
  variant = 'secondary',
  size = 'md',
  block,
  className,
  children,
  ...rest
}: Omit<LinkProps, 'className'> & {
  variant?: ButtonVariant;
  size?: ButtonSize;
  block?: boolean;
  className?: string;
}) {
  return (
    <Link className={buttonClassName(variant, size, cn(block && 'w-full', className))} {...rest}>
      {children}
    </Link>
  );
}
