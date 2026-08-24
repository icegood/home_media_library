import { test as base, expect, type Page } from '@playwright/test';

// Log in through the API so the session cookie lands in the API request
// context; specs then navigate straight to deep links.
export async function login(page: Page, baseURL: string | undefined, loginName = 'ice', password = 'test') {
  const res = await page.request.post(`${baseURL}/api/v1/auth/login`, {
    data: { login: loginName, password },
  });
  if (!res.ok()) throw new Error(`Login failed with status ${res.status()}`);
}

export async function firstLibraryId(page: Page): Promise<number | null> {
  const libs = await page.request.get('/api/v1/libraries').then(r => (r.ok() ? r.json() : []));
  return Array.isArray(libs) && libs.length > 0 ? libs[0].id : null;
}

// A library whose media actually has GPS coordinates — required for map
// marker/cluster specs. Returns null when no library has geo data.
export async function firstGpsLibraryId(page: Page): Promise<number | null> {
  const libs = await page.request.get('/api/v1/libraries').then(r => (r.ok() ? r.json() : []));
  if (!Array.isArray(libs)) return null;
  for (const lib of libs) {
    const items = await page.request.get(`/api/v1/map?library=${lib.id}`).then(r => (r.ok() ? r.json() : []));
    if (Array.isArray(items) && items.length > 0) return lib.id;
  }
  return null;
}

// First library id that has any media at all (timeline-based specs).
export async function firstMediaLibraryId(page: Page): Promise<number | null> {
  const libs = await page.request.get('/api/v1/libraries').then(r => (r.ok() ? r.json() : []));
  if (!Array.isArray(libs)) return null;
  for (const lib of libs) {
    const stats = lib.stats ?? { images: 0, videos: 0 };
    if ((stats.images ?? 0) + (stats.videos ?? 0) > 0) return lib.id;
  }
  return null;
}

export async function firstFavoriteViewId(page: Page): Promise<number | null> {
  const views = await page.request.get('/api/v1/favorite-views').then(r => (r.ok() ? r.json() : []));
  return Array.isArray(views) && views.length > 0 ? views[0].id : null;
}

// Set the UI theme directly; the app derives CSS custom properties from
// <html data-theme>, so this exercises every theme without touching settings.
export async function setTheme(page: Page, theme: 'light' | 'dark' | 'forest') {
  await page.evaluate(t => { document.documentElement.dataset.theme = t; }, theme);
  await page.waitForTimeout(250);
}

export interface Rect { x: number; y: number; width: number; height: number }

// Offsets of an element's edges relative to a container rect, in whole pixels.
export function offsetsInside(inner: Rect, outer: Rect) {
  return {
    top: Math.round(inner.y - outer.y),
    left: Math.round(inner.x - outer.x),
    right: Math.round((outer.x + outer.width) - (inner.x + inner.width)),
    bottom: Math.round((outer.y + outer.height) - (inner.y + inner.height)),
  };
}

export const test = base;
export { expect };
