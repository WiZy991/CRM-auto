export const REPORTS = [
  {
    kind: 'funnel',
    title: 'Воронка по этапам',
    text: 'Сделки на каждом шаге, зависания и доля от открытых.',
  },
  {
    kind: 'pipeline',
    title: 'Сделки в работе',
    text: 'Открытые карточки: клиент, сумма, этап и срок без движения.',
  },
  {
    kind: 'won',
    title: 'Выданные клиентам',
    text: 'Закрытые сделки и выручка по каждой выдаче.',
  },
  {
    kind: 'lost',
    title: 'Сорванные сделки',
    text: 'Отказы и сумма, которая не дошла до выдачи.',
  },
  {
    kind: 'stale',
    title: 'Зависшие сделки',
    text: 'Карточки без смены этапа дольше нормы воронки.',
  },
  {
    kind: 'revenue',
    title: 'Выручка по месяцам',
    text: 'Сумма и число выдач за последние двенадцать месяцев.',
  },
  {
    kind: 'created',
    title: 'Новые сделки',
    text: 'Сколько карточек открыли по месяцам.',
  },
  {
    kind: 'markets',
    title: 'Китай и Япония',
    text: 'Воронка и выручка в разрезе рынка лота.',
  },
  {
    kind: 'cycle',
    title: 'Цикл сделки',
    text: 'Средний срок на этапе и дни от открытия до выдачи.',
  },
  {
    kind: 'requests',
    title: 'Заявки',
    text: 'Статусы обращений и последние заявки в работе.',
  },
  {
    kind: 'listings',
    title: 'Объявления',
    text: 'Лоты витрины по статусам модерации и продажи.',
  },
  {
    kind: 'ads',
    title: 'Реклама',
    text: 'Показы, клики и CTR баннеров на площадке.',
  },
  {
    kind: 'reviews',
    title: 'Отзывы',
    text: 'Оценки после выдачи и ответы дилера.',
  },
] as const;

export type ReportKind = (typeof REPORTS)[number]['kind'];

export function findReport(kind: string | undefined) {
  return REPORTS.find((item) => item.kind === kind);
}

export function downloadCsv(filename: string, headers: string[], rows: string[][]) {
  const lines = [headers, ...rows].map((row) => row.map(escapeCsvCell).join(';'));
  const blob = new Blob([`\uFEFF${lines.join('\n')}`], { type: 'text/csv;charset=utf-8' });
  const url = URL.createObjectURL(blob);
  const link = document.createElement('a');
  link.href = url;
  link.download = filename;
  link.click();
  URL.revokeObjectURL(url);
}

function escapeCsvCell(value: string) {
  if (/[;"\n]/.test(value)) {
    return `"${value.replaceAll('"', '""')}"`;
  }
  return value;
}
