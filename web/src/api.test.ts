import { afterEach, expect, test, vi } from "vitest";
import { api, MAX_VIDEO_THUMBNAILS, rangeQuery } from "./api";

function jsonResponse(body:unknown, status = 200) {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: status === 401 ? "Unauthorized" : "Status",
    headers: {get: (name:string) => (name.toLowerCase() === "content-type" ? "application/json" : "")},
    text: () => Promise.resolve(status === 204 ? "" : JSON.stringify(body))
  } as unknown as Response;
}
function textResponse(body:string, status = 200, contentType = "text/plain") {
  return {
    ok: status >= 200 && status < 300,
    status,
    statusText: "Status",
    headers: {get: (name:string) => (name.toLowerCase() === "content-type" ? contentType : "")},
    text: () => Promise.resolve(body)
  } as unknown as Response;
}

afterEach(() => {
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

test("rangeQuery omits empty ranges and serializes offsets and limits", () => {
  expect(rangeQuery()).toBe("");
  expect(rangeQuery({})).toBe("");
  expect(rangeQuery({offset:5})).toBe("?offset=5");
  expect(rangeQuery({limit:50})).toBe("?limit=50");
  expect(rangeQuery({offset:5, limit:50})).toBe("?offset=5&limit=50");
});

test("call parses JSON responses into typed values", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({id:1, login:"alice"})));
  await expect(api.me()).resolves.toEqual({id:1, login:"alice"});
  expect(vi.mocked(fetch).mock.calls[0][1]).toMatchObject({credentials:"same-origin"});
});

test("call treats 204 as undefined without parsing a body", async () => {
  const response = {...jsonResponse(undefined, 204)} as Response;
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(response));
  await expect(api.logout()).resolves.toBeUndefined();
});

test("call surfaces JSON error payloads as Error messages", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(jsonResponse({error:"library is locked"}, 409)));
  await expect(api.libraries()).rejects.toThrow("library is locked");
});

test("call falls back to status text for non-JSON errors", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("gateway down", 502)));
  await expect(api.libraries()).rejects.toThrow("502 Status from /api/v1/libraries");
});

test("call rejects non-JSON success payloads", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("<html></html>", 200, "text/html")));
  await expect(api.libraries()).rejects.toThrow("Expected JSON from /api/v1/libraries");
});

test("call rejects malformed JSON bodies with a descriptive error", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("{broken", 200, "application/json")));
  await expect(api.libraries()).rejects.toThrow("Expected JSON from /api/v1/libraries");
});

test("auth mutations post JSON bodies", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({user:{id:2, login:"alice", role:"regular"}}));
  vi.stubGlobal("fetch", fetchMock);
  await api.login("alice", "secret");
  expect(JSON.parse(String(fetchMock.mock.calls[0][1].body))).toEqual({login:"alice", password:"secret"});
});

test("setup posts the initial admin credentials", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({id:0, login:"root", role:"admin"}));
  vi.stubGlobal("fetch", fetchMock);
  await api.setup("root", "hunter22");
  expect(fetchMock.mock.calls[0]).toEqual(["/api/v1/setup", expect.objectContaining({method:"POST"})]);
});

test("settings PUT round-trips the payload", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({zoom:110}));
  vi.stubGlobal("fetch", fetchMock);
  await api.updateUserSettings({theme:"dark", language:"de", codec:"h264-aac-mp4", zoom:110, dateFormat:"iso", streamChunkSize:25, defaultThumbImage:"m", defaultThumbVideo:"m", defaultThumbFolder:"m", mapTileProviderLight:"osm", mapTileProviderDark:"carto", poiProviderLight:"overpass", poiProviderDark:"overpass", poiProviders:{overpass:{}}});
  expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/settings");
  expect(JSON.parse(String(fetchMock.mock.calls[0][1].body))).toMatchObject({theme:"dark", zoom:110});
});

test("url builders point at authenticated media endpoints", () => {
  expect(api.thumbnailUrl(7)).toBe("/api/v1/media/7/thumbnail?index=0");
  expect(api.thumbnailUrl(7, 3)).toBe("/api/v1/media/7/thumbnail?index=3");
  expect(api.folderThumbnailUrl(9)).toBe("/api/v1/folders/9/thumbnail");
  expect(api.contentUrl(4)).toBe("/api/v1/media/4/content");
  expect(api.contentUrl(4, true)).toBe("/api/v1/media/4/content?download=1");
  expect(api.logsDownloadUrl()).toBe("/api/v1/admin/logs/download");
  expect(api.playbackUrl(11, ["h264", "aac"])).toBe("/api/v1/media/11/play?codecs=h264%2Caac");
  expect(api.playbackUrl(11, ["h264"], 90)).toBe("/api/v1/media/11/play?codecs=h264&start=90");
});

test("media mutations target the REST resources they describe", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ok:true}));
  vi.stubGlobal("fetch", fetchMock);
  await api.favoriteMedia(30, 100);
  expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/favorite-views/30/media/100");
  expect(fetchMock.mock.calls[0][1].method).toBe("PUT");
  await api.unfavoriteMedia(30, 100);
  expect(fetchMock.mock.calls[1][1].method).toBe("DELETE");
  await api.updateGPS(100, "50.45,30.52");
  expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/media/100/gps");
  await api.bulkUpdateMedia({selectedIds:[1, 2], shiftMinutes:60});
  expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/media/bulk");
  await api.metadataRenew(1, {recreateExisting:true});
  expect(fetchMock.mock.calls[4][0]).toBe("/api/v1/admin/libraries/1/metadata/renew");
});

test("favorite view membership endpoints cover folders and expanded listings", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse([]));
  vi.stubGlobal("fetch", fetchMock);
  await api.favoriteViewMedia(30, true);
  expect(fetchMock.mock.calls[0][0]).toBe("/api/v1/favorite-views/30/media?expand=true");
  await api.favoriteViewMediaFull(30, true);
  expect(fetchMock.mock.calls[1][0]).toBe("/api/v1/favorite-views/30/media?full=true&expand=true");
  await api.favoriteFolder(30, 20);
  expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/favorite-views/30/folders/20");
  await api.mediaFavoriteViews(100);
  expect(fetchMock.mock.calls[3][0]).toBe("/api/v1/media/100/favorite-views");
  await api.folderFavoriteViews(20);
  expect(fetchMock.mock.calls[4][0]).toBe("/api/v1/folders/20/favorite-views");
});

test("geocode queries nominatim and returns parsed results", async () => {
  const fetchMock = vi.fn().mockResolvedValue(textResponse(JSON.stringify([{display_name:"Kyiv"}]), 200, "application/json"));
  vi.stubGlobal("fetch", fetchMock);
  await expect(api.geocode("Kyiv")).resolves.toEqual([{display_name:"Kyiv"}]);
  expect(String(fetchMock.mock.calls[0][0])).toContain("q=Kyiv");
});

test("geocode rejects HTTP failures and malformed bodies", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("nope", 500)));
  await expect(api.geocode("x")).rejects.toThrow("Geocoder returned HTTP 500");

  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse("{bad", 200, "application/json")));
  await expect(api.geocode("x")).rejects.toThrow("Geocoder returned an invalid response");

  vi.stubGlobal("fetch", vi.fn().mockResolvedValue(textResponse(JSON.stringify({weird:true}), 200, "application/json")));
  await expect(api.geocode("x")).rejects.toThrow("Geocoder returned an invalid response");
});

test("downloadArchive POSTs the selection and triggers a browser download", async () => {
  const blob = new Blob(["zip"]);
  const createObjectURL = vi.fn(() => "blob:archive");
  const revokeObjectURL = vi.fn();
  vi.stubGlobal("URL", Object.assign(URL, {createObjectURL, revokeObjectURL}));
  const click = vi.fn();
  const originalCreateElement = document.createElement.bind(document);
  vi.spyOn(document, "createElement").mockImplementation((tag:string) => {
    if (tag === "a") return {href:"", download:"", click, remove:vi.fn()} as unknown as HTMLAnchorElement;
    return originalCreateElement(tag as string) as HTMLElement;
  });
  const appendChild = vi.spyOn(document.body, "appendChild").mockImplementation(child => child);
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok:true, status:200, headers:{get:() => "application/zip"}, blob:() => Promise.resolve(blob)
  } as unknown as Response));

  await api.downloadArchive([1, 2], [20]);

  const [path, init] = vi.mocked(fetch).mock.calls[0]!;
  expect(path).toBe("/api/v1/archive");
  expect(JSON.parse(String(init!.body))).toEqual({ids:[1, 2], folders:[20]});
  expect(click).toHaveBeenCalled();
  expect(revokeObjectURL).toHaveBeenCalledWith("blob:archive");
  appendChild.mockRestore();
});

test("downloadArchive surfaces server-side error messages", async () => {
  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok:false, status:413, statusText:"Payload Too Large",
    text:() => Promise.resolve(JSON.stringify({error:"selection too large"}))
  } as unknown as Response));
  await expect(api.downloadArchive([1])).rejects.toThrow("selection too large");

  vi.stubGlobal("fetch", vi.fn().mockResolvedValue({
    ok:false, status:500, statusText:"Internal Server Error",
    text:() => Promise.resolve("not json")
  } as unknown as Response));
  await expect(api.downloadArchive([1])).rejects.toThrow("500 Internal Server Error");
});

test("every remaining endpoint hits its REST resource with the right method", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ok:true}));
  vi.stubGlobal("fetch", fetchMock);
  const calls:[Promise<unknown>, string, string?][] = [
    [api.updateEmail("a@b.c"), "/api/v1/me/email", "PUT"],
    [api.changePassword("old", "new12345"), "/api/v1/me/password", "PUT"],
    [api.forgotPassword("a@b.c"), "/api/v1/auth/forgot-password", "POST"],
    [api.resetPassword("tok", "new12345"), "/api/v1/auth/reset-password", "POST"],
    [api.about(), "/api/v1/about"],
    [api.libraryStats(1), "/api/v1/libraries/1/stats"],
    [api.folderStats(1, 20), "/api/v1/libraries/1/folders/20/stats"],
    [api.favoriteViewStats(30), "/api/v1/favorite-views/30/stats"],
    [api.createLibrary({name:"N", roots:[{path:"/m"}]}), "/api/v1/admin/libraries", "POST"],
    [api.scanLibrary(1), "/api/v1/admin/libraries/1/scan", "POST"],
    [api.createThumbnails(1), "/api/v1/admin/libraries/1/thumbnails", "POST"],
    [api.cleanupOrphanThumbnails(), "/api/v1/admin/thumbnails/orphans", "POST"],
    [api.vacuumDatabase(), "/api/v1/admin/db/vacuum", "POST"],
    [api.users(), "/api/v1/admin/users"],
    [api.createUser({login:"bob", role:"regular", password:"verylongpass1"}), "/api/v1/admin/users", "POST"],
    [api.updateUser(2, {login:"alice", role:"admin"}), "/api/v1/admin/users/2", "PUT"],
    [api.libraryAccess(1), "/api/v1/admin/libraries/1/access"],
    [api.setLibraryAccess(1, 2, true), "/api/v1/admin/libraries/1/access/2", "PUT"],
    [api.scheduledTasks(), "/api/v1/admin/scheduled-tasks"],
    [api.createScheduledTask({name:"n", taskType:"scan", libraryId:1, cron:"* * * * *", enabled:true}), "/api/v1/admin/scheduled-tasks", "POST"],
    [api.updateScheduledTask(5, {name:"n", taskType:"scan", libraryId:1, cron:"* * * * *", enabled:false}), "/api/v1/admin/scheduled-tasks/5", "PUT"],
    [api.deleteScheduledTask(5), "/api/v1/admin/scheduled-tasks/5", "DELETE"],
    [api.jobs(), "/api/v1/admin/jobs"],
    [api.logs(50), "/api/v1/admin/logs?limit=50"],
    [api.clearLogs(), "/api/v1/admin/logs", "DELETE"],
    [api.pauseJob("j1"), "/api/v1/admin/jobs/j1/pause", "POST"],
    [api.resumeJob("j1"), "/api/v1/admin/jobs/j1/resume", "POST"],
    [api.cancelJob("j1"), "/api/v1/admin/jobs/j1/cancel", "POST"],
    [api.shutdown(), "/api/v1/admin/shutdown?mode=docker", "POST"],
    [api.shutdown("signal"), "/api/v1/admin/shutdown?mode=signal", "POST"],
    [api.importEmby({configRoot:"/r", pathMappings:[]}), "/api/v1/admin/import/emby", "POST"],
    [api.filesystem("/media"), "/api/v1/admin/filesystem?path=%2Fmedia"],
    [api.filesystem(), "/api/v1/admin/filesystem?path="],
    [api.entries(1), "/api/v1/libraries/1/entries"],
    [api.entries(1, {offset:10, limit:20}), "/api/v1/libraries/1/entries?offset=10&limit=20"],
    [api.folder(1, 20), "/api/v1/libraries/1/folders/20"],
    [api.folderEntries(1, 20, {limit:5}), "/api/v1/libraries/1/folders/20/entries?limit=5"],
    [api.libraryMedia(1), "/api/v1/libraries/1/media"],
    [api.folderMedia(1, 20), "/api/v1/libraries/1/folders/20/media"],
    [api.favoriteViews(), "/api/v1/favorite-views"],
    [api.createFavoriteView("Best"), "/api/v1/favorite-views", "POST"],
    [api.updateFavoriteView(30, "New"), "/api/v1/favorite-views/30", "PUT"],
    [api.deleteFavoriteView(30), "/api/v1/favorite-views/30", "DELETE"],
    [api.videoThumbnails(100), "/api/v1/media/100/thumbnails"]
  ];
  for (const [pending, url, method] of calls) {
    await pending;
    const matches = fetchMock.mock.calls.filter(([calledUrl, init]) =>
      calledUrl === url && (init.method ?? "GET") === (method ?? "GET"));
    expect(matches.length > 0, `missing ${method ?? "GET"} call to ${url}`).toBe(true);
  }
});

test("MAX_VIDEO_THUMBNAILS matches the backend contract", () => {
  expect(MAX_VIDEO_THUMBNAILS).toBe(100);
});

test("module import survives restricted storage contexts", async () => {
  vi.resetModules();
  const original = window.localStorage;
  Object.defineProperty(window, "localStorage", {
    configurable: true,
    get() { throw new Error("storage blocked"); }
  });
  const fresh = await import("./api");
  Object.defineProperty(window, "localStorage", {value: original, configurable: true});
  expect(typeof fresh.api.me).toBe("function");
});

test("library admin endpoints cover update delete and scan", async () => {
  const fetchMock = vi.fn().mockResolvedValue(jsonResponse({ok:true}));
  vi.stubGlobal("fetch", fetchMock);
  await api.updateLibrary(1, {name:"Renamed", roots:[{path:"/m"}]});
  expect(fetchMock.mock.calls[0]).toEqual(["/api/v1/admin/libraries/1", expect.objectContaining({method:"PUT"})]);
  await api.deleteLibrary(1);
  expect(fetchMock.mock.calls[1]).toEqual(["/api/v1/admin/libraries/1", expect.objectContaining({method:"DELETE"})]);
  await api.map(undefined, undefined, undefined, 30);
  expect(fetchMock.mock.calls[2][0]).toBe("/api/v1/map?favorite=30");
});
