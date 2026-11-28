import { login, firstGpsLibraryId, test, expect } from './helpers';

test('a click beside a selected map area keeps it and shows no error', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstGpsLibraryId(page);
  test.skip(libId == null, 'no gps library');
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`/map?library=${libId}`);
  await page.waitForSelector('.cluster-marker', { timeout: 15_000 });
  await page.waitForTimeout(1500);
  const map = (await page.locator('.leaflet-container').boundingBox())!;
  const cx = map.x + map.width / 2, cy = map.y + map.height / 2;
  await page.getByRole('button', { name: 'Select area' }).evaluate(el => (el as HTMLElement).click());
  await page.waitForTimeout(300);
  await page.mouse.move(cx - 150, cy - 100);
  await page.mouse.down();
  await page.mouse.move(cx, cy, { steps: 8 });
  await page.mouse.move(cx + 150, cy + 100, { steps: 8 });
  await page.mouse.up();
  await page.waitForTimeout(1500);
  const rect = page.locator('.leaflet-overlay-pane path.leaflet-interactive');
  await expect(rect).toHaveCount(1);
  const before = await rect.getAttribute('d');
  // A press beside the selected box without dragging keeps the selection and
  // shows no error (it used to send a zero-size box: "invalid bbox").
  await page.mouse.move(map.x + 250, map.y + map.height - 120);
  await page.mouse.down();
  await page.mouse.up();
  await page.waitForTimeout(1000);
  await expect(page.locator('.map-notice')).toHaveCount(0);
  await expect(rect).toHaveCount(1);
  expect(await rect.getAttribute('d')).toBe(before);
  await expect(page.locator('.map-timeline-panel')).toBeVisible();
});
