import { describe, expect, test } from "vitest";
import { medianTakenDate, nearestWaybackRelease, parseWaybackConfig, waybackTileUrl } from "./wayback";

const config = {
  "10": {itemTitle: "World Imagery (Wayback 2014-02-20)"},
  "64776": {itemTitle: "World Imagery (Wayback 2023-08-31)"},
  "4230": {itemTitle: "World Imagery (Wayback 2020-07-29)"},
  "5000": {itemTitle: "World Imagery (Wayback 2020-09-02)"},
  "bad": {itemTitle: "World Imagery (Wayback 2019-01-01)"},
  "77": {itemTitle: "no date"},
};

describe("wayback", () => {
  test("parses releases newest first, skipping malformed entries", () => {
    expect(parseWaybackConfig(config)).toEqual([
      {release: 64776, date: "2023-08-31"},
      {release: 5000, date: "2020-09-02"},
      {release: 4230, date: "2020-07-29"},
      {release: 10, date: "2014-02-20"},
    ]);
    expect(parseWaybackConfig(null)).toEqual([]);
  });

  test("picks the release closest to a date", () => {
    const releases = parseWaybackConfig(config);
    expect(nearestWaybackRelease(releases, "2020-08-02T12:43:24Z")?.release).toBe(4230);
    expect(nearestWaybackRelease(releases, "2020-08-20")?.release).toBe(5000);
    expect(nearestWaybackRelease(releases, "2010-01-01")?.release).toBe(10);
    expect(nearestWaybackRelease(releases, "")).toBeUndefined();
  });

  test("median capture date ignores items without one", () => {
    expect(medianTakenDate([{takenAt: "2020-08-02T10:00:00Z"}, {takenAt: ""}, {takenAt: "2020-08-01T09:00:00Z"}, {takenAt: "2021-01-01T00:00:00Z"}])).toBe("2020-08-02");
    expect(medianTakenDate([{takenAt: ""}])).toBe("");
  });

  test("builds the release tile url", () => {
    expect(waybackTileUrl(4230)).toBe("https://wayback.maptiles.arcgis.com/arcgis/rest/services/World_Imagery/WMTS/1.0.0/default028mm/MapServer/tile/4230/{z}/{y}/{x}");
  });
});
