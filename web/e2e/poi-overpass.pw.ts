import { login, firstLibraryId, firstGpsLibraryId, test, expect } from './helpers';

test('POI proxy never bubbles a 406 from Overpass to the client', async ({ page, request, baseURL }) => {
  await login(page, baseURL);
  const direct = await request.get(`${baseURL}/api/v1/map/poi?bbox=30.5,50.0,30.6,50.1&categories=food&theme=light`);
  // Whatever the high-level status (200 → JSON array, 502 → "Overpass
  // connection refused"), the body must NEVER contain "Not Acceptable" or
  // a bare 406 — that means overpass-api.de was reached *and* its mod_negotiation
  // rejected the request, which would re-introduce the bug we fixed.
  const text = await direct.text();
  expect(text).not.toMatch(/Not Acceptable/);
  expect(text).not.toMatch(/\b406\b/);
});

test('POI overlay: opening Categories shows a checkbox list in a themed popup', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstGpsLibraryId(page) ?? await firstLibraryId(page);
  test.skip(libId == null, 'no libraries configured');
  await page.goto(`/map?library=${libId}`);
  await page.waitForSelector('.leaflet-container', { timeout: 15_000 });
  await page.locator('.top-menu-handle').hover();
  const poiToggle = page.getByRole('checkbox', { name: 'POI' });
  await poiToggle.evaluate(input => {
    const el = input as HTMLInputElement;
    if (!el.checked) {
      el.click();
      el.dispatchEvent(new Event('change', { bubbles: true }));
    }
  });
  await expect(poiToggle).toBeChecked();
  const trigger = page.getByRole('button', { name: /^Categories/ });
  await expect(trigger).toBeVisible();
  await trigger.evaluate(btn => (btn as HTMLButtonElement).click());
  const panel = page.getByRole('dialog', { name: 'POI categories' });
  await expect(panel).toBeVisible({ timeout: 5000 });
  expect(panel).toHaveClass('poi-cat-panel');
  await expect(panel).toHaveCSS('position', 'fixed');
  expect((await panel.evaluate(el => (el as HTMLElement).style.right))).not.toBe('');
  expect((await panel.evaluate(el => (el as HTMLElement).style.top))).not.toBe('');
  // Six checkboxes, no broken layout.
  expect(await panel.getByRole('checkbox').count()).toBe(6);
});
