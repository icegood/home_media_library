import { login, test, expect, type Page } from './helpers';

// EXIF orientation is applied by the browser when it decodes the file, not by
// the app. The viewer must therefore render exactly what the browser decoded:
// the same orientation, undistorted. These checks catch the two ways rotation
// goes wrong in practice — a transposed display (the photo appears sideways
// because a rotation was layered on top of the browser's own) and a stretched
// one (the box no longer matches the decoded pixel aspect ratio).
//
// The browser's naturalWidth/naturalHeight is the source of truth for every
// assertion here: stored EXIF metadata can be stale after a library move, and
// the question under test is what the user actually sees on screen.

const ROTATED_LIMIT = 4;
const SAMPLE_LIMIT = 8;

interface Sample {
  libraryId: number;
  id: number;
  folderId: number;
  name: string;
  rawWidth: number;
  rawHeight: number;
  rotatedHint: boolean;
}

interface Decoded {
  naturalWidth: number;
  naturalHeight: number;
  boxWidth: number;
  boxHeight: number;
}

function exifOf(item: { metadata?: Record<string, unknown> }): Record<string, unknown> {
  const exif = (item.metadata ?? {}).exif;
  return (exif && typeof exif === 'object' ? exif : {}) as Record<string, unknown>;
}

function numberOr(value: unknown): number {
  return typeof value === 'number' && value > 0 ? value : 0;
}

// A stored file whose ExifImage dimensions are the transpose of its stored
// Image dimensions, or whose Orientation is one of the transposing values
// (5-8). This only picks *candidates*: the verdict is always taken from what
// the browser decodes, so a mis-picked candidate can never fail a test.
function looksTransposed(exif: Record<string, unknown>): boolean {
  const width = numberOr(exif.ImageWidth);
  const height = numberOr(exif.ImageHeight);
  const exifWidth = numberOr(exif.ExifImageWidth);
  const exifHeight = numberOr(exif.ExifImageHeight);
  if (width && height && exifWidth === height && exifHeight === width) return true;
  const orientation = numberOr(exif.Orientation);
  return orientation >= 5 && orientation <= 8;
}

async function imagesByLibrary(page: Page): Promise<Map<number, Sample[]>> {
  const out = new Map<number, Sample[]>();
  const libs = await page.request.get('/api/v1/libraries').then(r => (r.ok() ? r.json() : []));
  if (!Array.isArray(libs)) return out;
  for (const lib of libs as { id: number }[]) {
    const media = await page.request.get(`/api/v1/libraries/${lib.id}/media`)
      .then(r => (r.ok() ? r.json() : []));
    if (!Array.isArray(media)) continue;
    const samples = (media as { id: number; folderId: number; name: string; kind: string; metadata?: Record<string, unknown> }[])
      .filter(item => item.kind === 'image')
      .map(item => {
        const exif = exifOf(item);
        return {
          libraryId: lib.id,
          id: item.id,
          folderId: item.folderId,
          name: item.name,
          rawWidth: numberOr(exif.ImageWidth),
          rawHeight: numberOr(exif.ImageHeight),
          rotatedHint: looksTransposed(exif),
        };
      });
    if (samples.length) out.set(lib.id, samples);
  }
  return out;
}

// Rotated-looking photos first (that is the interesting case), then a
// round-robin of the rest so no library goes unchecked.
async function imageSamples(page: Page): Promise<Sample[]> {
  const byLibrary = await imagesByLibrary(page);
  const every = [...byLibrary.values()].flat();
  const rotated = every.filter(sample => sample.rotatedHint).slice(0, ROTATED_LIMIT);
  const rest = every.filter(sample => !rotated.includes(sample));
  const picked = [...rotated];
  let added = true;
  while (picked.length < SAMPLE_LIMIT && added) {
    added = false;
    for (const samples of byLibrary.values()) {
      const next = samples.find(sample => !picked.includes(sample));
      if (!next) continue;
      picked.push(next);
      added = true;
      if (picked.length >= SAMPLE_LIMIT) break;
    }
  }
  return picked;
}

// Open one item and wait until the image has actually decoded, so the measured
// natural size is the real one and not zero.
async function openImage(page: Page, sample: Sample): Promise<Decoded> {
  await page.goto(`/library/${sample.libraryId}/view/${sample.folderId}?item=${sample.id}`);
  await page.waitForSelector('.viewer-media > img', { timeout: 15_000 });
  await page.waitForFunction(() => {
    const img = document.querySelector<HTMLImageElement>('.viewer-media > img');
    return Boolean(img?.complete && img.naturalWidth > 0);
  }, undefined, { timeout: 15_000 });
  return page.evaluate(() => {
    const img = document.querySelector<HTMLImageElement>('.viewer-media > img')!;
    const box = img.getBoundingClientRect();
    return {
      naturalWidth: img.naturalWidth,
      naturalHeight: img.naturalHeight,
      boxWidth: box.width,
      boxHeight: box.height,
    };
  });
}

function ratio(width: number, height: number) {
  return width / height;
}

test('viewer shows images in their decoded orientation, undistorted', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const samples = await imageSamples(page);
  test.skip(samples.length === 0, 'no image media to check');

  for (const sample of samples) {
    const decoded = await openImage(page, sample);
    const natural = ratio(decoded.naturalWidth, decoded.naturalHeight);
    const shown = ratio(decoded.boxWidth, decoded.boxHeight);
    // object-fit: contain scales uniformly, so a correct render keeps the
    // aspect ratio. A transposed or stretched render drifts far beyond 2%.
    const drift = Math.abs(shown - natural) / natural;
    expect(
      drift,
      `${sample.name} shows at ${decoded.boxWidth.toFixed(0)}x${decoded.boxHeight.toFixed(0)} ` +
      `but decodes at ${decoded.naturalWidth}x${decoded.naturalHeight} (aspect drift ${(drift * 100).toFixed(1)}%)`,
    ).toBeLessThan(0.02);
  }
});

test('orientation flags do not transpose a photo: it is shown the way the file encodes it', async ({ page, baseURL }) => {
  await login(page, baseURL);
  const samples = await imageSamples(page);
  test.skip(samples.length === 0, 'no image media to check');

  let transposed = 0;
  for (const sample of samples) {
    const decoded = await openImage(page, sample);
    if (!sample.rawWidth || !sample.rawHeight) continue;
    const raw = ratio(sample.rawWidth, sample.rawHeight);
    const natural = ratio(decoded.naturalWidth, decoded.naturalHeight);
    // Orientation 5-8 rotate by a quarter turn, so the decoded aspect is the
    // reciprocal of the stored one. That is the case that used to display
    // sideways, and the viewer must follow the decode, not the raw file.
    const quarterTurned = Math.abs(natural - 1 / raw) / natural < 0.02;
    if (!quarterTurned) continue;
    transposed += 1;
    // Displayed orientation must equal the decoded orientation, not the stored
    // one: a 90 degree swap is exactly the reported bug. A portrait file that
    // decodes landscape (Rotate 90/270) must be shown landscape.
    const shownLandscape = decoded.boxWidth > decoded.boxHeight;
    const decodedLandscape = natural > 1;
    expect(
      shownLandscape,
      `${sample.name} stores ${sample.rawWidth}x${sample.rawHeight}, decodes ${decoded.naturalWidth}x${decoded.naturalHeight}, ` +
      `but is displayed at ${decoded.boxWidth.toFixed(0)}x${decoded.boxHeight.toFixed(0)}`,
    ).toBe(decodedLandscape);
    expect(
      Math.abs(ratio(decoded.boxWidth, decoded.boxHeight) - natural) / natural,
      `${sample.name} is displayed at the wrong aspect ratio for a rotated photo`,
    ).toBeLessThan(0.02);
  }
  test.skip(transposed === 0, 'no rotated (quarter-turned) photos in the sampled media');
});
