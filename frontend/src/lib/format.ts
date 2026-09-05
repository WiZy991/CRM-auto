const DATE = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'short',
  year: 'numeric',
});

const DATETIME = new Intl.DateTimeFormat('ru-RU', {
  day: '2-digit',
  month: 'short',
  hour: '2-digit',
  minute: '2-digit',
});

const NUMBER = new Intl.NumberFormat('ru-RU');

export function formatDate(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : DATE.format(date);
}

export function formatDateTime(iso: string): string {
  const date = new Date(iso);
  return Number.isNaN(date.getTime()) ? iso : DATETIME.format(date);
}

export function formatNumber(value: number): string {
  return NUMBER.format(value);
}

export function formatPercent(value: number): string {
  return `${(value * 100).toFixed(1)} %`;
}

export function formatRubMinor(minor: number): string {
  return new Intl.NumberFormat('ru-RU', {
    style: 'currency',
    currency: 'RUB',
    maximumFractionDigits: 0,
  }).format(minor / 100);
}

export function dealerCityLabel(city?: string): string {
  const trimmed = city?.trim() ?? '';
  return trimmed === '' ? 'город не указан' : trimmed;
}

export function formatMonth(ym: string): string {
  const [year, month] = ym.split('-').map(Number);
  if (!year || !month) return ym;
  return new Intl.DateTimeFormat('ru-RU', { month: 'short', year: 'numeric' }).format(
    new Date(year, month - 1, 1),
  );
}
