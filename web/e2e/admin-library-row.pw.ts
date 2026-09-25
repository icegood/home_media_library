import { login, test, expect } from './helpers';

// Admin library rows: name, dots and Delete share one line, and on phone
// viewports the table scrolls sideways instead of squeezing the name or
// stacking Delete underneath it.

const PHONE = { width: 390, height: 780 };

test('admin library row keeps Delete on the name line', async ({ page, baseURL }) => {
  await login(page, baseURL);
  await page.goto('/admin?section=libraries');
  const row = page.locator('.admin-library').first();
  await expect(row).toBeVisible({ timeout: 10_000 });

  const name = await row.locator('.library-name-button').boundingBox();
  const dots = await row.locator('button[aria-label^="Edit library "]').boundingBox();
  const move = await row.locator('button.row-action.secondary').boundingBox();
  const del = await row.locator('button.danger').boundingBox();
  const rowBox = await row.boundingBox();
  expect(Math.abs(dots!.y - name!.y)).toBeLessThanOrEqual(6);
  expect(Math.abs(move!.y - name!.y)).toBeLessThanOrEqual(6);
  expect(Math.abs(del!.y - name!.y)).toBeLessThanOrEqual(6);
  // Left to right: name, dots, Move, Delete.
  expect(move!.x).toBeGreaterThan(dots!.x + dots!.width - 1);
  expect(del!.x).toBeGreaterThan(move!.x + move!.width - 1);
  // The row must not grow a second line just to fit Move and Delete.
  expect(rowBox!.height).toBeLessThanOrEqual(name!.height + 8);
});

test('admin library row scrolls sideways on a phone', async ({ page, baseURL }) => {
  await login(page, baseURL);
  await page.setViewportSize(PHONE);
  await page.goto('/admin?section=libraries');
  const row = page.locator('.admin-library').first();
  await expect(row).toBeVisible({ timeout: 10_000 });

  const table = page.locator('.library-table');
  const overflowX = await table.evaluate(el => getComputedStyle(el).overflowX);
  expect(['auto', 'scroll']).toContain(overflowX);

  const name = await row.locator('.library-name-button').boundingBox();
  const del = await row.locator('button.danger').boundingBox();
  // Still one line when squeezed into a 390px viewport.
  expect(Math.abs(del!.y - name!.y)).toBeLessThanOrEqual(6);
  expect(await row.locator('button.row-action.secondary').count()).toBe(1);
  // Delete is reachable by scrolling the table, not clipped away.
  expect(name!.width).toBeGreaterThan(60);
});
