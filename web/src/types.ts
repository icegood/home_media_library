export type Role = "admin" | "regular";
export type ID = number;
export interface User { id:ID; login:string; role:Role; email?:string }
export interface LibraryUserAccess { user:User; allowed:boolean }
export interface LibraryRoot { id:ID; path?:string; watch?:boolean }
export interface KindStats { images:number; videos:number; documents:number }
export interface Library { id:ID; name:string; roots?:LibraryRoot[]; stats?:KindStats }
export interface FavoriteView { id:ID; name:string; count:number }
export interface FavoriteViewMembership extends FavoriteView { contains:boolean }
export interface Media {
  id:ID; folderId:ID; relativePath:string; name:string;
  kind:"image"|"video"|"document"; mimeType:string; size:number;
  metadata:Record<string, unknown>; gps:string; takenAt:string;
  metadataError?:string; thumbnailError?:string;
  favorite?:boolean;
  trajectoryStart?:boolean;
  trajectoryEnd?:boolean;
  trajectoryName?:string;
}
export interface MapMedia extends Media {
  libraryId:ID;
}
export type POICategory = "food" | "fuel_parking" | "lodging" | "attraction" | "health" | "shops";
export interface POI {
  id:string;
  name:string;
  category:POICategory;
  lat:number;
  lon:number;
  website?:string;
  wikipediaTitle?:string;
  imageUrl?:string;
}
export interface GeocodeResult {
  place_id:number;
  display_name:string;
  lat:string;
  lon:string;
  boundingbox:[string, string, string, string];
}
export interface MediaFolder {
  id:ID; parentId:ID; relativePath:string; name:string;
}
export interface ThumbnailRef { mediaId:ID; index:number }
export interface Entry {
  id:ID; name:string; relativePath:string; type:"folder"|"media";
  media?:Media; folder?:MediaFolder; folderThumbnails?:ThumbnailRef[]; folderThumbnail?:ID;
}
export interface FolderEntries {
  entries: Entry[];
  chain: MediaFolder[];
}
export interface VideoThumbnail { index:number; timeSeconds:number; url:string }
export interface FilesystemDirectory { name:string; path:string }
export interface FilesystemListing { root:string; path:string; parent:string; directories:FilesystemDirectory[] }
export interface EmbyImportedUser { user:User; temporaryPassword?:string; existed:boolean }
export interface EmbyImportResult { users:EmbyImportedUser[]; libraries:Library[]; access:{libraryId:ID; userId:ID}[] }
export interface JobStatus {
  id:string; category:"scan"|"thumbnail-create"|string; type?:string; libraryId:ID; libraryName:string; rootPath:string;
  status:"running"|"paused"|"cancelling"|"cancelled"|"done"|"failed"|string; paused:boolean; cancelable:boolean;
  currentPath:string; processed:number; total:number;
  error:string; startedAt:string; finishedAt?:string;
}
export interface ScheduledTask {
  id:ID; name:string; taskType:"scan"|"thumbnail-create"|"vacuum"; libraryId:ID;
  cron:string; enabled:boolean; lastRunAt?:string; nextRunAt:string;
}
export interface LogTail { path:string; lines:string[] }
export interface About {
  product:string;
  version:string;
  revision:string;
  buildDate:string;
  goVersion:string;
  gatewayEnabled:boolean;
}
