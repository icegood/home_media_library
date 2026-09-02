import type { About, EmbyImportResult, Entry, FavoriteView, FavoriteViewMembership, FilesystemListing, FolderEntries, GeocodeResult, ID, JobStatus, KindStats, Library, LibraryUserAccess, LogTail, MapMedia, Media, MediaFolder, POI, Role, ScheduledTask, User, VideoThumbnail } from "./types";

const base = import.meta.env.VITE_API_URL ?? "/api/v1";

try {
  localStorage.removeItem("token");
} catch {
  // Ignore restricted storage contexts; auth now uses the HttpOnly cookie.
}

function authUrl(path:string) {
  return `${base}${path}`;
}

export function rangeQuery(range?:{offset?:number; limit?:number}) {
  if (!range || (range.offset == null && range.limit == null)) return "";
  const params = new URLSearchParams();
  if (range.offset != null) params.set("offset", String(range.offset));
  if (range.limit != null) params.set("limit", String(range.limit));
  return `?${params.toString()}`;
}

async function call<T>(path:string, init:RequestInit = {}):Promise<T> {
  const response = await fetch(`${base}${path}`, {
    ...init,
    credentials:"same-origin",
    headers: {"Content-Type":"application/json", ...init.headers}
  });
  const contentType = response.headers.get("content-type") ?? "";
  const body = response.status === 204 ? "" : await response.text();
  const json = () => {
    try {
      return body ? JSON.parse(body) : undefined;
    } catch {
      throw new Error(`Expected JSON from ${base}${path}, got ${contentType || "unknown content type"}`);
    }
  };
  if (!response.ok) {
    if (contentType.includes("application/json")) throw new Error(json()?.error ?? response.statusText);
    throw new Error(`${response.status} ${response.statusText || "HTTP error"} from ${base}${path}`);
  }
  if (response.status === 204) return undefined as T;
  if (!contentType.includes("application/json")) throw new Error(`Expected JSON from ${base}${path}, got ${contentType || "unknown content type"}`);
  return json() as T;
}

export const MAX_VIDEO_THUMBNAILS = 100;

export const api = {
  setupStatus: () => call<{required:boolean}>("/setup"),
  setup: (login:string, password:string) =>
    call<User>("/setup", {method:"POST", body:JSON.stringify({login,password})}),
  async login(login:string, password:string) {
    const result = await call<{user:User}>("/auth/login", {method:"POST", body:JSON.stringify({login,password})});
    return result.user;
  },
  logout: () => call<void>("/auth/logout", {method:"POST"}),
  me: () => call<User>("/me"),
  updateEmail: (email:string) =>
    call<{email:string}>("/me/email", {method:"PUT", body:JSON.stringify({email})}),
  changePassword: (currentPassword:string, newPassword:string) =>
    call<{ok:boolean}>("/me/password", {method:"PUT", body:JSON.stringify({currentPassword,newPassword})}),
  forgotPassword: (email:string) =>
    call<{sent:boolean; reason?:string}>("/auth/forgot-password", {method:"POST", body:JSON.stringify({email})}),
  resetPassword: (token:string, password:string) =>
    call<{ok:boolean}>("/auth/reset-password", {method:"POST", body:JSON.stringify({token,password})}),
  userSettings: () => call<UserSettings>("/settings"),
  updateUserSettings: (settings:UserSettings) =>
    call<UserSettings>("/settings", {method:"PUT", body:JSON.stringify(settings)}),
  about: () => call<About>("/about"),
  libraries: () => call<Library[]>("/libraries"),
  libraryStats: (id:ID) => call<KindStats>(`/libraries/${id}/stats`),
  folderStats: (id:ID, folderId:ID) => call<KindStats>(`/libraries/${id}/folders/${folderId}/stats`),
  favoriteViewStats: (id:ID) => call<KindStats>(`/favorite-views/${id}/stats`),
  createLibrary: (input:{name:string; roots:{path:string; watch?:boolean}[]}) =>
    call<Library>("/admin/libraries", {method:"POST", body:JSON.stringify(input)}),
  updateLibrary: (id:ID, input:{name:string; roots:{path:string; watch?:boolean}[]}) =>
    call<Library>(`/admin/libraries/${id}`, {method:"PUT", body:JSON.stringify(input)}),
  deleteLibrary: (id:ID) => call<void>(`/admin/libraries/${id}`, {method:"DELETE"}),
  scanLibrary: (id:ID) => call<JobStatus>(`/admin/libraries/${id}/scan`, {method:"POST"}),
  createThumbnails: (id:ID, input:{recreateExisting?:boolean} = {}) =>
    call<JobStatus>(`/admin/libraries/${id}/thumbnails`, {method:"POST", body:JSON.stringify(input)}),
  cleanupOrphanThumbnails: () => call<JobStatus>("/admin/thumbnails/orphans", {method:"POST"}),
  vacuumDatabase: () => call<JobStatus>("/admin/db/vacuum", {method:"POST"}),
  users: () => call<User[]>("/admin/users"),
  createUser: (input:{login:string; role:Role; password:string}) =>
    call<User>("/admin/users", {method:"POST", body:JSON.stringify(input)}),
  updateUser: (id:ID, input:{login:string; role:Role; password?:string}) =>
    call<User>(`/admin/users/${id}`, {method:"PUT", body:JSON.stringify(input)}),
  libraryAccess: (libraryId:ID) => call<LibraryUserAccess[]>(`/admin/libraries/${libraryId}/access`),
  setLibraryAccess: (libraryId:ID, userId:ID, allowed:boolean) =>
    call<void>(`/admin/libraries/${libraryId}/access/${userId}`, {method:"PUT", body:JSON.stringify({allowed})}),
  scheduledTasks: () => call<ScheduledTask[]>("/admin/scheduled-tasks"),
  createScheduledTask: (input:{name:string; taskType:"scan"|"thumbnail-create"|"vacuum"; libraryId:ID; cron:string; enabled:boolean}) =>
    call<ScheduledTask>("/admin/scheduled-tasks", {method:"POST", body:JSON.stringify(input)}),
  updateScheduledTask: (id:ID, input:{name:string; taskType:"scan"|"thumbnail-create"|"vacuum"; libraryId:ID; cron:string; enabled:boolean}) =>
    call<ScheduledTask>(`/admin/scheduled-tasks/${id}`, {method:"PUT", body:JSON.stringify(input)}),
  deleteScheduledTask: (id:ID) => call<void>(`/admin/scheduled-tasks/${id}`, {method:"DELETE"}),
  jobs: () => call<JobStatus[]>("/admin/jobs"),
  logs: (limit = 300) => call<LogTail>(`/admin/logs?limit=${encodeURIComponent(String(limit))}`),
  clearLogs: () => call<{path:string}>("/admin/logs", {method:"DELETE"}),
  logsDownloadUrl: () => authUrl("/admin/logs/download"),
  pauseJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/pause`, {method:"POST"}),
  resumeJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/resume`, {method:"POST"}),
  cancelJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/cancel`, {method:"POST"}),
  shutdown: (mode:"docker"|"signal" = "docker") => call<{status:string; mode:string}>(`/admin/shutdown?mode=${encodeURIComponent(mode)}`, {method:"POST"}),
  importEmby: (input:{configRoot:string; pathMappings:{from:string; to:string}[]}) =>
    call<EmbyImportResult>("/admin/import/emby", {method:"POST", body:JSON.stringify(input)}),
  filesystem: (path = "") => call<FilesystemListing>(`/admin/filesystem?path=${encodeURIComponent(path)}`),
  entries: (libraryId:ID, range?:{offset?:number; limit?:number}) =>
    call<Entry[]>(`/libraries/${libraryId}/entries${rangeQuery(range)}`),
  folder: (libraryId:ID, folderId:ID) => call<MediaFolder>(`/libraries/${libraryId}/folders/${folderId}`),
  folderEntries: (libraryId:ID, folderId:ID, range?:{offset?:number; limit?:number}) =>
    call<FolderEntries>(`/libraries/${libraryId}/folders/${folderId}/entries${rangeQuery(range)}`),
  libraryMedia: (libraryId:ID) => call<Media[]>(`/libraries/${libraryId}/media`),
  folderMedia: (libraryId:ID, folderId:ID) => call<Media[]>(`/libraries/${libraryId}/folders/${folderId}/media`),
  favoriteViews: () => call<FavoriteView[]>("/favorite-views"),
  mediaFavoriteViews: (id:ID) => call<FavoriteViewMembership[]>(`/media/${id}/favorite-views`),
  folderFavoriteViews: (id:ID) => call<FavoriteViewMembership[]>(`/folders/${id}/favorite-views`),
  createFavoriteView: (name:string) => call<FavoriteView>("/favorite-views", {method:"POST", body:JSON.stringify({name})}),
  updateFavoriteView: (id:ID, name:string) => call<FavoriteView>(`/favorite-views/${id}`, {method:"PUT", body:JSON.stringify({name})}),
  deleteFavoriteView: (id:ID) => call<void>(`/favorite-views/${id}`, {method:"DELETE"}),
  favoriteViewMedia: (id:ID, expand=false) => call<{id:ID; name:string; mimeType?:string; isFolder?:boolean}[]>(`/favorite-views/${id}/media${expand ? "?expand=true" : ""}`),
  favoriteViewMediaFull: (id:ID, expand=false) => call<Media[]>(`/favorite-views/${id}/media?full=true${expand ? "&expand=true" : ""}`),
  favoriteFolder: (viewId:ID, folderId:ID) => call<{ok:boolean}>(`/favorite-views/${viewId}/folders/${folderId}`, {method:"PUT"}),
  unfavoriteFolder: (viewId:ID, folderId:ID) => call<{ok:boolean}>(`/favorite-views/${viewId}/folders/${folderId}`, {method:"DELETE"}),
  media: (id:ID) => call<Media>(`/media/${id}`),
  favoriteMedia: (viewId:ID, id:ID) => call<Media>(`/favorite-views/${viewId}/media/${id}`, {method:"PUT"}),
  unfavoriteMedia: (viewId:ID, id:ID) => call<Media>(`/favorite-views/${viewId}/media/${id}`, {method:"DELETE"}),
  map: (libraryId?:ID, folderId?:ID, bounds?:{west:number; south:number; east:number; north:number}, favoriteViewId?:ID) => {
    const params:string[] = [];
    if (libraryId != null) params.push(`library=${libraryId}`);
    if (folderId != null) params.push(`folder=${folderId}`);
    if (favoriteViewId != null) params.push(`favorite=${favoriteViewId}`);
    if (bounds != null) params.push(`bbox=${bounds.west},${bounds.south},${bounds.east},${bounds.north}`);
    return call<MapMedia[]>(`/map${params.length > 0 ? `?${params.join("&")}` : ""}`);
  },
  poi: (bounds:{west:number; south:number; east:number; north:number}, categories:string[], theme:string) => {
    const params = `bbox=${bounds.west},${bounds.south},${bounds.east},${bounds.north}&categories=${encodeURIComponent(categories.join(","))}&theme=${encodeURIComponent(theme)}`;
    const controller = new AbortController();
    const timer = window.setTimeout(() => controller.abort(), 8000);
    return call<POI[]>(`/map/poi?${params}`, {signal: controller.signal}).finally(() => window.clearTimeout(timer));
  },
  updateGPS: (id:ID, gps:string|null) =>
    call<Media>(`/media/${id}/gps`, {method:"PATCH", body:JSON.stringify({gps})}),
  updateMediaDetails: (id:ID, input:{name:string; gps:string|null; takenAt:string|null}) =>
    call<Media>(`/media/${id}/details`, {method:"PATCH", body:JSON.stringify(input)}),
  setTrajectoryStart: (id:ID, folderId:ID, start:boolean) =>
    call<{folderId:ID; mediaId:ID; start:boolean; trajectoryStart:boolean}>(`/media/${id}/trajectory-start`, {method:"PATCH", body:JSON.stringify({folderId, start})}),
  setTrajectoryEnd: (id:ID, folderId:ID, end:boolean) =>
    call<{folderId:ID; mediaId:ID; end:boolean; trajectoryEnd:boolean}>(`/media/${id}/trajectory-end`, {method:"PATCH", body:JSON.stringify({folderId, end})}),
  setTrajectoryName: (id:ID, folderId:ID, name:string) =>
    call<{folderId:ID; mediaId:ID; name:string}>(`/media/${id}/trajectory-name`, {method:"PATCH", body:JSON.stringify({folderId, name})}),
  bulkUpdateMedia: (input:{selectedIds?:ID[]; selectedFolders?:ID[]; gps?:string|null; takenAt?:string|null; shiftMinutes?:number|null}) =>
    call<{id:ID; takenAt?:string; gps?:string}[]>(`/media/bulk`, {method:"PATCH", body:JSON.stringify(input)}),
  metadataRenew: (libraryId:ID, input:{recreateExisting?:boolean; updateGps?:boolean; updateTakenAt?:boolean}) =>
    call<JobStatus>(`/admin/libraries/${libraryId}/metadata/renew`, {method:"POST", body:JSON.stringify(input)}),
  settings: () => call<AdminSettings>("/admin/settings"),
  updateSettings: (settings:AdminSettings) =>
    call<AdminSettings>("/admin/settings", {method:"PUT", body:JSON.stringify(settings)}),
  thumbnailUrl: (id:ID, index = 0) => authUrl(`/media/${id}/thumbnail?index=${index}`),
  folderThumbnailUrl: (id:ID) => authUrl(`/folders/${id}/thumbnail`),
  videoThumbnails: (id:ID) => call<VideoThumbnail[]>(`/media/${id}/thumbnails`),
  async geocode(query:string) {
    const params = new URLSearchParams({q:query, format:"jsonv2", limit:"5", "accept-language":navigator.language ?? "en"});
    const response = await fetch(`https://nominatim.openstreetmap.org/search?${params.toString()}`, {headers:{Accept:"application/json"}});
    if (!response.ok) throw new Error(`Geocoder returned HTTP ${response.status}`);
    let results: unknown;
    try {
      results = JSON.parse(await response.text());
    } catch {
      throw new Error("Geocoder returned an invalid response");
    }
    if (!Array.isArray(results)) throw new Error("Geocoder returned an invalid response");
    return results as GeocodeResult[];
  },
  contentUrl: (id:ID, download = false) => authUrl(`/media/${id}/content${download ? "?download=1" : ""}`),
  async documentContent(id:ID) {
    // In Android Capacitor the WebView's built-in PDF viewer fires a separate
    // request without sending the HttpOnly auth cookie and surfaces "401
    // Authorization required". Proxy the file through the authenticated
    // fetch helper and hand the iframe a blob URL so the auth state stays
    // attached.
    const response = await fetch(authUrl(`/media/${id}/content`), {credentials:"same-origin"});
    if (!response.ok) throw new Error(`${response.status} ${response.statusText || "HTTP error"} from ${base}/media/${id}/content`);
    const blob = await response.blob();
    return URL.createObjectURL(blob);
  },
  async downloadArchive(ids:ID[], folders:ID[] = []) {
    const response = await fetch(`${base}/archive`, {
      method:"POST",
      credentials:"same-origin",
      headers:{"Content-Type":"application/json"},
      body:JSON.stringify({ids, folders})
    });
    if (!response.ok) {
      const body = await response.text();
      let message = `${response.status} ${response.statusText || "HTTP error"}`;
      try {
        const parsed: {error?:string}|null = JSON.parse(body);
        if (parsed && parsed.error) message = parsed.error;
      } catch {
        // Non-JSON error body; keep the status-based message.
      }
      throw new Error(message);
    }
    const blob = await response.blob();
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = "media-archive.zip";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
    URL.revokeObjectURL(url);
  },
  playbackUrl: (id:ID, codecs:string[], start = 0) =>
    authUrl(`/media/${id}/play?codecs=${encodeURIComponent(codecs.join(","))}${start > 0 ? `&start=${start}` : ""}`)
};

export interface AdminSettings {
  httpEnabled:boolean;
  httpsEnabled:boolean;
  publicDns:string;
  acmeEmail:string;
  httpsCertificateExpiresAt:string;
  httpsGatewayEnabled:boolean;
  thumbnailWidth:number;
  thumbnailHeight:number;
  videoThumbnailFirstSeconds:number;
  videoThumbnailMaxCount:number;
  videoThumbnailMinIntervalSeconds:number;
  workerPoolSize:number;
  sessionMaxAgeHours:number;
  finishedJobRetentionMinutes:number;
  logLevel:"D"|"I"|"W"|"E";
  logRotateMaxSizeMB:number;
  logRotateMaxBackups:number;
  logRotateMaxAgeDays:number;
  smtpHost:string;
  smtpPort:number;
  smtpUsername:string;
  smtpPassword?:string;
  smtpFrom:string;
  mapTileProviders?:Record<string, Record<string, string>>;
  poiProviders?:Record<string, Record<string, string>>;
}

export type TranscodeSchemaId =
  | "h264-aac-mp4"
  | "h264-opus-mp4"
  | "vp9-opus-webm"
  | "vp9-vorbis-webm"
  | "av1-opus-webm"
  | "hevc-aac-mp4"
  | "hevc-opus-mp4"
  | "vp8-vorbis-webm"
  | "vp8-opus-webm";

export type LanguageSetting = "auto" | "en" | "ua" | "de" | "nl" | "fi" | "sv" | "pl" | "che" | "slo" | "hu" | "es" | "it" | "sl" | "no" | "pt";
export interface UserSettings {
  theme:"light"|"dark"|"forest"|"system";
  language:LanguageSetting;
  codec:TranscodeSchemaId;
  zoom:number;
  dateFormat:"auto"|"iso"|"dmy"|"dmy-ss"|"mdy"|"mdy-ss";
  streamChunkSize:number;
  defaultThumbImage:string;
  defaultThumbVideo:string;
  defaultThumbFolder:string;
  mapTileProviderLight:MapTileSource;
  mapTileProviderDark:MapTileSource;
  mapTileProviders?:Record<string, Record<string, string>>;
  poiProviderLight:POISource;
  poiProviderDark:POISource;
  poiProviders?:Record<string, Record<string, string>>;
}

export type MapTileSource = "osm" | "esri" | "esri:satellite" | "carto" | "carto:voyager" | "carto:light" | "carto:dark";
export type POISource = "overpass" | "geoapify" | "mapbox";
