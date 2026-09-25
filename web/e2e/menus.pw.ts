import { login, firstLibraryId, setTheme, offsetsInside, test, expect } from './helpers';

// The three-dot item menus must be one component everywhere: same pill
// trigger style, portal popup, and top-right placement on cards.

test('item menus are unified and top-right on library and folder tiles', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstLibraryId(page);
  test.skip(libId == null, 'no libraries configured');

  await page.goto('/');
  await page.waitForSelector('.library-tile', { timeout: 10_000 });
  await setTheme(page, 'dark');
  const tileCard = await page.locator('.library-tile').first().boundingBox();
  const tileBtn = await page.locator('.library-tile button.menu-summary').first().boundingBox();
  const tileBg = await page.locator('.library-tile button.menu-summary').first()
    .evaluate(el => getComputedStyle(el).backgroundColor);
  const tileOffsets = offsetsInside(tileBtn!, tileCard!);
  // top-right corner (10px inset + 1px border tolerance)
  expect(tileOffsets.top).toBeLessThanOrEqual(12);
  expect(tileOffsets.right).toBeLessThanOrEqual(12);
  expect(tileOffsets.left).toBeGreaterThan(tileOffsets.right); // clearly right side

  // Admin library rows do not use a popup at all: the dots open the actions
  // dialog, and Delete is a separate row button on the same line.
  await page.goto('/admin?section=libraries');
  await page.waitForSelector('button[aria-label^="Edit library "]', { timeout: 10_000 });
  await setTheme(page, 'dark');
  const row = page.locator('.admin-library').first();
  const dots = row.locator('button[aria-label^="Edit library "]');
  const del = row.locator('button.danger');
  await dots.click();
  await expect(page.getByRole('dialog', { name: /^Library actions / })).toBeVisible();
  expect(await page.locator('.item-submenu').count()).toBe(0);
  await page.getByRole('button', { name: 'Close' }).click();
  await expect(page.getByRole('dialog', { name: /^Library actions / })).toBeHidden();

  const nameBox = await row.locator('.library-name-button').boundingBox();
  const dotsBox = await dots.boundingBox();
  const delBox = await del.boundingBox();
  // One line: name, dots and Delete share the same vertical band.
  expect(Math.abs(dotsBox!.y - nameBox!.y)).toBeLessThanOrEqual(6);
  expect(Math.abs(delBox!.y - nameBox!.y)).toBeLessThanOrEqual(6);
  expect(delBox!.x).toBeGreaterThan(dotsBox!.x + dotsBox!.width - 1);
  // Nothing wraps below the name, so the row is no taller than its name cell.
  const rowBox = await row.boundingBox();
  expect(rowBox!.height).toBeLessThanOrEqual(nameBox!.height + 8);

  // Same trigger background on folder tiles as on library tiles (one style).
  await page.goto(`/library/${libId}`);
  const firstFolder = page.locator('.folder-entry').first();
  if (await firstFolder.count()) {
    await setTheme(page, 'dark');
    const folderBtn = firstFolder.locator('button.menu-summary');
    await expect(folderBtn).toBeVisible();
    const folderBg = await folderBtn.evaluate(el => getComputedStyle(el).backgroundColor);
    expect(folderBg).toBe(tileBg);
    const card = await firstFolder.boundingBox();
    const btn = await folderBtn.boundingBox();
    const offs = offsetsInside(btn!, card!);
    expect(offs.top).toBeLessThanOrEqual(12);
    expect(offs.right).toBeLessThanOrEqual(12);
  }
});

test('menus keep theme-aware colors in dark and forest themes', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstLibraryId(page);
  test.skip(libId == null, 'no libraries configured');
  await page.goto(`/library/${libId}`);
  await page.waitForSelector('.folder-entry, .media', { timeout: 10_000 });
  for (const theme of ['dark', 'forest'] as const) {
    await setTheme(page, theme);
    const bg = await page.locator('button.menu-summary').first()
      .evaluate(el => getComputedStyle(el).backgroundColor);
    // light-theme hardcoded value was rgb(238,243,248) — must never appear now
    expect(bg).not.toBe('rgb(238, 243, 248)');
  }
});
