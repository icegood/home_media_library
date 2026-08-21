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

  await page.goto('/');

  // After login, prefer to find a favorite-button on the landing page. If none
  // exist, query the API for libraries and open the first library page which is
  // more likely to contain media items with favorite buttons.
  try {
    await page.waitForSelector('.favorite-button', { timeout: 3000 });
  } catch (e) {
    // Try to find a library via API
    const libs = await request.get(`${baseURL}/api/v1/libraries`).then(r => r.json()).catch(() => []);
    if (Array.isArray(libs) && libs.length > 0) {
      const libId = libs[0].id;
      await page.goto(`/library/${libId}`);
      await page.waitForSelector('.favorite-button', { timeout: 5000 });
    } else {
      // No libraries/media available — capture screenshot and fail with helpful message
      try { await page.screenshot({ path: 'test-output/fav-no-libs.png', fullPage: true }); } catch (e) {}
      throw new Error('No favorite-button found and no libraries present to test against. Create test data or adjust the environment.');
    }
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
