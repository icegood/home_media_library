import { login, firstMediaLibraryId, test, expect } from './helpers';

test('trajectory name persists on the media card and survives reloads across listings', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const libId = await firstMediaLibraryId(page);
  test.skip(libId == null, 'no libraries with media');

  // Find a media item with a folder and a date so it shows in the timeline too;
  // trajectory flags must survive every media listing, not just the folder card.
  const mediaRes = await page.request.get(`/api/v1/libraries/${libId}/media`);
  test.skip(!mediaRes.ok(), 'cannot fetch library media');
  const mediaList = await mediaRes.json();
  test.skip(!Array.isArray(mediaList) || mediaList.length === 0, 'no media in library');
  const firstMedia = mediaList.find((m: any) => Number.isFinite(m.folderId) && typeof m.id === 'number' && !!m.takenAt);
  test.skip(!firstMedia, 'no suitable media with folder');
  const folderId: number = firstMedia.folderId;
  const mediaId: number = firstMedia.id;

  // Ensure clean state: unset any existing trajectory start for this item/folder
  await page.request.fetch(`/api/v1/media/${mediaId}/trajectory-start`, {
    method: 'PATCH',
    data: { folderId, start: false },
    headers: { 'Content-Type': 'application/json' },
  });

  const uniqueName = `E2E Trajectory ${Date.now()}`;

  // Trajectory editing lives on the folder-scoped media card: there the parent
  // folder is unambiguous, so setting a start here is always well-defined.
  await page.goto(`/library/${libId}/timeline`);
  const card = page.locator(`article.card.media[data-kb-id="m${mediaId}"]`);
  await card.waitFor({ state: 'attached', timeout: 15_000 });
  const setStartBtn = card.getByRole('button', { name: 'Set trajectory start' });
  await expect(setStartBtn).toBeVisible({ timeout: 10_000 });
  await setStartBtn.click();

  const dialog = page.getByRole('dialog', { name: /Name trajectory/ });
  await expect(dialog).toBeVisible({ timeout: 10_000 });
  const nameInput = dialog.getByRole('textbox', { name: 'Trajectory name' });
  await nameInput.fill(uniqueName);
  await dialog.getByRole('button', { name: 'Save' }).click();
  await expect(dialog).not.toBeVisible({ timeout: 10_000 });

  await expect(card.getByRole('button', { name: 'Unset trajectory start' })).toBeVisible({ timeout: 10_000 });

  // Verify persistence via API: /media/:id carries the name.
  const mediaCheck = await page.request.get(`/api/v1/media/${mediaId}`);
  if (mediaCheck.ok()) {
    const mediaData = await mediaCheck.json();
    const restoredName = mediaData.trajectoryName ?? mediaData.trajectory_name ?? '';
    if (restoredName) expect(restoredName).toBe(uniqueName);
  }
  // The map endpoint reports the name for the owning folder too.
  const mapRes = await page.request.get(`/api/v1/map?library=${libId}&folder=${folderId}`);
  if (mapRes.ok()) {
    const mapItems: any[] = await mapRes.json();
    const found = mapItems.find((m) => m.id === mediaId);
    if (found) expect(found.trajectoryName).toBe(uniqueName);
  }

  // Reload the timeline: the flag must survive a fresh load. This listing goes
  // through /libraries/:id/media, which used to drop trajectory flags entirely —
  // the regression this test was added to catch.
  await page.reload();
  const reloadedCard = page.locator(`article.card.media[data-kb-id="m${mediaId}"]`);
  await reloadedCard.waitFor({ state: 'attached', timeout: 15_000 });
  await expect(reloadedCard.getByRole('button', { name: 'Unset trajectory start' })).toBeVisible({ timeout: 10_000 });

  // Folder-scoped listing must also keep the marker.
  const timelineRes = await page.request.get(`/api/v1/libraries/${libId}/folders/${folderId}/media`);
  if (timelineRes.ok()) {
    const rows: any[] = await timelineRes.json();
    const found = rows.find((m) => m.id === mediaId);
    if (found) {
      expect(found.trajectoryStart).toBe(true);
      if (found.trajectoryName) expect(found.trajectoryName).toBe(uniqueName);
    }
  }

  // Cleanup: unset start to leave DB clean
  await page.request.fetch(`/api/v1/media/${mediaId}/trajectory-start`, {
    method: 'PATCH',
    data: { folderId, start: false },
    headers: { 'Content-Type': 'application/json' },
  });
});