import { render, screen } from '@testing-library/react';
import { MemoryRouter } from 'react-router-dom';
import { describe, expect, it } from 'vitest';

import { FunnelStrip } from '@/ui';
import { DealersCatalog } from '@/pages/public/InfoPages';
import { dealerCityLabel } from '@/lib/format';
import type { PublicDealer } from '@/lib/api';

function dealer(partial: Partial<PublicDealer>): PublicDealer {
  return {
    user_id: '00000000-0000-0000-0000-000000000001',
    slug: 'vostok',
    company_name: 'Восток Авто',
    city: 'Владивосток',
    description: 'Импорт из Японии',
    services: [],
    work_countries: ['jp'],
    rating_avg: 0,
    rating_count: 0,
    deals_won: 3,
    deals_total: 10,
    cars_active: 2,
    verified: false,
    ...partial,
  };
}

describe('dealerCityLabel', () => {
  it('подставляет подпись, если город не заполнен', () => {
    expect(dealerCityLabel('')).toBe('город не указан');
    expect(dealerCityLabel('   ')).toBe('город не указан');
    expect(dealerCityLabel('Владивосток')).toBe('Владивосток');
  });
});

describe('FunnelStrip', () => {
  it('показывает семь ячеек с нулями', () => {
    render(
      <FunnelStrip
        items={[
          { stage: 'lead', title: 'Лид', count: 0 },
          { stage: 'needs', title: 'Потребность', count: 0 },
          { stage: 'contract', title: 'Договор', count: 0 },
          { stage: 'payment', title: 'Оплата', count: 0 },
          { stage: 'shipping', title: 'Привоз', count: 0 },
          { stage: 'customs', title: 'Растаможка', count: 0 },
          { stage: 'handover', title: 'Выдача', count: 0 },
        ]}
      />,
    );
    expect(screen.getByText('01')).toBeInTheDocument();
    expect(screen.getByText('07')).toBeInTheDocument();
    expect(screen.getAllByText('0')).toHaveLength(7);
  });
});

describe('DealersCatalog', () => {
  it('показывает пустое состояние', () => {
    render(
      <MemoryRouter>
        <DealersCatalog items={[]} />
      </MemoryRouter>,
    );
    expect(screen.getByText('Каталог дилеров пуст')).toBeInTheDocument();
  });

  it('рисует карточки сети', () => {
    render(
      <MemoryRouter>
        <DealersCatalog items={[dealer({ city: '' }), dealer({ slug: 'msk', company_name: 'Столица', city: 'Москва' })]} />
      </MemoryRouter>,
    );
    expect(screen.getByText('Восток Авто')).toBeInTheDocument();
    expect(screen.getByText('Столица')).toBeInTheDocument();
    expect(screen.getByText('город не указан')).toBeInTheDocument();
    expect(screen.getByText('Москва')).toBeInTheDocument();
  });
});
