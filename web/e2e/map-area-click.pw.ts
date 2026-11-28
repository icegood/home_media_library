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

test('going up from a subfolder map centres its first geotagged item at maximum zoom', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstGpsLibraryId(page);
  test.skip(libId == null, 'no gps library');
  const all = await page.request.get(`/api/v1/map?library=${libId}`).then(r => r.json()) as {id:number; folderId:number; gps:string}[];
  // A geotagged item whose folder has a parent folder.
  let child = 0, parent = 0;
  for (const item of all.filter(entry => entry.gps)) {
    const entries = await page.request.get(`/api/v1/libraries/${libId}/folders/${item.folderId}/entries?limit=1`).then(r => r.json()) as {chain?:{id:number}[]};
    const chain = entries.chain ?? [];
    if (chain.length >= 2) { child = chain[chain.length - 1].id; parent = chain[chain.length - 2].id; break; }
  }
  test.skip(child === 0, 'no nested geotagged folder');
  const settings = await page.request.get('/api/v1/settings').then(r => r.ok() ? r.json() : {}) as {mapMaxZoom?:number};
  const maxZoom = settings.mapMaxZoom || 19;
  await page.setViewportSize({ width: 1280, height: 800 });
  const centred = async () => {
    await expect.poll(async () => page.evaluate(zoom => [...document.querySelectorAll('.leaflet-tile-container img')].some(img => (img as HTMLImageElement).src.includes(`/${zoom}/`)), maxZoom), { timeout: 10_000 }).toBe(true);
    const box = (await page.locator('.leaflet-container').boundingBox())!;
    const near = await page.evaluate(([x, y]) => [...document.querySelectorAll('.leaflet-marker-icon')].some(el => {
      const r = el.getBoundingClientRect();
      return Math.abs(r.x + r.width / 2 - x) < 40 && Math.abs(r.y + r.height / 2 - y) < 60;
    }), [box.x + box.width / 2, box.y + box.height / 2]);
    expect(near).toBe(true);
  };
  await page.goto(`/map?library=${libId}&folder=${child}`);
  await page.waitForSelector('.leaflet-marker-icon', { timeout: 15_000 });
  await page.waitForTimeout(800);
  // Breadcrumb up to the parent's map.
  await page.locator(`.header-crumbs a[href="/map?library=${libId}&folder=${parent}"]`).evaluate(el => (el as HTMLElement).click());
  await page.waitForURL(`**/map?library=${libId}&folder=${parent}`);
  await centred();
  await page.screenshot({ path: 'test-output/return-crumb.png' });
  // Back to the child, then Forward (a history pop) up to the parent again.
  await page.goBack();
  await page.waitForURL(`**/map?library=${libId}&folder=${child}`);
  await page.waitForTimeout(1500);
  await page.goForward();
  await page.waitForURL(`**/map?library=${libId}&folder=${parent}`);
  await centred();
  await page.screenshot({ path: 'test-output/return-pop.png' });
});
