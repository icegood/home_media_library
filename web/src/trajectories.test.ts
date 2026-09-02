import { describe, expect, test } from "vitest";
import { buildTrajectories } from "./App";
import type { MapMedia } from "./types";

function item(partial:Partial<MapMedia> & {id:number}):MapMedia {
  const {id, ...rest} = partial;
  return {
    id,
    libraryId: 1,
    folderId: 1,
    relativePath: "",
    name: `${id}.jpg`,
    kind: "image",
    mimeType: "image/jpeg",
    size: 1,
    metadata: {},
    gps: "",
    takenAt: "",
    ...rest,
  };
}

describe("buildTrajectories", () => {
  test("orders by takenAt ascending and breaks at starts; items before the first start are ignored", () => {
    const segments = buildTrajectories([
      item({id: 1, gps: "10,10", takenAt: "2025-01-01T03:00:00Z"}),
      item({id: 2, gps: "10.1,10.1", takenAt: "2025-01-01T01:00:00Z"}),
      item({id: 4, gps: "10.3,10.3", takenAt: "2025-01-01T05:00:00Z", trajectoryStart: true}),
      item({id: 3, gps: "10.2,10.2", takenAt: "2025-01-01T04:00:00Z"}),
    ]);
    expect(segments).toHaveLength(1);
    expect(segments[0].points).toEqual([[10.3,10.3]]);
    expect(segments[0].start.trajectoryStart).toBe(true);
  });

  test("tie-breaks equal takenAt by id within a started segment", () => {
    const segments = buildTrajectories([
      item({id: 99, gps: "9,9", takenAt: "2025-01-01T00:00:00Z", trajectoryStart: true}),
      item({id: 5, gps: "10,10", takenAt: "2025-01-01T01:00:00Z"}),
      item({id: 3, gps: "10.1,10.1", takenAt: "2025-01-01T01:00:00Z"}),
    ]);
    expect(segments[0].points).toEqual([[9,9],[10.1,10.1],[10,10]]);
  });

  test("ignores items without GPS or without a parseable takenAt, including before the first start", () => {
    const segments = buildTrajectories([
      item({id: 2, gps: "not,valid", takenAt: "2025-01-01T01:00:00Z"}),
      item({id: 3, gps: "11,11", takenAt: ""}),
    ]);
    expect(segments).toHaveLength(0);
  });

  test("a single-point segment draws a start marker but no edge", () => {
    const segments = buildTrajectories([
      item({id: 1, gps: "10,10", takenAt: "2025-01-01T00:00:00Z", trajectoryStart: true}),
    ]);
    expect(segments).toHaveLength(1);
    expect(segments[0].points).toEqual([[10,10]]);
    expect(segments[0].start.trajectoryStart).toBe(true);
  });

  test("an end point closes the segment; items after it and before the next start are ignored", () => {
    const segments = buildTrajectories([
      item({id: 0, gps: "9.9,9.9", takenAt: "2025-01-01T00:00:00Z", trajectoryStart: true}),
      item({id: 1, gps: "10,10", takenAt: "2025-01-01T05:00:00Z"}),
      item({id: 2, gps: "10.1,10.1", takenAt: "2025-01-01T06:00:00Z", trajectoryEnd: true}),
      item({id: 3, gps: "10.2,10.2", takenAt: "2025-01-01T07:00:00Z"}),
      item({id: 4, gps: "10.3,10.3", takenAt: "2025-01-01T08:00:00Z"}),
    ]);
    expect(segments).toHaveLength(1);
    expect(segments[0].points).toEqual([[9.9,9.9],[10,10],[10.1,10.1]]);
  });

  test("a point that is both start and end becomes a standalone single-point segment", () => {
    const segments = buildTrajectories([
      item({id: 1, gps: "10,10", takenAt: "2025-01-01T01:00:00Z"}),
      item({id: 2, gps: "10.1,10.1", takenAt: "2025-01-01T02:00:00Z", trajectoryStart: true, trajectoryEnd: true}),
      item({id: 3, gps: "10.2,10.2", takenAt: "2025-01-01T03:00:00Z"}),
    ]);
    expect(segments).toHaveLength(1);
    expect(segments[0].points).toEqual([[10.1,10.1]]);
  });

  test("keeps videos and images on the same started trajectory", () => {
    const segments = buildTrajectories([
      item({id: 1, gps: "10,10", takenAt: "2025-01-01T00:00:00Z", kind: "image", trajectoryStart: true}),
      item({id: 2, gps: "10.1,10.1", takenAt: "2025-01-01T01:00:00Z", kind: "video"}),
      item({id: 3, gps: "10.2,10.2", takenAt: "2025-01-01T02:00:00Z", kind: "image"}),
    ]);
    expect(segments).toHaveLength(1);
    expect(segments[0].points).toHaveLength(3);
  });
});
