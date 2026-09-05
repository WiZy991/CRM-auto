import { createContext, use } from 'react';

import type { CurrentUser, Role } from '@/lib/api';

export type AuthStatus = 'loading' | 'authenticated' | 'guest';

export interface AuthContextValue {
  status: AuthStatus;
  user: CurrentUser | null;

  /** Подтверждён ли хотя бы один канал связи. От этого зависит доступ к
   *  созданию заявок и объявлений — сервер проверяет то же самое. */
  isVerified: boolean;

  hasRole: (...roles: readonly Role[]) => boolean;

  applyUser: (user: CurrentUser) => void;
  login: (login: string, password: string) => Promise<CurrentUser>;
  register: (input: {
    role: Exclude<Role, 'admin'>;
    email: string;
    phone: string;
    password: string;
    full_name: string;
  }) => Promise<CurrentUser>;
  logout: () => Promise<void>;
  reload: () => Promise<void>;
}

export const AuthContext = createContext<AuthContextValue | null>(null);

export function useAuth(): AuthContextValue {
  const value = use(AuthContext);
  if (!value) {
    throw new Error('useAuth вызван вне AuthProvider');
  }
  return value;
}
