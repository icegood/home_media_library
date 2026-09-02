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

  await page.goto('/admin?section=libraries');
  await page.waitForSelector('button[aria-label^="Library menu "]', { timeout: 10_000 });
  await setTheme(page, 'dark');
  await page.locator('button[aria-label^="Library menu "]').first().click();
  const adminPopup = page.locator('.item-submenu.portal-fixed').last();
  await expect(adminPopup).toBeVisible();
  expect(await adminPopup.evaluate(el => getComputedStyle(el).position)).toBe('fixed');
  await page.mouse.click(8, 400);
  await expect(adminPopup).toBeHidden();

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
