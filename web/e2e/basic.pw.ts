import { test, expect } from '@playwright/test';

test('home page serves and has heading or app root', async ({ page }) => {
  await page.goto('/');
  // Basic smoke check: expect some app content
  const hasHeading = await page.locator('text=Media Library').count();
  if (hasHeading) {
    await expect(page.locator('text=Media Library')).toHaveCount(1);
  } else {
    // fallback: ensure the app root element exists
    await expect(page.locator('#app, main')).toHaveCount(1);
  }
});
