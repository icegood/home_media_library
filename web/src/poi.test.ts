import { afterEach, expect, test, vi } from "vitest";
import { wikidataPhoto, wikipediaSummary } from "./App";

afterEach(() => {
  vi.unstubAllGlobals();
});

test("wikipediaSummary turns the REST summary into a thumbnail, extract and page URL", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({
    ok:true,
    json: async () => ({thumbnail:{source:"https://example.com/t.jpg"}, extract:"A very famous cafe. This is a long description.", content_urls:{desktop:{page:"https://en.wikipedia.org/wiki/Famous_Cafe"}}})
  })));
  const result = await wikipediaSummary("Famous_Cafe");
  expect(result.thumbnail).toBe("https://example.com/t.jpg");
  expect(result.url).toBe("https://en.wikipedia.org/wiki/Famous_Cafe");
  expect(result.extract).toContain("A very famous cafe");
});

test("wikipediaSummary tolerates missing thumbnails and failed requests", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ok:true, json: async () => ({extract:"Just text"})})));
  const result = await wikipediaSummary("Plain_Article");
  expect(result.thumbnail).toBeUndefined();
  expect(result.url).toBeUndefined();
  expect(result.extract).toContain("Just text");
  vi.stubGlobal("fetch", vi.fn(async () => { throw new Error("network down"); }));
  const failed = await wikipediaSummary("Offline_Article");
  expect(failed.extract).toBeUndefined();
  expect(failed.thumbnail).toBeUndefined();
});

test("wikidataPhoto resolves a Commons thumbnail from a Wikidata Q-id", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({
    ok:true,
    json: async () => ({entities:{Q42:{claims:{P18:[{mainsnak:{datavalue:{value:"Example.jpg"}}}]}}}})
  })));
  const result = await wikidataPhoto("Q42");
  expect(result.thumbnail).toBe("https://commons.wikimedia.org/wiki/Special:FilePath/Example.jpg?width=360");
  expect(result.url).toBe("https://www.wikidata.org/wiki/Q42");
});

test("wikidataPhoto returns the Wikidata page URL even when the entity has no image claim", async () => {
  vi.stubGlobal("fetch", vi.fn(async () => ({ok:true, json: async () => ({entities:{Q1:{claims:{}}}})})));
  const result = await wikidataPhoto("Q1");
  expect(result.thumbnail).toBeUndefined();
  expect(result.url).toBe("https://www.wikidata.org/wiki/Q1");
});