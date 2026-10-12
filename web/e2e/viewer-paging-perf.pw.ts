import { login, test, expect } from './helpers';

// Play-mode paging with the "Next media" button over a slow link (every API
// call delayed by 80 ms): prefetched neighbours must appear without blank
// frames. Each step logs when the URL changed, when the <img> got its new src,
// when it was loaded and visible, plus per-request timings.

const STEPS = 12;

test('viewer paging shows prefetched neighbours without blank frames', async ({ page, baseURL }) => {
  test.setTimeout(180_000);
  await login(page, baseURL);
  // Simulate a remote server: every API call takes an extra 80 ms.
  await page.route('**/api/v1/**', async route => { await new Promise(r => setTimeout(r, 80)); await route.continue(); });
  const libs = await page.request.get('/api/v1/libraries').then(r => r.json());
  let pick: { lib: number; folder: number; ids: number[] } | null = null;
  for (const lib of libs) {
    const media = await page.request.get(`/api/v1/libraries/${lib.id}/media`).then(r => (r.ok() ? r.json() : []));
    const byFolder = new Map<number, { id: number; name: string; size: number }[]>();
    for (const m of media) {
      if (m.kind !== 'image') continue;
      const list = byFolder.get(m.folderId) ?? [];
      list.push(m);
      byFolder.set(m.folderId, list);
    }
    for (const [folder, list] of byFolder) {
      const big = list.filter(m => m.size > 2_000_000);
      if (big.length > STEPS + 2) {
        list.sort((a, b) => a.name.localeCompare(b.name));
        pick = { lib: lib.id, folder, ids: list.map(m => m.id) };
        break;
      }
    }
    if (pick) break;
  }
  test.skip(!pick, 'no folder with enough large images');
  const { lib, folder, ids } = pick!;
  await page.goto(`/library/${lib}/view/${folder}?item=${ids[0]}`);
  await page.waitForFunction(() => {
    const img = document.querySelector('.viewer-media img') as HTMLImageElement | null;
    return !!img && img.complete && img.naturalWidth > 0 && getComputedStyle(img).opacity === '1';
  }, null, { timeout: 30_000 });
  await page.waitForTimeout(1500);
  const rows: string[] = [];
  let blank = 0;
  for (let step = 0; step < STEPS; step++) {
    const before = page.url();
    const result = await page.evaluate(async (prevUrl) => {
      performance.clearResourceTimings();
      const t0 = performance.now();
      (document.querySelector('button[aria-label="Next media"]') as HTMLButtonElement).click();
      let blankFrames = 0;
      let urlAt = -1, srcAt = -1, completeAt = -1, visibleAt = -1, prevSrc = (document.querySelector('.viewer-media img') as HTMLImageElement | null)?.src ?? '';
      let longest = 0;
      let last = t0;
      while (performance.now() - t0 < 20_000) {
        await new Promise(requestAnimationFrame);
        const now = performance.now();
        longest = Math.max(longest, now - last);
        last = now;
        if (urlAt < 0 && location.href !== prevUrl) urlAt = now - t0;
        const img = document.querySelector('.viewer-media img') as HTMLImageElement | null;
        if (!img || img.naturalWidth === 0 || getComputedStyle(img).opacity !== '1') blankFrames++;
        if (!img) continue;
        if (srcAt < 0 && img.src !== prevSrc) srcAt = now - t0;
        if (srcAt >= 0 && completeAt < 0 && img.complete && img.naturalWidth > 0) completeAt = now - t0;
        if (srcAt >= 0 && img.complete && img.naturalWidth > 0 && getComputedStyle(img).opacity === '1') { visibleAt = now - t0; break; }
      }
      const res = performance.getEntriesByType('resource') as PerformanceResourceTiming[];
      const reqs = res.map(r => `${r.name.replace(/^.*\/api\/v1/, '').replace(/\?.*$/, '')} start=${Math.round(r.startTime - t0)} dur=${Math.round(r.duration)} size=${r.transferSize}`);
      return { blankFrames, urlAt: Math.round(urlAt), srcAt: Math.round(srcAt), completeAt: Math.round(completeAt), visibleAt: Math.round(visibleAt), longestFrame: Math.round(longest), reqs };
    }, before);
    blank += result.blankFrames;
    rows.push(`step ${step + 1}: blankFrames=${result.blankFrames} url=${result.urlAt} src=${result.srcAt} loaded=${result.completeAt} visible=${result.visibleAt} longestFrame=${result.longestFrame}\n    ${result.reqs.join('\n    ')}`);
    await page.waitForTimeout(300);
  }
  console.log(rows.join('\n'));
  expect(blank).toBe(0);
});
