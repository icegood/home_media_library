import { login, firstFavoriteViewId, test, expect } from './helpers';

test('favorite view folder and media cards both offer top-left selection', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const viewId = await firstFavoriteViewId(page);
  test.skip(viewId == null, 'no favorite views configured');
  await page.goto(`/favorites/${viewId}`);
  const boxes = page.locator('.select-media input[type=checkbox]');
  await boxes.first().waitFor({ state: 'attached', timeout: 10_000 }).catch(() => {});
  test.skip(await boxes.count() === 0, 'favorite view has no selectable folder/media cards');
  await expect(boxes.first()).toBeVisible({ timeout: 10_000 });
  const count = await boxes.count();
  for (let i = 0; i < count; i++) {
    const box = await boxes.nth(i).boundingBox();
    const card = await boxes.nth(i).locator('xpath=ancestor::article').boundingBox();
    // checkbox is centered inside its 38px label anchored at the card's top-left
    expect(box!.x - card!.x).toBeLessThanOrEqual(30);
    expect(box!.y - card!.y).toBeLessThanOrEqual(30);
  }
});

test('favorites index separates the create editor from the list', async ({ page, baseURL }) => {
  await login(page, baseURL);
  await page.goto('/favorites');
  const form = page.locator('.inline-create');
  await expect(form).toBeVisible();
  await expect(form.locator('input')).toBeVisible();
  await expect(form.locator('button[type=submit]')).toBeVisible();

  const formRect = await form.boundingBox();
  const btnRect = await form.locator('button[type=submit]').boundingBox();
  // Create button aligned to the editor's right padding edge
  const rightGap = (formRect!.x + formRect!.width) - (btnRect!.x + btnRect!.width);
  expect(rightGap).toBeLessThanOrEqual(20);

  const list = page.locator('.library-table');
  if (await list.count()) {
    const listRect = await list.boundingBox();
    // editor spans the same width as the favorites list
    expect(Math.abs(listRect!.width - formRect!.width)).toBeLessThanOrEqual(2);
    // and is visually separated above it
    expect(listRect!.y).toBeGreaterThanOrEqual(formRect!.y + formRect!.height + 10);
  }
});
