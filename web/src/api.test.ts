import { afterEach, expect, test, vi } from "vitest";
import { api } from "./api";

function mockFetch(body:unknown) {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    headers: { get: (name:string) => (name.toLowerCase() === "content-type" ? "application/json" : "") },
    text: () => Promise.resolve(JSON.stringify(body))
  } as unknown as Response);
}

afterEach(() => {
  vi.restoreAllMocks();
});

test("map builds a query string only from the params it receives", async () => {
  const fetchMock = mockFetch([]);
  vi.stubGlobal("fetch", fetchMock);

  await api.map();
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/map", expect.anything());

  await api.map(5);
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/map?library=5", expect.anything());

  await api.map(5, 9);
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/map?library=5&folder=9", expect.anything());

  await api.map(undefined, undefined, {west:10, south:20, east:30, north:40});
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/map?bbox=10,20,30,40", expect.anything());

  await api.map(5, undefined, {west:10, south:20, east:30, north:40});
  expect(fetchMock).toHaveBeenCalledWith("/api/v1/map?library=5&bbox=10,20,30,40", expect.anything());
});
