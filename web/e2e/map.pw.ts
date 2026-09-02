import { login, firstLibraryId, firstGpsLibraryId, setTheme, offsetsInside, test, expect } from './helpers';

test('map filters bar is fully visible when the top menu opens', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstLibraryId(page);
  test.skip(libId == null, 'no libraries configured');
  await page.goto(`/map?library=${libId}`);
  await page.waitForSelector('.leaflet-container', { timeout: 15_000 });
  await page.locator('.top-menu-handle').hover();
  await page.waitForTimeout(500);
  const select = page.locator('.map-page .bar-select').first();
  await expect(select).toBeVisible();
  const box = await select.boundingBox();
  expect(box!.height).toBeGreaterThanOrEqual(20); // not clipped by the autohide bar
});

test('selection panel sits right, below the header, on desktop and phone', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstGpsLibraryId(page);
  test.skip(libId == null, 'no libraries configured');
  await page.setViewportSize({ width: 1280, height: 800 });
  await page.goto(`/map?library=${libId}`);
  await page.waitForSelector('.cluster-marker', { timeout: 15_000 });
  // click the biggest cluster
  const markers = await page.locator('.cluster-marker').all();
  let best = markers[0], bestCount = -1;
  for (const m of markers) {
    const n = parseInt((await m.textContent()).trim(), 10);
    if (n > bestCount) { bestCount = n; best = m; }
  }
  await best.click({ force: true });
  const panel = page.locator('.map-timeline-panel');
  await expect(panel).toBeVisible();
  let box = await panel.boundingBox();
  let vp = page.viewportSize()!;
  expect(box!.x + box!.width / 2).toBeGreaterThan(vp.width / 2); // right half
  expect(box!.y).toBeGreaterThanOrEqual(60); // below header zone
  expect(box!.y + box!.height <= vp.height).toBeTruthy();

  await page.setViewportSize({ width: 390, height: 780 });
  await page.waitForTimeout(600);
  box = await panel.boundingBox();
  vp = page.viewportSize()!;
  expect(box!.width).toBeLessThanOrEqual(vp.width * 0.85); // never full screen
  expect(box!.x + box!.width / 2).toBeGreaterThan(vp.width / 2); // still right
  expect(box!.y).toBeGreaterThanOrEqual(60);
});

test('clicking a panel item pages through exactly the selected range', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstGpsLibraryId(page);
  test.skip(libId == null, 'no libraries configured');
  await page.goto(`/map?library=${libId}`);
  await page.waitForSelector('.cluster-marker', { timeout: 15_000 });
  const cluster = page.locator('.cluster-marker').first();
  await cluster.click({ force: true });
  const items = await page.locator('.map-area-item').count();
  test.skip(items === 0, 'empty selection');
  await page.locator('.map-area-item').first().click();
  await page.waitForSelector('.viewer-page', { timeout: 10_000 });
  const listParam = new URL(page.url()).searchParams.get('list') ?? '';
  expect(listParam).toMatch(/^\d+(,\d+)*$/);
  let steps = 0;
  for (; steps <= items; steps++) {
    const next = page.locator('button[aria-label="Next media"]');
    if (!(await next.isEnabled().catch(() => false))) break;
    await next.click();
    await page.waitForTimeout(300);
  }
  expect(steps).toBe(items - 1); // range strictly limited to the selection
});
