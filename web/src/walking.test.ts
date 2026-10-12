import { act, renderHook } from "@testing-library/react";
import { afterEach, expect, test, vi } from "vitest";
import { bearingDegrees, compassPoint, distanceMeters, formatDistance, useMyPosition } from "./walking";

afterEach(() => {
  delete window.MLLocation;
});

test("distance and bearing match known reference points", () => {
  // Kyiv Maidan → Kyiv Pechersk Lavra is about 2.95 km to the south-east.
  const maidan:[number,number] = [50.4501, 30.5234];
  const lavra:[number,number] = [50.4347, 30.5573];
  expect(distanceMeters(maidan, lavra)).toBeGreaterThan(2900);
  expect(distanceMeters(maidan, lavra)).toBeLessThan(3000);
  expect(compassPoint(bearingDegrees(maidan, lavra))).toBe("SE");
  expect(distanceMeters(maidan, maidan)).toBe(0);
  expect(compassPoint(bearingDegrees([0, 0], [1, 0]))).toBe("N");
  expect(compassPoint(bearingDegrees([0, 0], [0, -1]))).toBe("W");
  expect(compassPoint(359)).toBe("N");
});

test("distances are formatted at walking scale", () => {
  expect(formatDistance(12.4)).toBe("12 m");
  expect(formatDistance(999.4)).toBe("999 m");
  expect(formatDistance(1234)).toBe("1.2 km");
  expect(formatDistance(15400)).toBe("15 km");
});

test("the native bridge feeds positions and is stopped on disable", () => {
  const start = vi.fn();
  const stop = vi.fn();
  window.MLLocation = {start, stop};
  const {result, rerender} = renderHook(({enabled}) => useMyPosition(enabled), {initialProps:{enabled:true}});
  expect(start).toHaveBeenCalledTimes(1);
  act(() => { window.dispatchEvent(new CustomEvent("ml-location", {detail:{lat:50, lng:30, accuracy:8}})); });
  expect(result.current.position).toEqual({lat:50, lng:30, accuracy:8});
  act(() => { window.dispatchEvent(new CustomEvent("ml-location-error", {detail:"Location permission was denied."})); });
  expect(result.current.error).toBe("Location permission was denied.");
  rerender({enabled:false});
  expect(stop).toHaveBeenCalledTimes(1);
  expect(result.current.position).toBeNull();
});
