// Esri World Imagery Wayback: dated archive releases of the Esri satellite
// basemap. The release list comes from Esri's public config file; each release
// has its own tile path, so the map can show imagery as of a given date.

export const WAYBACK_CONFIG_URL = "https://s3-us-west-2.amazonaws.com/config.maptiles.arcgis.com/waybackconfig.json";

export type WaybackRelease = { release:number; date:string };

// Newest first. Entries without a release number or a YYYY-MM-DD date in the
// title are skipped.
export function parseWaybackConfig(config:unknown):WaybackRelease[] {
  if (!config || typeof config !== "object") return [];
  const releases:WaybackRelease[] = [];
  for (const [key, value] of Object.entries(config as Record<string, unknown>)) {
    const release = Number(key);
    const title = value && typeof value === "object" ? (value as {itemTitle?:unknown}).itemTitle : undefined;
    const date = typeof title === "string" ? title.match(/\d{4}-\d{2}-\d{2}/)?.[0] : undefined;
    if (!Number.isInteger(release) || release <= 0 || !date) continue;
    releases.push({release, date});
  }
  return releases.sort((left, right) => right.date.localeCompare(left.date) || right.release - left.release);
}

export function waybackTileUrl(release:number) {
  return `https://wayback.maptiles.arcgis.com/arcgis/rest/services/World_Imagery/WMTS/1.0.0/default028mm/MapServer/tile/${release}/{z}/{y}/{x}`;
}

function day(date:string) {
  const time = Date.parse(`${date.slice(0, 10)}T00:00:00Z`);
  return Number.isNaN(time) ? null : time;
}

// The release whose date is closest to the given date (YYYY-MM-DD or an ISO
// timestamp); on a tie the later release wins.
export function nearestWaybackRelease(releases:readonly WaybackRelease[], date:string):WaybackRelease|undefined {
  const target = day(date);
  if (target === null) return undefined;
  let best:WaybackRelease|undefined;
  let bestDistance = Infinity;
  for (const entry of releases) {
    const time = day(entry.date);
    if (time === null) continue;
    const distance = Math.abs(time - target);
    if (distance < bestDistance || (distance === bestDistance && best && entry.date > best.date)) {
      best = entry;
      bestDistance = distance;
    }
  }
  return best;
}

// Median capture date (YYYY-MM-DD) of the given items, or "" when none has one.
export function medianTakenDate(items:readonly {takenAt?:string}[]):string {
  const dates = items.map(item => (item.takenAt ?? "").slice(0, 10)).filter(date => day(date) !== null).sort();
  return dates.length > 0 ? dates[Math.floor((dates.length - 1) / 2)] : "";
}
