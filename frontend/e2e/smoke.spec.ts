import { expect, test } from '@playwright/test';

const FRONT = process.env.E2E_BASE_URL ?? 'http://127.0.0.1:5173';
const API = process.env.E2E_API_URL ?? 'http://127.0.0.1:8080';

async function live(): Promise<boolean> {
  try {
    const [page, api] = await Promise.all([fetch(FRONT, { signal: AbortSignal.timeout(2000) }), fetch(`${API}/healthz`, { signal: AbortSignal.timeout(2000) })]);
    return page.ok && api.ok;
  } catch {
    return false;
  }
}

test.beforeAll(async () => {
  test.skip(!(await live()), 'нужны запущенные Vite :5173 и API :8080');
});

test('каталог и сеть дилеров открываются', async ({ page }) => {
  await page.goto('/');
  await expect(page.getByRole('heading', { name: /Автомобиль с аукциона/i })).toBeVisible();

  await page.goto('/catalog');
  await expect(page.getByRole('heading', { name: /Автомобили из Китая и Японии/i })).toBeVisible();

  await page.goto('/dealers');
  await expect(page.getByRole('heading', { name: /Дилеры-импортёры/i })).toBeVisible();
  await expect(page.getByText('Каталог дилеров пуст')).toHaveCount(0);

  await page.goto('/sellers');
  await expect(page.getByRole('heading', { name: /Поставщики Китая и Японии/i })).toBeVisible();
  await expect(page.getByText('Справочник откроется после входа дилера')).toHaveCount(0);
});

test('на телефоне шапка не забита кнопками', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/');
  await expect(page.getByRole('button', { name: 'Меню' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Войти' })).toBeVisible();
  await expect(page.getByRole('link', { name: 'Регистрация' })).toHaveCount(0);

  await page.getByRole('button', { name: 'Меню' }).click();
  await expect(page.getByRole('navigation', { name: 'Мобильное меню' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Мобильное меню' }).getByRole('link', { name: 'Каталог' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Мобильное меню' }).getByRole('link', { name: 'Регистрация' })).toBeVisible();
});

test('seed-дилер на телефоне видит нижнее меню кабинета', async ({ page }) => {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto('/login');
  await page.getByLabel('Почта или телефон').fill('dealer@local.test');
  await page.getByLabel('Пароль').fill('Devpass12!');
  await page.getByRole('button', { name: 'Войти' }).click();
  await expect(page).toHaveURL(/\/app/);
  await expect(page.getByRole('heading', { name: 'Воронка' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Кабинет' }).getByRole('link', { name: 'Воронка' })).toBeVisible();
  await expect(page.getByRole('navigation', { name: 'Кабинет' }).getByRole('link', { name: 'Заявки' })).toBeVisible();
  await expect(page.getByRole('button', { name: 'Ещё' })).toBeVisible();
});

test('seed-дилер входит в воронку', async ({ page }) => {
  await page.goto('/login');
  await page.getByLabel('Почта или телефон').fill('dealer@local.test');
  await page.getByLabel('Пароль').fill('Devpass12!');
  await page.getByRole('button', { name: 'Войти' }).click();
  await expect(page).toHaveURL(/\/app/);
  await expect(page.getByRole('heading', { name: 'Воронка' })).toBeVisible();
});
