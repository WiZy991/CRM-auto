/**
 * Ошибки API в том виде, в котором их отдаёт сервер.
 *
 * Формат ответа фиксирован: {"error": {"code", "message", "details"}}.
 * Интерфейс ветвится по машинному `code`, человеку показывается `message`,
 * а `details` раскладывается по полям формы.
 */

/** Коды из internal/pkg/apierr. Перечислены явно, чтобы опечатка в сравнении
 *  становилась ошибкой типизации, а не молча неверной веткой. */
export const API_ERROR_CODES = [
  'validation_failed',
  'bad_request',
  'unauthorized',
  'forbidden',
  'not_found',
  'conflict',
  'too_many_requests',
  'payload_too_large',
  'unsupported_media_type',
  'internal_error',
  'service_unavailable',
  'invalid_credentials',
  'account_locked',
  'email_not_verified',
  'phone_not_verified',
  'captcha_required',
  'captcha_invalid',
  'token_expired',
  'token_reused',
  'invalid_stage_transition',
] as const;

export type ApiErrorCode = (typeof API_ERROR_CODES)[number];

export class ApiError extends Error {
  readonly status: number;
  readonly code: ApiErrorCode | string;
  readonly details: Readonly<Record<string, string>>;
  readonly requestId: string;

  constructor(params: {
    status: number;
    code: string;
    message: string;
    details?: Record<string, string>;
    requestId?: string;
  }) {
    super(params.message);
    this.name = 'ApiError';
    this.status = params.status;
    this.code = params.code;
    this.details = Object.freeze({ ...params.details });
    this.requestId = params.requestId ?? '';
  }

  is(...codes: readonly (ApiErrorCode | string)[]): boolean {
    return codes.includes(this.code);
  }

  /** Сообщение для конкретного поля формы. */
  fieldError(field: string): string | undefined {
    return this.details[field];
  }

  /** Повтор имеет смысл: сбой сети или временная недоступность сервиса.
   *  На 4xx повторять бессмысленно — ответ не изменится. */
  get isRetryable(): boolean {
    return this.status === 0 || this.status === 503 || this.status >= 500;
  }
}

/** Обрыв соединения: сервер не ответил вовсе. Отличается от ошибки сервера
 *  тем, что запрос мог и не дойти, поэтому повторять изменяющие операции
 *  автоматически нельзя. */
export class NetworkError extends ApiError {
  constructor(cause?: unknown) {
    super({
      status: 0,
      code: 'service_unavailable',
      message: 'Нет связи с сервером. Проверьте подключение к сети.',
    });
    this.name = 'NetworkError';
    if (cause instanceof Error) this.cause = cause;
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

/** Текст для показа пользователю из чего угодно, что прилетело в catch. */
export function errorMessage(error: unknown): string {
  if (isApiError(error)) {
    const detail = Object.values(error.details)[0];
    if (detail && detail !== error.message) return detail;
    return error.message;
  }
  if (error instanceof Error && error.message) return error.message;
  return 'Непредвиденная ошибка. Попробуйте повторить.';
}
