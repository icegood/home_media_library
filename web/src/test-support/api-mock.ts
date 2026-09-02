// Maximally functional in-memory replacement for the REST client used by unit
// tests. Mocking stays at the API boundary only: every method behaves like the
// real backend (stateful CRUD, derived responses, URL builders) instead of
// returning ad-hoc literals, so component tests exercise realistic flows.
// Each method is a vi.fn wrapping its default implementation: tests may still
// override single methods, and reset() restores both state and defaults.
import { vi } from "vitest";
import type { AdminSettings } from "../api";
import type { About, Media, User } from "../types";

interface POIPoint {id:string; name:string; category:string; lat:number; lon:number; website?:string; wikipediaTitle?:string}

interface LibraryRow { id:number; name:string; roots:{id:number; path:string; watch?:boolean}[] }
interface FolderRow { id:number; parentId:number; relativePath:string; name:string }
interface FavoriteViewRow { id:number; name:string; count:number }
interface JobRow { id:string; category:string; status:string; paused?:boolean; cancelable?:boolean }
interface TaskRow { id:number; name:string; taskType:string; libraryId:number; cron:string; enabled:boolean }
interface AccessRow { user:User; allowed:boolean }

function mediaRow(overrides:Partial<Media> = {}):Media {
  return {id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"", ...overrides};
}

function createMockApi() {
  let nextId = 1000;
  let state:any;

  function resetState() {
    nextId = 1000;
    state = {
      setupRequired:false,
      currentUser:null as User|null,
      users:[
        {id:0, login:"admin", role:"admin"},
        {id:2, login:"alice", role:"regular"}
      ] as User[],
      libraries:[
        {id:1, name:"Family", roots:[{id:10, path:"/media/family/photos"}]}
      ] as LibraryRow[],
      access:new Map<number, AccessRow[]>([[1, [
        {user:{id:0, login:"admin", role:"admin"}, allowed:true},
        {user:{id:2, login:"alice", role:"regular"}, allowed:false}
      ]]]),
      folders:new Map<number, FolderRow>([[20, {id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]]),
      folderChain:new Map<number, FolderRow[]>([[20, [{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]]]),
      entries:new Map<number, unknown[]>([[1, []]]),
      folderEntries:new Map<number, unknown[]>([[20, []]]),
      media:new Map<number, Media>([
        [100, mediaRow()],
        [999, mediaRow({id:999})]
      ]),
      libraryMedia:new Map<number, Media[]>(),
      favoriteViews:[] as FavoriteViewRow[],
      favoriteItems:new Map<number, Set<number>>(),
      jobs:[] as JobRow[],
      tasks:[] as TaskRow[],
      logLines:[
        "2026/07/31 08:00:00 I media API listening on :8080",
        "2026/07/31 08:01:00 E thumbnail failed"
      ],
      mapPoints:[] as unknown[],
      poiPoints:[] as POIPoint[],
      userSettings:{theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", language:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", mapTileProviderLight:"osm", mapTileProviderDark:"osm", mapTileProviders:{carto:{apiKey:""}}, poiProviderLight:"overpass", poiProviderDark:"overpass", poiProviders:{overpass:{endpoint:""}}},
      adminSettings:{
        httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"", httpsGatewayEnabled:true,
        thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:100, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
        sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30,
        smtpHost:"", smtpPort:587, smtpUsername:"", smtpPassword:"", smtpFrom:"", mapTileProviders:{carto:{apiKey:""}}, poiProviders:{overpass:{endpoint:""}}
      } as AdminSettings,
      filesystem:{
        "/media":{root:"/media", path:"/media", parent:"", directories:[{name:"photos", path:"/media/photos"}]},
        "/media/photos":{root:"/media", path:"/media/photos", parent:"/media", directories:[]}
      } as Record<string, unknown>
    };
  }

  function findLibrary(id:number) {
    return state.libraries.find((library:LibraryRow) => library.id === Number(id));
  }
  function requireJob(id:string):JobRow {
    const job = state.jobs.find((row:JobRow) => row.id === id);
    if (!job) throw new Error(`unknown job ${id}`);
    return job;
  }

  resetState();

  const api = {
    // --- bootstrap / auth -------------------------------------------------
    setupStatus: vi.fn(async () => ({required:state.setupRequired})),
    setup: vi.fn(async (login:string, password:string) => {
      if (!login || !password) throw new Error("login and password are required");
      state.setupRequired = false;
      const admin = {id:0, login, role:"admin"} as User;
      state.users.unshift(admin);
      state.currentUser = admin;
      return {...admin};
    }),
    login: vi.fn(async (login:string, password:string) => {
      const user = state.users.find((row:User) => row.login === login);
      if (!user || !password || password === "wrong") throw new Error("invalid credentials");
      state.currentUser = user;
      return user;
    }),
    logout: vi.fn(async () => { state.currentUser = null; }),
    me: vi.fn(async () => {
      if (!state.currentUser) throw new Error("unauthorized");
      return state.currentUser;
    }),
    updateEmail: vi.fn(async (email:string) => {
      if (state.currentUser) state.currentUser.email = email;
      return {email};
    }),
    changePassword: vi.fn(async (currentPassword:string, newPassword:string) => {
      if (currentPassword === "bad") throw new Error("wrong password");
      void newPassword;
      return {ok:true};
    }),
    forgotPassword: vi.fn(async (email:string) => ({sent:Boolean(email)})),
    resetPassword: vi.fn(async () => ({ok:true})),
    // --- per-user preferences --------------------------------------------
    userSettings: vi.fn(async () => ({...state.userSettings})),
    updateUserSettings: vi.fn(async (settings:typeof state.userSettings) => {
      state.userSettings = {...state.userSettings, ...settings};
      return {...state.userSettings};
    }),
    about: vi.fn(async ():Promise<About> => ({product:"Media Library", version:"0.1.0", revision:"abc123", buildDate:"2026-01-01T00:00:00Z", goVersion:"go1.23", gatewayEnabled:false})),
    // --- libraries --------------------------------------------------------
    libraries: vi.fn(async () => JSON.parse(JSON.stringify(state.libraries))),
    libraryStats: vi.fn(async () => ({images:10, videos:2, documents:0})),
    folderStats: vi.fn(async (_id:number, folderId:number) => {
      void folderId;
      return {images:1, videos:1, documents:0};
    }),
    favoriteViewStats: vi.fn(async (id:number) => {
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(id));
      return {images:view ? Math.max(view.count - 1, 0) : 0, videos:view ? 1 : 0, documents:0};
    }),
    createLibrary: vi.fn(async (input:{name:string; roots:{path:string; watch?:boolean}[]}) => {
      const library:LibraryRow = {id:++nextId, name:input.name, roots:[]};
      library.roots = input.roots.map(root => ({id:++nextId, path:root.path, watch:root.watch}));
      state.libraries.push(library);
      state.access.set(library.id, []);
      return JSON.parse(JSON.stringify(library));
    }),
    updateLibrary: vi.fn(async (id:number, input:{name:string; roots:{path:string; watch?:boolean}[]}) => {
      const library = findLibrary(id);
      if (!library) throw new Error(`library ${id} not found`);
      library.name = input.name;
      library.roots = input.roots.map((root, index) => ({id:(library.roots[index] ?? {id:++nextId}).id as number, path:root.path, watch:root.watch}));
      return JSON.parse(JSON.stringify(library));
    }),
    deleteLibrary: vi.fn(async (id:number) => {
      state.libraries = state.libraries.filter((library:LibraryRow) => library.id !== Number(id));
    }),
    scanLibrary: vi.fn(async (id:number) => {
      const job:JobRow = {id:`scan-${id}`, category:"scan", status:"running"};
      state.jobs.push(job);
      return {...job};
    }),
    createThumbnails: vi.fn(async (id:number, input:{recreateExisting?:boolean} = {}) => {
      void input;
      const job:JobRow = {id:`thumb-${id}`, category:"thumbnail-create", status:"running"};
      state.jobs.push(job);
      return {...job};
    }),
    metadataRenew: vi.fn(async (id:number, input:{recreateExisting?:boolean; updateGps?:boolean; updateTakenAt?:boolean}) => {
      void input;
      const job:JobRow = {id:`metadata-${id}`, category:"metadata-renew", status:"running"};
      state.jobs.push(job);
      return {...job};
    }),
    cleanupOrphanThumbnails: vi.fn(async () => {
      const job:JobRow = {id:"orphans-1", category:"orphan-thumbnail-cleanup", status:"running"};
      state.jobs.push(job);
      return {...job};
    }),
    vacuumDatabase: vi.fn(async () => {
      const job:JobRow = {id:"vacuum-1", category:"vacuum", status:"running"};
      state.jobs.push(job);
      return {...job};
    }),
    // --- users & permissions ----------------------------------------------
    users: vi.fn(async () => state.users.map((row:User) => ({...row}))),
    createUser: vi.fn(async (input:{login:string; role:string; password:string}) => {
      const user = {id:++nextId, login:input.login, role:input.role} as User;
      state.users.push(user);
      return {...user};
    }),
    updateUser: vi.fn(async (id:number, input:{login:string; role:string}) => {
      const user = state.users.find((row:User) => row.id === Number(id));
      if (!user) throw new Error(`user ${id} not found`);
      Object.assign(user, input);
      return {...user};
    }),
    libraryAccess: vi.fn(async (libraryId:number) =>
      JSON.parse(JSON.stringify(state.access.get(Number(libraryId)) ?? []))),
    setLibraryAccess: vi.fn(async (libraryId:number, userId:number, allowed:boolean) => {
      const rows:AccessRow[] = state.access.get(Number(libraryId)) ?? [];
      const existing = rows.find(row => row.user.id === Number(userId));
      if (existing) existing.allowed = allowed;
      else {
        const user = state.users.find((row:User) => row.id === Number(userId));
        rows.push({user:{...(user ?? {id:Number(userId), login:`u${userId}`, role:"regular"})}, allowed});
      }
      state.access.set(Number(libraryId), rows);
    }),
    // --- admin maintenance -------------------------------------------------
    scheduledTasks: vi.fn(async () => state.tasks.map((row:TaskRow) => ({...row}))),
    createScheduledTask: vi.fn(async (input:Omit<TaskRow,"id">) => {
      const task:TaskRow = {id:++nextId, ...input};
      state.tasks.push(task);
      return {...task};
    }),
    updateScheduledTask: vi.fn(async (id:number, input:Omit<TaskRow,"id">) => {
      const task = state.tasks.find((row:TaskRow) => row.id === Number(id));
      if (!task) throw new Error(`task ${id} not found`);
      Object.assign(task, input);
      return {...task};
    }),
    deleteScheduledTask: vi.fn(async (id:number) => {
      state.tasks = state.tasks.filter((task:TaskRow) => task.id !== Number(id));
    }),
    jobs: vi.fn(async () => state.jobs.map((job:JobRow) => ({...job}))),
    logs: vi.fn(async () => ({path:"/runtime/app-config/logs/app.log", lines:[...state.logLines]})),
    clearLogs: vi.fn(async () => { state.logLines = []; return {path:"/runtime/app-config/logs/app.log"}; }),
    logsDownloadUrl: vi.fn(() => "/api/v1/admin/logs/download"),
    pauseJob: vi.fn(async (id:string) => {
      const job = requireJob(id);
      job.status = "paused";
      job.paused = true;
      job.cancelable = true;
      return {...job};
    }),
    resumeJob: vi.fn(async (id:string) => {
      const job = requireJob(id);
      job.status = "running";
      job.paused = false;
      job.cancelable = true;
      return {...job};
    }),
    cancelJob: vi.fn(async (id:string) => {
      const job = requireJob(id);
      job.status = "cancelling";
      job.paused = false;
      job.cancelable = true;
      return {...job};
    }),
    shutdown: vi.fn(async (mode:"docker"|"signal" = "docker") => ({status:"stopping", mode})),
    importEmby: vi.fn(async () => ({libraries:[], users:[], access:[]})),
    filesystem: vi.fn(async (path = "") => {
      const listing = state.filesystem[path || "/media"];
      if (!listing) throw new Error(`no such directory: ${path}`);
      return JSON.parse(JSON.stringify(listing));
    }),
    // --- content browsing --------------------------------------------------
    entries: vi.fn(async (libraryId:number, range?:{offset?:number; limit?:number}) => {
      const all = state.entries.get(Number(libraryId)) ?? [];
      const offset = range?.offset ?? 0;
      return JSON.parse(JSON.stringify(range?.limit == null ? all.slice(offset) : all.slice(offset, offset + range.limit)));
    }),
    folder: vi.fn(async (_libraryId:number, folderId:number) => {
      const folder = state.folders.get(Number(folderId));
      if (!folder) throw new Error(`folder ${folderId} not found`);
      return {...folder};
    }),
    folderEntries: vi.fn(async (_libraryId:number, folderId:number) => ({
      entries:JSON.parse(JSON.stringify(state.folderEntries.get(Number(folderId)) ?? [])),
      chain:JSON.parse(JSON.stringify(state.folderChain.get(Number(folderId)) ?? []))
    })),
    libraryMedia: vi.fn(async (libraryId:number) =>
      JSON.parse(JSON.stringify(state.libraryMedia.get(Number(libraryId)) ?? [...state.media.values()].filter(row => row.kind === "image")))),
    folderMedia: vi.fn(async (_libraryId:number, folderId:number) =>
      JSON.parse(JSON.stringify([...state.media.values()].filter(row => row.folderId === Number(folderId))))),
    media: vi.fn(async (id:number) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      return JSON.parse(JSON.stringify(row));
    }),
    videoThumbnails: vi.fn(async () => []),
    map: vi.fn(async (_libraryId?:number, _folderId?:number, bounds?:{west:number; south:number; east:number; north:number}) => {
      const points = state.mapPoints as {lat:number; lng:number; id:number; name:string}[];
      const filtered = bounds ? points.filter(point => point.lat >= bounds.south && point.lat <= bounds.north && point.lng >= bounds.west && point.lng <= bounds.east) : points;
      return JSON.parse(JSON.stringify(filtered));
    }),
    poi: vi.fn(async (bounds:{west:number; south:number; east:number; north:number}, _categories:string[], _theme:string) => {
      const points:POIPoint[] = state.poiPoints;
      const filtered = bounds ? points.filter(point => point.lat >= bounds.south && point.lat <= bounds.north && point.lon >= bounds.west && point.lon <= bounds.east) : points;
      return JSON.parse(JSON.stringify(filtered));
    }),
    geocode: vi.fn(async () => []),
    updateGPS: vi.fn(async (id:number, gps:string|null) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      row.gps = gps ?? "";
      return JSON.parse(JSON.stringify(row));
    }),
    updateMediaDetails: vi.fn(async (id:number, input:{name:string; gps:string|null; takenAt:string|null}) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      Object.assign(row, {name:input.name, gps:input.gps ?? "", takenAt:input.takenAt ?? ""});
      return JSON.parse(JSON.stringify(row));
    }),
    setTrajectoryStart: vi.fn(async (id:number, folderId:number, start:boolean) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      row.trajectoryStart = start;
      return {folderId:Number(folderId), mediaId:row.id, start, trajectoryStart:start};
    }),
    setTrajectoryEnd: vi.fn(async (id:number, folderId:number, end:boolean) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      row.trajectoryEnd = end;
      return {folderId:Number(folderId), mediaId:row.id, end, trajectoryEnd:end};
    }),
    setTrajectoryName: vi.fn(async (id:number, folderId:number, name:string) => {
      const row = state.media.get(Number(id));
      if (!row) throw new Error(`media ${id} not found`);
      row.trajectoryName = name;
      return {folderId:Number(folderId), mediaId:row.id, name};
    }),
    bulkUpdateMedia: vi.fn(async (input:{selectedIds?:number[]; selectedFolders?:number[]; gps?:string|null; takenAt?:string|null; shiftMinutes?:number|null}) => {
      const targets = [...state.media.values()].filter(row =>
        (input.selectedIds?.includes(row.id)) === true ||
        (input.selectedFolders?.includes(row.folderId)) === true);
      for (const row of targets) {
        if (input.gps !== undefined) row.gps = input.gps ?? "";
        if (input.takenAt !== undefined) row.takenAt = input.takenAt ?? "";
        if (input.shiftMinutes != null && row.takenAt) {
          row.takenAt = new Date(new Date(row.takenAt).getTime() + input.shiftMinutes * 60000).toISOString();
        }
      }
      return targets.map(row => ({id:row.id, takenAt:row.takenAt || undefined, gps:row.gps || undefined}));
    }),
    // --- favorites ----------------------------------------------------------
    favoriteViews: vi.fn(async () => JSON.parse(JSON.stringify(state.favoriteViews))),
    createFavoriteView: vi.fn(async (name:string) => {
      const view:FavoriteViewRow = {id:++nextId, name, count:0};
      state.favoriteViews.push(view);
      state.favoriteItems.set(view.id, new Set());
      return {...view};
    }),
    updateFavoriteView: vi.fn(async (id:number, name:string) => {
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(id));
      if (!view) throw new Error(`favorite view ${id} not found`);
      view.name = name;
      return {...view};
    }),
    deleteFavoriteView: vi.fn(async (id:number) => {
      state.favoriteViews = state.favoriteViews.filter((view:FavoriteViewRow) => view.id !== Number(id));
      state.favoriteItems.delete(Number(id));
    }),
    favoriteViewMedia: vi.fn(async (id:number, expand=false) => {
      const ids = state.favoriteItems.get(Number(id)) ?? new Set();
      return [...ids].map(itemId => {
        const row = state.media.get(itemId);
        return expand && row ? {...JSON.parse(JSON.stringify(row))} : {id:itemId, name:row?.name ?? `item-${itemId}`, kind:row?.kind};
      });
    }),
    favoriteViewMediaFull: vi.fn(async (id:number) => {
      const ids = state.favoriteItems.get(Number(id)) ?? new Set();
      return [...ids].map(itemId => JSON.parse(JSON.stringify(state.media.get(itemId) ?? mediaRow({id:itemId})))).filter(Boolean);
    }),
    favoriteFolder: vi.fn(async (viewId:number, folderId:number) => {
      void folderId;
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(viewId));
      if (view) view.count += 1;
      return {ok:true};
    }),
    unfavoriteFolder: vi.fn(async (viewId:number, folderId:number) => {
      void folderId;
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(viewId));
      if (view) view.count = Math.max(0, view.count - 1);
      return {ok:true};
    }),
    mediaFavoriteViews: vi.fn(async (id:number) => {
      return state.favoriteViews
        .filter((view:FavoriteViewRow) => state.favoriteItems.get(view.id)?.has(Number(id)))
        .map((view:FavoriteViewRow) => ({id:view.id, name:view.name}));
    }),
    folderFavoriteViews: vi.fn(async () => []),
    favoriteMedia: vi.fn(async (viewId:number, id:number) => {
      const items = state.favoriteItems.get(Number(viewId)) ?? new Set();
      items.add(Number(id));
      state.favoriteItems.set(Number(viewId), items);
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(viewId));
      if (view) view.count = items.size;
      const row = state.media.get(Number(id));
      return JSON.parse(JSON.stringify({...mediaRow(), ...(row ?? {}), favorite:true}));
    }),
    unfavoriteMedia: vi.fn(async (viewId:number, id:number) => {
      state.favoriteItems.get(Number(viewId))?.delete(Number(id));
      const view = state.favoriteViews.find((row:FavoriteViewRow) => row.id === Number(viewId));
      if (view) view.count = (state.favoriteItems.get(Number(viewId)) ?? new Set()).size;
      const row = state.media.get(Number(id));
      return JSON.parse(JSON.stringify({...mediaRow(), ...(row ?? {}), favorite:false}));
    }),
    // --- urls & downloads ---------------------------------------------------
    thumbnailUrl: vi.fn((id:number, index = 0) => `/api/v1/media/${id}/thumbnail?index=${index}`),
    folderThumbnailUrl: vi.fn((id:number) => `/api/v1/folders/${id}/thumbnail`),
    contentUrl: vi.fn((id:number, download = false) => `/api/v1/media/${id}/content${download ? "?download=1" : ""}`),
    playbackUrl: vi.fn((id:number, codecs:string[], start = 0) =>
      `/api/v1/media/${id}/play?codecs=${encodeURIComponent(codecs.join(","))}${start > 0 ? `&start=${start}` : ""}`),
    downloadArchive: vi.fn(async () => {}),
    documentContent: vi.fn(async (_id:number) => "blob:mock-document"),
    // --- admin settings -------------------------------------------------------
    settings: vi.fn(async () => ({...state.adminSettings})),
    updateSettings: vi.fn(async (settings:AdminSettings) => {
      state.adminSettings = {...state.adminSettings, ...settings};
      return {...state.adminSettings};
    })
  };

  // Captured exactly once, before any test can override a method, so reset()
  // always falls back to the functional defaults rather than a previous
  // test's overrides.
  const defaultImpls = new Map<string, unknown>(Object.entries(api)
    .map(([name, spy]) => [name, (spy as {getMockImplementation?:() => unknown}).getMockImplementation?.()]));

  return {
    api,
    // Lets a test plant scenario data directly into the backend state, so
    // every method (including derived ones like stats) sees a consistent
    // world instead of overriding single endpoints.
    seed(mutator:(state:any) => void) {
      mutator(state);
    },
    reset() {
      resetState();
      for (const [name, spy] of Object.entries(api)) {
        const mockable = spy as {mockReset?:unknown; mockImplementation?:(impl:unknown) => void};
        if (typeof mockable.mockReset !== "function") continue;
        (spy as {mockReset:() => void}).mockReset();
        const fallback = defaultImpls.get(name);
        // Re-applying the captured implementation keeps this independent of
        // whether the Vitest version's mockReset restores originals itself.
        if (typeof mockable.mockImplementation === "function" && typeof fallback === "function") {
          mockable.mockImplementation(fallback);
        }
      }
    }
  };
}

const instance = createMockApi();

// The scenario seeder and state reset live directly on the exported mock so
// tests can reach them through the same object they override methods on.
// Deliberately loose: tests override individual methods with scenario data
// whose shapes may be narrower than the functional defaults.
type LooseApi = Record<string, any>;
export const mockApi = instance.api as unknown as LooseApi & {
  seed:(mutator:(state:any) => void) => void;
  resetAll:() => void;
};
mockApi.seed = instance.seed;
mockApi.resetAll = instance.reset;

export function resetMockApi() {
  mockApi.resetAll();
}
