import type { BadgeTone } from '@/ui';
import type { CarStatus, RequestStatus, Stage } from '@/lib/api';

/** Короткие подписи для канбана: полные названия с сервера не влезают в колонку. */
const STAGE_BOARD_TITLE: Record<Stage, string> = {
  lead: 'Лид',
  needs: 'Потребность',
  contract: 'Договор',
  payment: 'Оплата',
  shipping: 'Привоз',
  customs: 'Растаможка',
  handover: 'Выдача',
};

export function stageBoardTitle(stage: Stage, fallback = ''): string {
  return STAGE_BOARD_TITLE[stage] ?? fallback;
}

export function stageIndexLabel(position: number): string {
  return String(position).padStart(2, '0');
}

export function requestTone(status: RequestStatus): BadgeTone {
  switch (status) {
    case 'answered':
    case 'converted':
      return 'jade';
    case 'in_progress':
      return 'amber';
    case 'rejected':
      return 'danger';
    case 'new':
      return 'harbor';
    default:
      return 'neutral';
  }
}

export function carStatusTitle(status: CarStatus): string {
  switch (status) {
    case 'draft':
      return 'Черновик';
    case 'moderation':
      return 'На проверке';
    case 'active':
      return 'В продаже';
    case 'reserved':
      return 'Забронирован';
    case 'sold':
      return 'Продан';
    case 'archived':
      return 'В архиве';
    default:
      return status;
  }
}

export function carStatusTone(status: CarStatus): BadgeTone {
  switch (status) {
    case 'active':
      return 'jade';
    case 'moderation':
      return 'amber';
    case 'reserved':
      return 'harbor';
    case 'sold':
      return 'accent';
    case 'archived':
    case 'draft':
    default:
      return 'neutral';
  }
}

export function bannerStatusTone(status: string): BadgeTone {
  switch (status) {
    case 'active':
      return 'jade';
    case 'moderation':
      return 'amber';
    case 'rejected':
      return 'danger';
    case 'paused':
    case 'expired':
      return 'harbor';
    default:
      return 'neutral';
  }
}

export const REQUEST_STATUSES = [
  { value: 'new', title: 'Новая' },
  { value: 'in_progress', title: 'В работе' },
  { value: 'answered', title: 'Есть ответ' },
  { value: 'converted', title: 'Стала сделкой' },
  { value: 'rejected', title: 'Отклонена' },
  { value: 'closed', title: 'Закрыта' },
] as const;
