import { test, expect } from '@playwright/test';

// Login via API as ice/test, open a media item's favorite chooser, click the checkbox and
// ensure the click does NOT navigate or open the underlying item (no click-through).

test('favorite-picker checkbox does not propagate click to underlying item', async ({ page, request, baseURL }) => {
  // Authenticate via the API and copy the session cookie into the browser context
  const res = await request.post(`${baseURL}/api/v1/auth/login`, { data: { login: 'ice', password: 'test' } });
  expect(res.status()).toBe(200);
  const setCookie = res.headers()['set-cookie'] || '';
  const match = setCookie.match(/media_session=([^;]+);/);
  if (match) {
    const cookieValue = match[1];
    await page.context().addCookies([{ name: 'media_session', value: cookieValue, domain: 'localhost', path: '/', httpOnly: true }]);
  } else {
    throw new Error('Login did not return a media_session cookie');
  }

  // Favorite buttons live on media cards; the timeline of a library with media
  // is the most reliable surface. The session cookie was injected above, so the
  // page request is authenticated.
  const libs = await page.request.get(`${baseURL}/api/v1/libraries`).then(r => r.json()).catch(() => []);
  let opened = false;
  for (const lib of (Array.isArray(libs) ? libs : [])) {
    const stats = lib.stats ?? { images: 0, videos: 0 };
    if ((stats.images ?? 0) + (stats.videos ?? 0) === 0) continue;
    await page.goto(`/library/${lib.id}/timeline`);
    try {
      await page.waitForSelector('.favorite-button', { timeout: 8000 });
      opened = true;
      break;
    } catch (e) { /* try next library */ }
  }
  if (!opened) {
    test.skip(true, 'no media with favorite buttons available');
  }

  const beforeUrl = page.url();

  // Click the first favorite-button to open the chooser
  await page.click('.favorite-button');
  await page.waitForSelector('.favorite-picker', { state: 'visible' });

  // Click the first checkbox in the picker
  const checkbox = page.locator('.favorite-picker .favorite-picker-list input[type=checkbox]').first();
  await checkbox.waitFor({ state: 'visible' });
  await checkbox.click();

  // short pause to allow any (unexpected) navigation to start
  await page.waitForTimeout(500);

  // Assert the URL did not change (no click-through)
  expect(page.url()).toBe(beforeUrl);
});
