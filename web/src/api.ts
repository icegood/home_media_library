import type { EmbyImportResult, Entry, FavoriteView, FilesystemListing, ID, JobStatus, Library, LibraryUserAccess, LogTail, MapMedia, Media, MediaFolder, Role, User, VideoThumbnail } from "./types";

const base = import.meta.env.VITE_API_URL ?? "/api/v1";

try {
  localStorage.removeItem("token");
} catch {
  // Ignore restricted storage contexts; auth now uses the HttpOnly cookie.
}

function authUrl(path:string) {
  return `${base}${path}`;
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
  userSettings: () => call<UserSettings>("/settings"),
  updateUserSettings: (settings:UserSettings) =>
    call<UserSettings>("/settings", {method:"PUT", body:JSON.stringify(settings)}),
  libraries: () => call<Library[]>("/libraries"),
  createLibrary: (input:{name:string; roots:{path:string}[]}) =>
    call<Library>("/admin/libraries", {method:"POST", body:JSON.stringify(input)}),
  updateLibrary: (id:ID, input:{name:string; roots:{path:string}[]}) =>
    call<Library>(`/admin/libraries/${id}`, {method:"PUT", body:JSON.stringify(input)}),
  deleteLibrary: (id:ID) => call<void>(`/admin/libraries/${id}`, {method:"DELETE"}),
  scanLibrary: (id:ID) => call<JobStatus>(`/admin/libraries/${id}/scan`, {method:"POST"}),
  createThumbnails: (id:ID, input:{recreateExisting?:boolean} = {}) =>
    call<JobStatus>(`/admin/libraries/${id}/thumbnails`, {method:"POST", body:JSON.stringify(input)}),
  cleanupOrphanThumbnails: () => call<JobStatus>("/admin/thumbnails/orphans", {method:"POST"}),
  users: () => call<User[]>("/admin/users"),
  createUser: (input:{login:string; role:Role; password:string}) =>
    call<User>("/admin/users", {method:"POST", body:JSON.stringify(input)}),
  updateUser: (id:ID, input:{login:string; role:Role; password?:string}) =>
    call<User>(`/admin/users/${id}`, {method:"PUT", body:JSON.stringify(input)}),
  libraryAccess: (libraryId:ID) => call<LibraryUserAccess[]>(`/admin/libraries/${libraryId}/access`),
  setLibraryAccess: (libraryId:ID, userId:ID, allowed:boolean) =>
    call<void>(`/admin/libraries/${libraryId}/access/${userId}`, {method:"PUT", body:JSON.stringify({allowed})}),
  jobs: () => call<JobStatus[]>("/admin/jobs"),
  logs: (limit = 300) => call<LogTail>(`/admin/logs?limit=${encodeURIComponent(String(limit))}`),
  pauseJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/pause`, {method:"POST"}),
  resumeJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/resume`, {method:"POST"}),
  cancelJob: (id:string) => call<JobStatus>(`/admin/jobs/${id}/cancel`, {method:"POST"}),
  shutdown: (mode:"docker"|"signal" = "docker") => call<{status:string; mode:string}>(`/admin/shutdown?mode=${encodeURIComponent(mode)}`, {method:"POST"}),
  importEmby: (input:{configRoot:string; pathMappings:{from:string; to:string}[]}) =>
    call<EmbyImportResult>("/admin/import/emby", {method:"POST", body:JSON.stringify(input)}),
  filesystem: (path = "") => call<FilesystemListing>(`/admin/filesystem?path=${encodeURIComponent(path)}`),
  entries: (libraryId:ID) => call<Entry[]>(`/libraries/${libraryId}/entries`),
  folder: (libraryId:ID, folderId:ID) => call<MediaFolder>(`/libraries/${libraryId}/folders/${folderId}`),
  folderEntries: (libraryId:ID, folderId:ID) => call<Entry[]>(`/libraries/${libraryId}/folders/${folderId}/entries`),
  libraryMedia: (libraryId:ID) => call<Media[]>(`/libraries/${libraryId}/media`),
  favoriteViews: () => call<FavoriteView[]>("/favorite-views"),
  createFavoriteView: (name:string) => call<FavoriteView>("/favorite-views", {method:"POST", body:JSON.stringify({name})}),
  updateFavoriteView: (id:ID, name:string) => call<FavoriteView>(`/favorite-views/${id}`, {method:"PUT", body:JSON.stringify({name})}),
  deleteFavoriteView: (id:ID) => call<void>(`/favorite-views/${id}`, {method:"DELETE"}),
  favoriteViewMedia: (id:ID) => call<Media[]>(`/favorite-views/${id}/media`),
  media: (id:ID) => call<Media>(`/media/${id}`),
  favoriteMedia: (viewId:ID, id:ID) => call<Media>(`/favorite-views/${viewId}/media/${id}`, {method:"PUT"}),
  unfavoriteMedia: (viewId:ID, id:ID) => call<Media>(`/favorite-views/${viewId}/media/${id}`, {method:"DELETE"}),
  map: () => call<MapMedia[]>("/map"),
  updateGPS: (id:ID, gps:string|null) =>
    call<Media>(`/media/${id}/gps`, {method:"PATCH", body:JSON.stringify({gps})}),
  updateMediaDetails: (id:ID, input:{name:string; gps:string|null; takenAt:string|null}) =>
    call<Media>(`/media/${id}/details`, {method:"PATCH", body:JSON.stringify(input)}),
  settings: () => call<AdminSettings>("/admin/settings"),
  updateSettings: (settings:AdminSettings) =>
    call<AdminSettings>("/admin/settings", {method:"PUT", body:JSON.stringify(settings)}),
  thumbnailUrl: (id:ID, index = 0) => authUrl(`/media/${id}/thumbnail?index=${index}`),
  folderThumbnailUrl: (id:ID) => authUrl(`/folders/${id}/thumbnail`),
  videoThumbnails: (id:ID) => call<VideoThumbnail[]>(`/media/${id}/thumbnails`),
  contentUrl: (id:ID) => authUrl(`/media/${id}/content`),
  playbackUrl: (id:ID, codecs:string[]) =>
    authUrl(`/media/${id}/play?codecs=${encodeURIComponent(codecs.join(","))}`)
};

export interface AdminSettings {
  transcodeCodec:"h264"|"h265"|"vp9";
  httpEnabled:boolean;
  httpsEnabled:boolean;
  publicDns:string;
  acmeEmail:string;
  httpsCertificateExpiresAt:string;
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
}

export interface UserSettings {
  theme:"light"|"dark";
}
