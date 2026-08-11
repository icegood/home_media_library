import { act, cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App, resetFolderEntriesCache } from "./App";

const mockApi = vi.hoisted(() => ({
  setupStatus: vi.fn(),
  me: vi.fn(),
  userSettings: vi.fn(),
  updateUserSettings: vi.fn(),
  updateEmail: vi.fn(),
  changePassword: vi.fn(),
  forgotPassword: vi.fn(),
  resetPassword: vi.fn(),
  libraries: vi.fn(),
  libraryStats: vi.fn(),
  map: vi.fn(),
  settings: vi.fn(),
  updateSettings: vi.fn(),
  createLibrary: vi.fn(),
  updateLibrary: vi.fn(),
  deleteLibrary: vi.fn(),
  logout: vi.fn(),
  scanLibrary: vi.fn(),
  createThumbnails: vi.fn(),
  users: vi.fn(),
  createUser: vi.fn(),
  updateUser: vi.fn(),
  libraryAccess: vi.fn(),
  setLibraryAccess: vi.fn(),
  entries: vi.fn(),
  folder: vi.fn(),
  folderEntries: vi.fn(),
  libraryMedia: vi.fn(),
  favoriteViews: vi.fn(),
  mediaFavoriteViews: vi.fn(),
  createFavoriteView: vi.fn(),
  updateFavoriteView: vi.fn(),
  deleteFavoriteView: vi.fn(),
  favoriteViewMedia: vi.fn(),
  media: vi.fn(),
  updateMediaDetails: vi.fn(),
  favoriteMedia: vi.fn(),
  unfavoriteMedia: vi.fn(),
  thumbnailUrl: vi.fn(),
  contentUrl: vi.fn(),
  folderThumbnailUrl: vi.fn(),
  jobs: vi.fn(),
  logs: vi.fn(),
  clearLogs: vi.fn(),
  logsDownloadUrl: vi.fn(() => "/api/v1/admin/logs/download"),
  pauseJob: vi.fn(),
  resumeJob: vi.fn(),
  cancelJob: vi.fn(),
  cleanupOrphanThumbnails: vi.fn(),
  vacuumDatabase: vi.fn(),
  shutdown: vi.fn(),
  importEmby: vi.fn(),
  filesystem: vi.fn(),
  videoThumbnails: vi.fn(),
  playbackUrl: vi.fn()
}));

vi.mock("./api", () => ({ api: mockApi, MAX_VIDEO_THUMBNAILS: 100 }));

beforeEach(() => {
  vi.resetAllMocks();
  resetFolderEntriesCache();
  (window as any).matchMedia = undefined;
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  document.documentElement.style.fontSize = "";
  mockApi.setupStatus.mockResolvedValue({required:false});
  mockApi.me.mockRejectedValue(new Error("unauthorized"));
  mockApi.userSettings.mockResolvedValue({theme:"light", codec:"h264-aac-mp4", zoom:100});
  mockApi.updateUserSettings.mockResolvedValue({theme:"dark", codec:"h264-aac-mp4", zoom:100});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[
    {id:10, path:"/media/family/photos"}
  ]}]);
  mockApi.libraryStats.mockResolvedValue({folders:3, files:12, images:10, videos:2});
  mockApi.map.mockResolvedValue([]);
  mockApi.settings.mockResolvedValue({
    httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"", httpsGatewayEnabled:true,
    thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:100, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
    sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30,
    smtpHost:"", smtpPort:587, smtpUsername:"", smtpPassword:"", smtpFrom:""
  });
  mockApi.updateSettings.mockResolvedValue({
    httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"", httpsGatewayEnabled:true,
    thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:100, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
    sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30,
    smtpHost:"", smtpPort:587, smtpUsername:"", smtpPassword:"", smtpFrom:""
  });
  mockApi.updateEmail.mockResolvedValue({email:""});
  mockApi.changePassword.mockResolvedValue({ok:true});
  mockApi.forgotPassword.mockResolvedValue({sent:true});
  mockApi.resetPassword.mockResolvedValue({ok:true});
  mockApi.createLibrary.mockResolvedValue({id:1, name:"Family"});
  mockApi.updateLibrary.mockResolvedValue({id:1, name:"Family"});
  mockApi.deleteLibrary.mockResolvedValue(undefined);
  mockApi.logout.mockResolvedValue(undefined);
  mockApi.scanLibrary.mockResolvedValue({id:"job-1", category:"scan", status:"running"});
  mockApi.createThumbnails.mockResolvedValue({id:"job-3", category:"thumbnail-create", status:"running"});
  mockApi.cleanupOrphanThumbnails.mockResolvedValue({id:"job-4", category:"orphan-thumbnail-cleanup", status:"running"});
  mockApi.vacuumDatabase.mockResolvedValue({id:"vacuum-1", category:"vacuum", status:"running"});
  mockApi.users.mockResolvedValue([{id:0, login:"admin", role:"admin"}, {id:2, login:"alice", role:"regular"}]);
  mockApi.createUser.mockResolvedValue({id:3, login:"bob", role:"regular"});
  mockApi.updateUser.mockResolvedValue({id:2, login:"alice", role:"regular"});
  mockApi.libraryAccess.mockResolvedValue([{user:{id:0, login:"admin", role:"admin"}, allowed:true}, {user:{id:2, login:"alice", role:"regular"}, allowed:false}]);
  mockApi.setLibraryAccess.mockResolvedValue(undefined);
  mockApi.entries.mockResolvedValue([]);
  mockApi.folder.mockResolvedValue({id:20, parentId:-1, relativePath:"Photos", name:"Photos"});
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]});
  mockApi.libraryMedia.mockResolvedValue([]);
  mockApi.favoriteViews.mockResolvedValue([]);
  mockApi.createFavoriteView.mockResolvedValue({id:30, name:"Favorites", count:0});
  mockApi.updateFavoriteView.mockResolvedValue({id:30, name:"Favorites", count:0});
  mockApi.deleteFavoriteView.mockResolvedValue(undefined);
  mockApi.favoriteViewMedia.mockResolvedValue([]);
  mockApi.media.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""});
  mockApi.updateMediaDetails.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"renamed.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"50.45,30.52", takenAt:""});
  mockApi.favoriteMedia.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"", favorite:true});
  mockApi.unfavoriteMedia.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"", favorite:false});
  mockApi.thumbnailUrl.mockReturnValue("/thumb.jpg");
  mockApi.contentUrl.mockReturnValue("/content.jpg");
  mockApi.folderThumbnailUrl.mockReturnValue("/folder-thumb.jpg");
  mockApi.jobs.mockResolvedValue([]);
  mockApi.logs.mockResolvedValue({path:"/runtime/app-config/logs/app.log", lines:["2026/07/31 08:00:00 I media API listening on :8080", "2026/07/31 08:01:00 E thumbnail failed"]});
  mockApi.pauseJob.mockResolvedValue({id:"job-1", category:"scan", status:"paused", paused:true, cancelable:true});
  mockApi.resumeJob.mockResolvedValue({id:"job-1", category:"scan", status:"running", paused:false, cancelable:true});
  mockApi.cancelJob.mockResolvedValue({id:"job-1", category:"scan", status:"cancelling", paused:false, cancelable:true});
  mockApi.shutdown.mockResolvedValue({status:"stopping"});
  mockApi.importEmby.mockResolvedValue({libraries:[], users:[], access:[]});
  mockApi.filesystem.mockResolvedValue({root:"/media", path:"/media", parent:"", directories:[
    {name:"photos", path:"/media/photos"}
  ]});
  mockApi.videoThumbnails.mockResolvedValue([]);
  mockApi.playbackUrl.mockReturnValue("/play.mp4");
});

afterEach(() => {
  cleanup();
});

test("shows login for an unauthenticated visitor", async () => {
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("button", {name:"Sign in"})).toBeInTheDocument();
});

test("unknown frontend route shows a 404 fallback", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  render(<MemoryRouter initialEntries={["/missing/page"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Page not found"})).toBeInTheDocument();
  expect(screen.getByText("/missing/page")).toBeInTheDocument();
  expect(screen.getByRole("link", {name:"Open libraries"})).toHaveAttribute("href", "/");
  expect(mockApi.libraries).not.toHaveBeenCalled();
});

test("settings navigation opens thumbnail subsection and shows video thumbnail settings", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Admin panel menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Thumbnails"}));
  expect(await screen.findByRole("heading", {name:"Admin panel"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Thumbnails"})).toBeInTheDocument();
  expect(screen.getByText("Video thumbnails")).toBeInTheDocument();
  expect(screen.getByLabelText("Max thumbnails")).toHaveValue(100);
  expect(screen.getByLabelText("Minimum interval, seconds")).toHaveValue(120);
});

test("settings navigation exposes network mail and timeout subsections", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Admin panel menu"));
  expect(await screen.findByRole("menuitem", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Mail"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Timeouts"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Network"}));
  expect(await screen.findByRole("heading", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByLabelText("Enable HTTP")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Mail"}));
  expect(await screen.findByRole("heading", {name:"Mail"})).toBeInTheDocument();
  expect(screen.getByLabelText("SMTP host")).toHaveValue("");
  fireEvent.click(screen.getByRole("button", {name:"Timeouts"}));
  expect(await screen.findByRole("heading", {name:"Timeouts"})).toBeInTheDocument();
  expect(screen.getByLabelText("Remember login, hours")).toHaveValue(720);
  expect(screen.getByLabelText("Remove inactive jobs after, minutes")).toHaveValue(10);
});

test("all backend settings have visible admin controls", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=network"]}><App/></MemoryRouter>);
  expect(await screen.findByLabelText("Enable HTTP")).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Enable HTTPS with Let’s Encrypt"));
  expect(await screen.findByLabelText("Public DNS name")).toBeInTheDocument();
  expect(screen.getByLabelText("Let’s Encrypt email")).toBeInTheDocument();
  expect(screen.getByLabelText("Certificate expires")).toHaveValue("Not installed yet");

  fireEvent.click(screen.getByRole("button", {name:"Mail"}));
  expect(await screen.findByLabelText("SMTP host")).toBeInTheDocument();
  expect(screen.getByLabelText("SMTP port")).toBeInTheDocument();
  expect(screen.getByLabelText("From address")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Thumbnails"}));
  expect(await screen.findByLabelText("Width")).toBeInTheDocument();
  expect(screen.getByLabelText("Height")).toBeInTheDocument();
  expect(screen.getByLabelText("First thumbnail, seconds")).toBeInTheDocument();
  expect(screen.getByLabelText("Max thumbnails")).toBeInTheDocument();
  expect(screen.getByLabelText("Minimum interval, seconds")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Background jobs"}));
  expect(await screen.findByLabelText("Worker pool size")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Timeouts"}));
  expect(await screen.findByLabelText("Remember login, hours")).toBeInTheDocument();
  expect(screen.getByLabelText("Remove inactive jobs after, minutes")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Logs"}));
  expect(await screen.findByLabelText("Logging level")).toBeInTheDocument();
  expect(screen.getByLabelText("Rotate after, MB")).toBeInTheDocument();
  expect(screen.getByLabelText("Keep rotated files")).toBeInTheDocument();
  expect(screen.getByLabelText("Keep logs, days")).toBeInTheDocument();
});

test("HTTPS controls are hidden when the gateway is not enabled", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.settings.mockResolvedValue({
    httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"", httpsGatewayEnabled:false,
    thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:100, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
    sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30,
    smtpHost:"", smtpPort:587, smtpUsername:"", smtpPassword:"", smtpFrom:""
  });
  render(<MemoryRouter initialEntries={["/admin?section=network"]}><App/></MemoryRouter>);
  expect(await screen.findByLabelText("Enable HTTP")).toBeInTheDocument();
  expect(screen.queryByLabelText("Enable HTTPS with Let’s Encrypt")).not.toBeInTheDocument();
  expect(screen.getByText(/optional gateway container is not enabled/)).toBeInTheDocument();
});

test("settings navigation opens libraries section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Admin panel menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Libraries"}));
  expect(await screen.findByRole("heading", {name:"Admin panel"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByText("Family")).toBeInTheDocument();
});

test("libraries view shows statistics on name click with a calculating state", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  let resolveStats:(value:{folders:number; files:number; images:number; videos:number}) => void = () => {};
  mockApi.libraryStats.mockReturnValueOnce(new Promise(resolve => { resolveStats = resolve; }));
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("button", {name:"Family"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Family"}));
  const dialog = await screen.findByRole("dialog", {name:"Library statistics Family"});
  expect(within(dialog).getByText("Calculating…")).toBeInTheDocument();
  resolveStats({folders:3, files:12, images:10, videos:2});
  expect(await within(dialog).findByText("Folders")).toBeInTheDocument();
  expect(within(dialog).getByText("Files")).toBeInTheDocument();
  expect(within(dialog).getByText("Images")).toBeInTheDocument();
  expect(within(dialog).getByText("Videos")).toBeInTheDocument();
  expect(within(dialog).getByText("12")).toBeInTheDocument();
});

test("library tile links into the library without a per-library map button", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const openLink = await screen.findByRole("link", {name:"Open library Family"});
  expect(openLink).toHaveAttribute("href", "/library/1");
  expect(screen.queryByRole("link", {name:"Map of library Family"})).not.toBeInTheDocument();
  expect(screen.queryByText(/folders ·/)).not.toBeInTheDocument();
});

test("admin library list hides statistics until the library name is clicked", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  await screen.findByRole("heading", {name:"Libraries"});
  expect(await screen.findByText("Family")).toBeInTheDocument();
  expect(screen.queryByText(/folders ·/)).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:/photos/}));
  expect(await screen.findByRole("dialog", {name:"Library statistics Family"})).toBeInTheDocument();
  expect(await within(screen.getByRole("dialog", {name:"Library statistics Family"})).findByText("Folders")).toBeInTheDocument();
});

test("authenticated header renders user menu and clicking logout logs out", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const userMenu = await screen.findByLabelText("User menu");
  expect(userMenu).toHaveTextContent("admin");
  expect(screen.queryByText("User: admin")).not.toBeInTheDocument();
  fireEvent.click(userMenu);
  const logoutButton = await screen.findByRole("menuitem", {name:"Logout"});
  expect(logoutButton).toBeInTheDocument();
  fireEvent.click(logoutButton);
  expect(userMenu.closest("details")).not.toHaveAttribute("open");
  await waitFor(() => expect(mockApi.logout).toHaveBeenCalledTimes(1));
  expect(await screen.findByRole("button", {name:"Sign in"})).toBeInTheDocument();
});

test("admin settings menu duplicates settings sections and opens selected section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const settingsMenu = await screen.findByLabelText("Admin panel menu");
  fireEvent.click(settingsMenu);
  const topMenu = settingsMenu.closest("details")?.querySelector('[role="menu"]');
  if (!(topMenu instanceof HTMLElement)) throw new Error("Settings submenu was not rendered");
  expect(within(topMenu).getByText("Media")).toBeInTheDocument();
  expect(within(topMenu).getByText("System")).toBeInTheDocument();
  expect(within(topMenu).getByText("Import")).toBeInTheDocument();
  expect(await screen.findByRole("menuitem", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Mail"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Timeouts"})).toBeInTheDocument();
  expect(screen.queryByRole("menuitem", {name:"Add library"})).not.toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Background jobs"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Logs"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Emby import"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Background jobs"}));
  expect(screen.getByLabelText("Admin panel menu").closest("details")).not.toHaveAttribute("open");
  expect(await screen.findByRole("heading", {name:"Background jobs"})).toBeInTheDocument();
  const sidebar = screen.getByLabelText("Admin panel sections");
  expect(within(sidebar).getByText("Media")).toBeInTheDocument();
  expect(within(sidebar).getByText("Access")).toBeInTheDocument();
  expect(within(sidebar).getByText("System")).toBeInTheDocument();
  expect(within(sidebar).getByText("Import")).toBeInTheDocument();
  expect(within(sidebar).getAllByRole("button").map(button => button.getAttribute("aria-label"))).toEqual([
    "Libraries",
    "Thumbnails",
    "Users",
    "Network",
    "Mail",
    "Timeouts",
    "Background jobs",
    "Scheduled tasks",
    "Logs",
    "Emby import",
  ]);
});

test("top menus close when another menu or page content is clicked", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const settingsMenu = await screen.findByLabelText("Admin panel menu");
  const userMenu = await screen.findByLabelText("User menu");
  fireEvent.click(settingsMenu);
  expect(settingsMenu.closest("details")).toHaveAttribute("open");
  fireEvent.pointerDown(userMenu);
  fireEvent.click(userMenu);
  expect(settingsMenu.closest("details")).not.toHaveAttribute("open");
  expect(userMenu.closest("details")).toHaveAttribute("open");
  fireEvent.pointerDown(await screen.findByText("Family"));
  expect(userMenu.closest("details")).not.toHaveAttribute("open");
});

test("regular user has no settings menu and picks theme in user settings", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByText("Your libraries")).toBeInTheDocument();
  expect(screen.queryByLabelText("Admin panel menu")).not.toBeInTheDocument();
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Theme"}), {target:{value:"dark"}});
  expect(document.documentElement.dataset.theme).toBe("light");
  expect(mockApi.updateUserSettings).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"dark", codec:"h264-aac-mp4", zoom:100, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains"}));
  await waitFor(() => expect(document.documentElement.dataset.theme).toBe("dark"));
});

test("system theme resolves via prefers-color-scheme and updates on change", async () => {
  let dark = false;
  const listeners:Array<(event:{matches:boolean})=>void> = [];
  const media:any = {
    get matches() { return dark; },
    addEventListener: (_:string, listener:any) => { listeners.push(listener); },
    removeEventListener: () => undefined,
  };
  window.matchMedia = vi.fn().mockReturnValue(media);
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Theme"}), {target:{value:"system"}});
  expect(document.documentElement.dataset.theme).toBe("light");
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"system", codec:"h264-aac-mp4", zoom:100, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains"}));
  dark = true;
  listeners.forEach(listener => listener({matches:true}));
  await waitFor(() => expect(document.documentElement.dataset.theme).toBe("dark"));
  dark = false;
  listeners.forEach(listener => listener({matches:false}));
  await waitFor(() => expect(document.documentElement.dataset.theme).toBe("light"));
});

test("user settings lists the full direct-play formats the browser supports", async () => {
  const originalCanPlayType = HTMLVideoElement.prototype.canPlayType;
  HTMLVideoElement.prototype.canPlayType = (type:string) => type.startsWith("video/mp4") ? "maybe" : "";
  try {
    mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
    render(<MemoryRouter><App/></MemoryRouter>);
    fireEvent.click(await screen.findByLabelText("User menu"));
    fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
    const dialog = await screen.findByRole("dialog", {name:"User settings"});
    expect(await within(dialog).findByText("Your browser plays directly without transcoding: MP4 — H.264 + AAC, MP4 — H.265 / HEVC + AAC.")).toBeInTheDocument();
  } finally {
    HTMLVideoElement.prototype.canPlayType = originalCanPlayType;
  }
});

test("user settings modal changes codec email and password", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular", email:"ice@example.com"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  expect(await within(dialog).findByText(/Your browser plays directly without transcoding/)).toBeInTheDocument();

  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Transcode schema"}), {target:{value:"vp9-opus-webm"}});
  expect(mockApi.updateUserSettings).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"vp9-opus-webm", zoom:100, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains"}));

  fireEvent.change(within(dialog).getByLabelText("Email address"), {target:{value:"new@example.com"}});
  fireEvent.blur(within(dialog).getByLabelText("Email address"));
  await waitFor(() => expect(mockApi.updateEmail).toHaveBeenCalledWith("new@example.com"));
  expect(await within(dialog).findByText("Email saved.")).toBeInTheDocument();

  fireEvent.click(within(dialog).getByRole("button", {name:/Change password/}));
  const passwordDialog = await screen.findByRole("dialog", {name:"Change password"});
  fireEvent.change(within(passwordDialog).getByLabelText("Current password"), {target:{value:"old-password"}});
  fireEvent.change(within(passwordDialog).getByLabelText("New password"), {target:{value:"a-new-password"}});
  fireEvent.change(within(passwordDialog).getByLabelText("Confirm new password"), {target:{value:"a-new-password"}});
  fireEvent.click(within(passwordDialog).getByRole("button", {name:"Update password"}));
  await waitFor(() => expect(mockApi.changePassword).toHaveBeenCalledWith("old-password", "a-new-password"));
  expect(await within(passwordDialog).findByText("Password updated.")).toBeInTheDocument();
});

test("user settings modal changes zoom grade numerically", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  expect(within(dialog).getByRole("combobox", {name:"Zoom"})).toHaveValue("100");
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Zoom"}), {target:{value:"120"}});
  expect(document.documentElement.style.fontSize).toBe("120%");
  expect(mockApi.updateUserSettings).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:120, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains"}));
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Zoom"}), {target:{value:"80"}});
  expect(document.documentElement.style.fontSize).toBe("80%");
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:80, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains"}));
});

test("virtual grid mounts only the visible window of many entries first", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const many = Array.from({length:200}, (_, index) => {
    const media = {
      id:index + 1, folderId:20, relativePath:`img-${index + 1}.jpg`, name:`img-${index + 1}.jpg`,
      kind:"image" as const, mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""
    };
    return {id:media.id, name:media.name, relativePath:media.relativePath, type:"media" as const, media};
  });
  mockApi.entries.mockResolvedValue(many);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await screen.findByText("img-1.jpg");
  const mounted = document.querySelectorAll(".media");
  expect(mounted.length).toBeGreaterThan(0);
  expect(mounted.length).toBeLessThan(many.length);
  expect(screen.queryByText("img-200.jpg")).not.toBeInTheDocument();
});

test("virtual grid shifts the mounted window while scrolling", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const many = Array.from({length:200}, (_, index) => {
    const media = {
      id:index + 1, folderId:20, relativePath:`img-${index + 1}.jpg`, name:`img-${index + 1}.jpg`,
      kind:"image" as const, mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""
    };
    return {id:media.id, name:media.name, relativePath:media.relativePath, type:"media" as const, media};
  });
  mockApi.entries.mockResolvedValue(many);
  const originalScrollY = Object.getOwnPropertyDescriptor(window, "scrollY");
  Object.defineProperty(window, "scrollY", {value:2000, writable:true, configurable:true});
  try {
    render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
    const firstItem = await screen.findByText("img-1.jpg");
    const browser = document.querySelector(".virtual-browser");
    expect(browser).not.toBeNull();
    const rowStep = 292 + 18;
    Object.defineProperty(browser!, "getBoundingClientRect", {value:() => ({
      top: 400 - 2000, left:0, right:1024, bottom:400 - 2000 + rowStep * 6, width:1024, height:rowStep * 6, x:0, y:400 - 2000,
    })});
    fireEvent.scroll(window);
    await waitFor(() => expect(screen.queryByText("img-1.jpg")).not.toBeInTheDocument());
    expect(screen.getByText("img-50.jpg")).toBeInTheDocument();
    expect(screen.queryByText("img-200.jpg")).not.toBeInTheDocument();
    expect(firstItem).not.toBeInTheDocument();
  } finally {
    if (originalScrollY) Object.defineProperty(window, "scrollY", originalScrollY);
    else delete (window as unknown as {scrollY?:number}).scrollY;
  }
});

test("login forgot password flow requests a reset link", async () => {
  mockApi.me.mockRejectedValue(new Error("unauthorized"));
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Forgot password?"}));
  fireEvent.change(screen.getByLabelText("Email"), {target:{value:"ice@example.com"}});
  fireEvent.click(screen.getByRole("button", {name:"Send reset link"}));
  await waitFor(() => expect(mockApi.forgotPassword).toHaveBeenCalledWith("ice@example.com"));
  expect(await screen.findByText("If an account exists for this email, a reset link has been sent.")).toBeInTheDocument();
});

test("reset page sets a new password from the token", async () => {
  render(<MemoryRouter initialEntries={["/reset?token=abcdef123456"]}><App/></MemoryRouter>);
  fireEvent.change(await screen.findByLabelText("New password"), {target:{value:"a-brand-new-password"}});
  fireEvent.change(screen.getByLabelText("Confirm password"), {target:{value:"a-brand-new-password"}});
  fireEvent.click(screen.getByRole("button", {name:"Reset password"}));
  await waitFor(() => expect(mockApi.resetPassword).toHaveBeenCalledWith("abcdef123456", "a-brand-new-password"));
  expect(await screen.findByText("Your password has been reset. You can now sign in.")).toBeInTheDocument();
});

test("admin user submenu does not contain settings sections", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const userMenu = await screen.findByLabelText("User menu");
  fireEvent.click(userMenu);
  const userSubmenu = userMenu.parentElement?.querySelector(".user-submenu");
  expect(userSubmenu).toBeTruthy();
  expect(userSubmenu).not.toHaveTextContent("Libraries");
  expect(userSubmenu).not.toHaveTextContent("Background jobs");
  expect(userSubmenu).not.toHaveTextContent("Logs");
  expect(userSubmenu).not.toHaveTextContent("Theme: light");
  expect(userSubmenu).toHaveTextContent("User settings");
  expect(userSubmenu).toHaveTextContent("Logout");
});

test("library tridot menu exposes library actions and thumbnail refresh options", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Add"})).toBeInTheDocument();
  expect(await screen.findByText("Family")).toBeInTheDocument();
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  expect(await screen.findByRole("heading", {name:"Add library"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Close"}));
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Refresh content"}));
  expect(screen.getByLabelText("Library menu Family").closest("details")).not.toHaveAttribute("open");
  await waitFor(() => expect(mockApi.scanLibrary).toHaveBeenCalledWith(1));
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(screen.getByRole("menuitem", {name:"Refresh thumbnails…"}));
  expect(await screen.findByRole("dialog", {name:"Refresh thumbnails Family"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Missing only"}));
  await waitFor(() => expect(mockApi.createThumbnails).toHaveBeenCalledWith(1, {recreateExisting:false}));
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(screen.getByRole("menuitem", {name:"Refresh thumbnails…"}));
  fireEvent.click(await screen.findByRole("button", {name:"Recreate existing"}));
  await waitFor(() => expect(mockApi.createThumbnails).toHaveBeenCalledWith(1, {recreateExisting:true}));
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  expect(screen.queryByRole("menuitem", {name:"Renew metadata"})).not.toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Rename"}));
  expect(await screen.findByRole("dialog", {name:"Edit library details"})).toBeInTheDocument();
  expect(screen.getByRole("heading", {name:"Edit details"})).toBeInTheDocument();
});

test("library tridot menu can delete a library after confirmation", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValueOnce([{id:1, name:"Family", roots:[
    {id:10, path:"/media/family/photos"}
  ]}]).mockResolvedValueOnce([]);
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Delete"}));
  expect(await screen.findByRole("dialog", {name:"Delete library Family"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Delete library"}));
  await waitFor(() => expect(mockApi.deleteLibrary).toHaveBeenCalledWith(1));
  expect(await screen.findByText(/deleted/i)).toBeInTheDocument();
});

test("settings menu switches between library subsections", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("button", {name:"Libraries"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Background jobs"}));
  expect(await screen.findByRole("heading", {name:"Background jobs"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Logs"}));
  expect(await screen.findByRole("heading", {name:"Logs"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Emby import"}));
  expect(await screen.findByRole("heading", {name:"Emby import"})).toBeInTheDocument();
});

test("emby import uses server directory pickers for config root and target paths", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.filesystem
    .mockResolvedValueOnce({root:"/runtime", path:"/runtime", parent:"", directories:[{name:"emby", path:"/runtime/emby"}]})
    .mockResolvedValueOnce({root:"/media", path:"/media", parent:"", directories:[{name:"photos", path:"/media/photos"}]});
  render(<MemoryRouter initialEntries={["/admin?section=emby"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Emby import"})).toBeInTheDocument();

  fireEvent.click(screen.getAllByRole("button", {name:"Browse"})[0]);
  expect(await screen.findByRole("dialog", {name:"Choose Emby config root"})).toBeInTheDocument();
  fireEvent.click(await screen.findByRole("button", {name:"Select"}));
  expect(screen.getByLabelText("Emby config root inside API container")).toHaveValue("/runtime/emby");

  fireEvent.change(screen.getByLabelText("Emby path"), {target:{value:"/old/photos"}});
  fireEvent.click(screen.getAllByRole("button", {name:"Browse"})[1]);
  expect(await screen.findByRole("dialog", {name:"Choose Docker target folder"})).toBeInTheDocument();
  fireEvent.click(await screen.findByRole("button", {name:"Select"}));
  expect(screen.getByLabelText("Docker path")).toHaveValue("/media/photos");

  fireEvent.click(screen.getByRole("button", {name:"Import from Emby"}));
  await waitFor(() => expect(mockApi.importEmby).toHaveBeenCalledWith({
    configRoot:"/runtime/emby",
    pathMappings:[{from:"/old/photos", to:"/media/photos"}]
  }));
});

test("admin can create users from settings users section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=users"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Users"})).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Login"), {target:{value:"bob"}});
  fireEvent.change(screen.getByLabelText("Password"), {target:{value:"verylongpass1"}});
  fireEvent.click(screen.getByRole("button", {name:"Add user"}));
  await waitFor(() => expect(mockApi.createUser).toHaveBeenCalledWith({login:"bob", role:"regular", password:"verylongpass1"}));
});

test("library edit modal can grant regular user read access", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(screen.getByRole("menuitem", {name:"Rename"}));
  expect(await screen.findByRole("group", {name:"Read access"})).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText(/alice/));
  await waitFor(() => expect(mockApi.setLibraryAccess).toHaveBeenCalledWith(1, 2, true));
});

test("admin can compact the database from the jobs section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
  try {
    render(<MemoryRouter initialEntries={["/admin?section=jobs"]}><App/></MemoryRouter>);
    fireEvent.click(await screen.findByRole("button", {name:"Compact database now"}));
    await waitFor(() => expect(mockApi.vacuumDatabase).toHaveBeenCalledTimes(1));
    expect(await screen.findByText("Vacuum started in the background. Track it in the job list below.")).toBeInTheDocument();
  } finally {
    confirmSpy.mockRestore();
  }
});

test("vacuum job appears under its category without controls and without a root path label", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.jobs.mockResolvedValue([{id:"vacuum-1", category:"vacuum", status:"running", cancelable:false, processed:1, total:1, libraryName:"Database maintenance"}]);
  render(<MemoryRouter initialEntries={["/admin?section=jobs"]}><App/></MemoryRouter>);
  expect(await screen.findByText("Vacuum")).toBeInTheDocument();
  expect(screen.getAllByText("Database maintenance").length).toBeGreaterThan(0);
  expect(screen.getByText("1/1")).toBeInTheDocument();
  expect(screen.queryByText(/no root/)).not.toBeInTheDocument();
  expect(screen.queryByRole("button", {name:"Pause"})).not.toBeInTheDocument();
  expect(screen.queryByRole("button", {name:"Cancel"})).not.toBeInTheDocument();
});

test("admin can request docker or process server stop from top settings action", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  const {unmount} = render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Stop Docker container"}));
  await waitFor(() => expect(mockApi.shutdown).toHaveBeenCalledWith("docker"));
  expect(screen.getByText("Server lifecycle")).toBeInTheDocument();
  unmount();
  mockApi.shutdown.mockClear();
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Stop server process"}));
  await waitFor(() => expect(mockApi.shutdown).toHaveBeenCalledWith("signal"));
  confirm.mockRestore();
});

test("admin logs section renders recent log lines and can refresh", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=logs"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Logs"})).toBeInTheDocument();
  expect(await screen.findByText(/media API listening/)).toBeInTheDocument();
  expect(screen.getByText(/thumbnail failed/)).toBeInTheDocument();
  expect(screen.getByText(/app.log/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Refresh"}));
  await waitFor(() => expect(mockApi.logs).toHaveBeenCalledTimes(2));
});

test("admin can clear the application log file from the logs section", async () => {
  const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.clearLogs.mockResolvedValue({path:"/runtime/app-config/logs/app.log"});
    render(<MemoryRouter initialEntries={["/admin?section=logs"]}><App/></MemoryRouter>);
    expect(await screen.findByText(/media API listening/)).toBeInTheDocument();
    fireEvent.click(screen.getByRole("button", {name:"Clear logs"}));
    await waitFor(() => expect(mockApi.clearLogs).toHaveBeenCalledTimes(1));
    expect(await screen.findByText("Logs cleared.")).toBeInTheDocument();
    expect(screen.queryByText(/media API listening/)).not.toBeInTheDocument();
    expect(screen.getByText("No log lines yet.")).toBeInTheDocument();
  } finally {
    confirmSpy.mockRestore();
  }
});

test("admin can download the full application log file from the logs section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=logs"]}><App/></MemoryRouter>);
  expect(await screen.findByText(/media API listening/)).toBeInTheDocument();
  const clickSpy = vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(() => {});
  try {
    fireEvent.click(screen.getByRole("button", {name:"Download logs"}));
    await waitFor(() => expect(clickSpy).toHaveBeenCalledTimes(1));
  } finally {
    clickSpy.mockRestore();
  }
});

test("library root can be selected from docker filesystem picker", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  fireEvent.click(await screen.findByRole("button", {name:"Browse"}));
  expect(await screen.findByRole("dialog", {name:"Choose root folder"})).toBeInTheDocument();
  fireEvent.click(await screen.findByRole("button", {name:"Select"}));
  expect(screen.getByLabelText("Root path")).toHaveValue("/media/photos");
});

test("modal windows close when clicking outside but not inside", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  const dialog = await screen.findByRole("dialog", {name:"Add library"});
  fireEvent.click(dialog.querySelector(".card")!);
  expect(screen.getByRole("dialog", {name:"Add library"})).toBeInTheDocument();
  fireEvent.click(dialog);
  expect(screen.queryByRole("dialog", {name:"Add library"})).not.toBeInTheDocument();
});

test("confirmation modals do not close when clicking outside", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Delete"}));
  const dialog = await screen.findByRole("dialog", {name:"Delete library Family"});
  fireEvent.click(dialog);
  expect(screen.getByRole("dialog", {name:"Delete library Family"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Cancel"}));
  expect(screen.queryByRole("dialog", {name:"Delete library Family"})).not.toBeInTheDocument();
});

test("library browser renders folder entries", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.entries.mockResolvedValue([{id:20, name:"Photos", relativePath:"Photos", type:"folder", folderThumbnail:24}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await screen.findByRole("button", {name:"Open folder Photos"});
  expect(screen.getByLabelText("Breadcrumb")).toHaveTextContent("Libraries / Family");
  expect(screen.getByLabelText("Breadcrumb")).not.toHaveTextContent("Root");
  expect(await screen.findByRole("button", {name:"Open folder Photos"})).toBeInTheDocument();
  fireEvent.click(await screen.findByLabelText("Folder menu Photos"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Refresh items"}));
  await waitFor(() => expect(mockApi.scanLibrary).toHaveBeenCalledWith(1));
  fireEvent.click(await screen.findByLabelText("Folder menu Photos"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Refresh thumbnails…"}));
  expect(await screen.findByRole("dialog", {name:"Refresh thumbnails Photos"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Missing only"}));
  await waitFor(() => expect(mockApi.createThumbnails).toHaveBeenCalledWith(1, {recreateExisting:false}));
  mockApi.folderEntries.mockResolvedValueOnce({entries:[
    {id:23, name:"Nested", relativePath:"Photos/Nested", type:"folder"},
    {id:101, name:"one.jpg", relativePath:"Photos/one.jpg", type:"media", media:{id:101, folderId:20, relativePath:"Photos/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:103, name:"two.mp4", relativePath:"Photos/two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"Photos/two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  fireEvent.click(screen.getAllByRole("button", {name:"Photos"})[0]);
  const dialog = await screen.findByRole("dialog", {name:"Folder statistics Photos"});
  expect(dialog).toBeInTheDocument();
  expect(within(dialog).getByText("Folders")).toBeInTheDocument();
  expect(within(dialog).getByText("Files")).toBeInTheDocument();
});

test("favorite view renders media and star removes item from that view", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:1}]);
  mockApi.favoriteViewMedia.mockResolvedValue([{id:100, folderId:20, relativePath:"2025/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"", favorite:true}]);
  render(<MemoryRouter initialEntries={["/favorites/30"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Best"})).toBeInTheDocument();
  expect(await screen.findByText("one.jpg")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Remove one.jpg from this favorite view"}));
  await waitFor(() => expect(mockApi.unfavoriteMedia).toHaveBeenCalledWith(30, 100));
  expect(screen.queryByText("one.jpg")).not.toBeInTheDocument();
});

test("library media card shows favorite views with checkboxes and toggles membership", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.mediaFavoriteViews.mockResolvedValue([
    {id:30, name:"Best", count:2, contains:true},
    {id:31, name:"Travel", count:0, contains:false}
  ]);
  mockApi.entries.mockResolvedValue([{id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Manage favorite views for one.jpg"}));
  expect(await screen.findByRole("dialog", {name:"Favorite views for one.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("checkbox", {name:/Best/})).toBeChecked();
  expect(screen.getByRole("checkbox", {name:/Travel/})).not.toBeChecked();
  fireEvent.click(screen.getByRole("checkbox", {name:/Travel/}));
  await waitFor(() => expect(mockApi.favoriteMedia).toHaveBeenCalledWith(31, 100));
});

test("favorite add dialog can create a new favorite view first", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.mediaFavoriteViews.mockResolvedValue([]);
  mockApi.createFavoriteView.mockResolvedValue({id:44, name:"New one", count:0});
  mockApi.entries.mockResolvedValue([{id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Manage favorite views for one.jpg"}));
  fireEvent.change(await screen.findByLabelText("New favorite view"), {target:{value:"New one"}});
  fireEvent.click(screen.getByRole("button", {name:"Create and add"}));
  await waitFor(() => expect(mockApi.createFavoriteView).toHaveBeenCalledWith("New one"));
  await waitFor(() => expect(mockApi.favoriteMedia).toHaveBeenCalledWith(44, 100));
});

test("selected media items can receive the same gps in bulk", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.entries.mockResolvedValue([
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}}
  ]);
  mockApi.updateMediaDetails.mockImplementation(async (id:number|string, input:{name:string; gps:string|null; takenAt:string|null}) => ({
    id:Number(id), folderId:20, relativePath:Number(id) === 100 ? "one.jpg" : "two.jpg", name:input.name,
    kind:"image", mimeType:"image/jpeg", size:Number(id) === 100 ? 10 : 20, metadata:{}, gps:input.gps ?? "", takenAt:input.takenAt ?? ""
  }));
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Select one.jpg"));
  fireEvent.click(await screen.findByLabelText("Select two.jpg"));
  fireEvent.change(screen.getByLabelText("GPS for selected"), {target:{value:"50.45,30.52"}});
  fireEvent.click(screen.getByRole("button", {name:"Apply GPS"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledTimes(2));
  expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"one.jpg", gps:"50.45,30.52", takenAt:null});
  expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(101, {name:"two.jpg", gps:"50.45,30.52", takenAt:null});
});

test("library timeline groups media by date along a vertical ruler", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"2020/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"},
    {id:102, folderId:20, relativePath:"2020/two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"", takenAt:"2020-08-21T13:34:00Z"},
    {id:101, folderId:21, relativePath:"old.jpg", name:"old.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/timeline"]}><App/></MemoryRouter>);
  await screen.findByText("two.mp4");
  const cards = Array.from(document.querySelectorAll("article.card.media"));
  expect(cards.map(card => card.textContent)).toEqual([
    expect.stringContaining("two.mp4"),
    expect.stringContaining("one.jpg"),
    expect.stringContaining("old.jpg")
  ]);
  expect(screen.queryByRole("heading", {name:"2020-08-21"})).not.toBeInTheDocument();
  expect(screen.queryByRole("heading", {name:"Unknown date"})).not.toBeInTheDocument();
  const groups = Array.from(document.querySelectorAll(".timeline-grid .timeline-group"));
  expect(groups.length).toBe(2);
  expect(groups[0].querySelectorAll(".timeline-group-grid .media").length).toBe(2);
  expect(groups[1].querySelectorAll(".timeline-group-grid .media").length).toBe(1);
  const dateLabels = Array.from(document.querySelectorAll(".timeline-group .timeline-group-date"));
  expect(dateLabels.map(label => label.textContent)).toEqual([
    expect.stringContaining("2020"),
    "Unknown date"
  ]);
  fireEvent.click(screen.getByRole("button", {name:"Images"}));
  expect(screen.getByText("one.jpg")).toBeInTheDocument();
  expect(screen.queryByText("two.mp4")).not.toBeInTheDocument();
  fireEvent.click(screen.getByText("one.jpg"));
  expect(mockApi.entries).not.toHaveBeenCalledWith(1, "2020");
});

test("long folder names are allowed to wrap inside the entry", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const longName = "this_is_a_very_long_folder_name_without_spaces_that_should_wrap_inside_the_tile";
  mockApi.entries.mockResolvedValue([{id:22, name:longName, relativePath:longName, type:"folder", folderThumbnail:24}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  const title = await screen.findByRole("button", {name:longName});
  expect(title).toHaveClass("folder-title-button");
});

test("media name and gps are saved only after explicit save", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  const name = await screen.findByLabelText("Name");
  fireEvent.change(name, {target:{value:"renamed.jpg"}});
  fireEvent.change(screen.getByLabelText("GPS"), {target:{value:"50.45,30.52"}});
  expect(mockApi.updateMediaDetails).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"renamed.jpg", gps:"50.45,30.52", takenAt:""}));
  expect(await screen.findByText("Item saved.")).toBeInTheDocument();
  expect(screen.getByRole("link", {name:"GPS: 50.45,30.52"})).toHaveAttribute("href", "/map?item=100");
  fireEvent.click(screen.getByRole("button", {name:"Hide info panel"}));
  expect(screen.getByRole("button", {name:"Show info panel"})).toHaveTextContent("<<");
});

test("clearing gps in the info panel sends an empty string so the backend can NULL it", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"50.45,30.52", takenAt:""}}
  ], chain: []});
  mockApi.updateMediaDetails.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  const gps = await screen.findByLabelText("GPS");
  expect(gps).toHaveValue("50.45,30.52");
  fireEvent.change(gps, {target:{value:""}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"one.jpg", gps:"", takenAt:""}));
  expect(await screen.findByText("Item saved.")).toBeInTheDocument();
});

test("media info shows full stored metadata as JSON under root nodes", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.folderEntries.mockResolvedValue({entries:[{id:100, name:"DSC_5743.jpg", relativePath:"DSC_5743.jpg", type:"media", media:{id:100, folderId:20, relativePath:"DSC_5743.jpg", name:"DSC_5743.jpg", kind:"image", mimeType:"image/jpeg", size:10, gps:"", takenAt:"2010-08-23T17:54:08Z", metadata:{exif:{Make:"NIKON CORPORATION", Model:"NIKON D50", DateTimeOriginal:"2010:08:23 17:54:08", FNumber:3.5, ExposureTime:0.03333333333, ISO:"0 1600", FocalLength:18, HistoryParams:"darktable-noise", BlueTRC:"(Binary data)", FileModifyDate:"2026:07:29 06:12:16+00:00", MIMEType:"image/jpeg"}, ffprobe:{streams:[{codec_type:"video", codec_name:"mjpeg"}]}}}}], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Show info panel"));
  expect(await screen.findByText("Metadata")).toBeInTheDocument();
  expect(screen.getByRole("heading", {name:"exif"})).toBeInTheDocument();
  expect(screen.getByRole("heading", {name:"ffprobe"})).toBeInTheDocument();
  expect(screen.getByText(/"Make": "NIKON CORPORATION"/)).toBeInTheDocument();
  expect(screen.getByText(/"HistoryParams": "darktable-noise"/)).toBeInTheDocument();
  expect(screen.getByText(new RegExp('"BlueTRC": "\\(Binary data\\)"'))).toBeInTheDocument();
  expect(screen.getByText(new RegExp('"FileModifyDate": "2026:07:29 06:12:16\\+00:00"'))).toBeInTheDocument();
  expect(screen.getByText(new RegExp('"MIMEType": "image/jpeg"'))).toBeInTheDocument();
  expect(screen.getByText(/"codec_name": "mjpeg"/)).toBeInTheDocument();
});

test("media viewer uses side arrows and keyboard shortcuts for previous and next", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: [{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("link", {name:"Photos"})).toHaveAttribute("href", "/library/1/folder/20");
  expect(screen.getByRole("button", {name:"Show main menu"})).toHaveTextContent("vv");
  fireEvent.click(screen.getByRole("button", {name:"Show main menu"}));
  expect(screen.getByRole("button", {name:"Hide main menu"})).toHaveTextContent("^^");
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Previous media"})).toHaveTextContent("<");
  expect(screen.getByRole("button", {name:"Previous media"})).toBeDisabled();
  expect(screen.getByRole("button", {name:"Next media"})).toHaveTextContent(">");
  expect(screen.getByRole("button", {name:"Zoom out"})).not.toBeDisabled();
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("100%");
  fireEvent.click(screen.getByRole("button", {name:"Zoom in"}));
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("125%");
  const image = screen.getByRole("img", {name:"one.jpg"});
  expect(image).toHaveClass("zoomed-image");
  fireEvent.click(screen.getByRole("button", {name:"Reset zoom"}));
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("100%");
  expect(image).toHaveStyle({transform:"translate(0px, 0px) scale(1)"});
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  expect(await screen.findByRole("img", {name:"two.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("100%");
  fireEvent.keyDown(window, {key:"ArrowLeft"});
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
  fireEvent.keyDown(window, {key:"ArrowRight"});
  expect(await screen.findByRole("img", {name:"two.jpg"})).toBeInTheDocument();
});

test("media viewer does not refetch items already present in folder entries", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  await screen.findByRole("img", {name:"one.jpg"});
  mockApi.media.mockClear();
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  await screen.findByRole("img", {name:"two.jpg"});
  expect(mockApi.media).not.toHaveBeenCalled();
});

test("media viewer fetches an item that is not in folder entries", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=999"]}><App/></MemoryRouter>);
  await waitFor(() => expect(mockApi.media).toHaveBeenCalledWith(999));
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
});

test("media viewer up link goes to containing folder", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[{id:100, name:"one.jpg", relativePath:"Trips/Day1/one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"Trips/Day1/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}], chain:[{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("link", {name:"Photos"}));
  await waitFor(() => expect(mockApi.folderEntries).toHaveBeenCalledWith(1, 20));
});

test("folder breadcrumb links go through the folder chain to the library root", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"},
    {id:21, parentId:20, relativePath:"Photos/Nested", name:"Nested"}
  ]});
  render(<MemoryRouter initialEntries={["/library/1/folder/21"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByText("Nested")).toBeInTheDocument());
  expect(screen.getByRole("link", {name:"Photos"})).toHaveAttribute("href", "/library/1/folder/20");
  expect(screen.getByRole("link", {name:"Family"})).toHaveAttribute("href", "/library/1");
  expect(screen.getByRole("link", {name:"Libraries"})).toHaveAttribute("href", "/");
});

test("breadcrumb uses the real folder name even when relativePath is empty", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[{id:7, parentId:-1, relativePath:"", name:"Trip Photos"}]});
  render(<MemoryRouter initialEntries={["/library/1/folder/7"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByText("Trip Photos")).toBeInTheDocument());
  expect(screen.queryByText("Folder 7")).not.toBeInTheDocument();
});

test("browser map button scopes to the library or current folder", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.entries.mockResolvedValue([]);
  const first = render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await waitFor(() => expect(first.getAllByRole("link", {name:"Map"}).some(link => link.getAttribute("href") === "/map?library=1")).toBe(true));
  first.unmount();
  mockApi.folderEntries.mockResolvedValue({entries:[], chain: []});
  const second = render(<MemoryRouter initialEntries={["/library/1/folder/20"]}><App/></MemoryRouter>);
  await waitFor(() => expect(second.getAllByRole("link", {name:"Map"}).some(link => link.getAttribute("href") === "/map?library=1&folder=20")).toBe(true));
});

test("media viewer without item query redirects to library root", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.entries.mockResolvedValue([]);
  render(<MemoryRouter initialEntries={["/library/1/view/20"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByLabelText("Breadcrumb")).toHaveTextContent("Libraries / Family"));
  await waitFor(() => expect(mockApi.entries).toHaveBeenCalledWith(1));
});

test("media info remounts metadata when navigating between neighboring files", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"DSC06360.JPG", relativePath:"DSC06360.JPG", type:"media", media:{id:100, folderId:20, relativePath:"DSC06360.JPG", name:"DSC06360.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{exif:{FileName:"DSC06360.JPG"}}, gps:"", takenAt:""}},
    {id:101, name:"DSC06361.JPG", relativePath:"DSC06361.JPG", type:"media", media:{id:101, folderId:20, relativePath:"DSC06361.JPG", name:"DSC06361.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{exif:{FileName:"DSC06361.JPG"}}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Show info panel"));
  expect(await screen.findByRole("heading", {name:"DSC06360.JPG"})).toBeInTheDocument();
  expect(screen.getByText(/"FileName": "DSC06360.JPG"/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  expect(await screen.findByRole("heading", {name:"DSC06361.JPG"})).toBeInTheDocument();
  expect(screen.queryByText(/"FileName": "DSC06360.JPG"/)).not.toBeInTheDocument();
  expect(screen.getByText(/"FileName": "DSC06361.JPG"/)).toBeInTheDocument();
});

test("old admin settings route redirects to the admin panel", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin/settings"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("heading", {name:"Admin panel"})).toBeInTheDocument());
});

test("video player shows the real duration from ffprobe metadata for transcoded streams", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"Part1.avi", relativePath:"Part1.avi", type:"media", media:{id:100, folderId:20, relativePath:"Part1.avi", name:"Part1.avi", kind:"video", mimeType:"video/x-msvideo", size:713616156, metadata:{ffprobe:{format:{duration:"4033.88"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([
    {index:0, timeSeconds:1, url:"/thumb0.jpg"},
    {index:1, timeSeconds:404, url:"/thumb1.jpg"},
    {index:2, timeSeconds:808, url:"/thumb2.jpg"}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  const slider = await screen.findByLabelText("Seek video");
  expect(slider).toHaveAttribute("max", "4033.88");
  expect(screen.getByText("67:13")).toBeInTheDocument();
  expect(mockApi.playbackUrl).toHaveBeenCalledWith(100, expect.anything(), 0);
  expect(screen.getByRole("button", {name:/Transcoded/})).toBeInTheDocument();
});

test("seeking a transcoded video requests a server-side start offset instead of relying on browser seeking", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"Part1.avi", relativePath:"Part1.avi", type:"media", media:{id:100, folderId:20, relativePath:"Part1.avi", name:"Part1.avi", kind:"video", mimeType:"video/x-msvideo", size:713616156, metadata:{ffprobe:{format:{duration:"4033.88"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  const slider = await screen.findByLabelText("Seek video");
  fireEvent.change(slider, {target:{value:"1234"}});
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 500)); });
  expect(mockApi.playbackUrl).toHaveBeenLastCalledWith(100, expect.anything(), 1234);
});

test("transcoded video keeps showing the absolute position after a server-side seek", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"Part1.avi", relativePath:"Part1.avi", type:"media", media:{id:100, folderId:20, relativePath:"Part1.avi", name:"Part1.avi", kind:"video", mimeType:"video/x-msvideo", size:713616156, metadata:{ffprobe:{format:{duration:"4033.88"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  const slider = await screen.findByLabelText("Seek video");
  fireEvent.change(slider, {target:{value:"1234"}});
  await act(async () => { await new Promise(resolve => setTimeout(resolve, 500)); });
  const preload = document.querySelectorAll(".video-stack video")[1] as HTMLVideoElement;
  fireEvent.canPlay(preload);
  fireEvent.timeUpdate(preload, {target:{currentTime:5}});
  expect(slider).toHaveValue("1239");
  expect(screen.getByText("20:39")).toBeInTheDocument();
});

test("transcode badge dropdown explains which parts are incompatible", async () => {
  const originalCanPlayType = HTMLVideoElement.prototype.canPlayType;
  HTMLVideoElement.prototype.canPlayType = (type:string) => type.startsWith("video/webm") ? "maybe" : "";
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.folderEntries.mockResolvedValue({entries:[
      {id:100, name:"clip.mkv", relativePath:"clip.mkv", type:"media", media:{id:100, folderId:20, relativePath:"clip.mkv", name:"clip.mkv", kind:"video", mimeType:"video/x-matroska", size:20, metadata:{ffprobe:{format:{duration:"12.34"}, streams:[{codec_type:"video", codec_name:"vp9"},{codec_type:"audio", codec_name:"mp3"}]}}, gps:"", takenAt:""}}
    ], chain: []});
    mockApi.videoThumbnails.mockResolvedValue([]);
    render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
    const badge = await screen.findByRole("button", {name:/Transcoded/});
    expect(screen.queryByText("Why transcoded?")).not.toBeInTheDocument();
    fireEvent.click(badge);
    expect(screen.getByText("Why transcoded?")).toBeInTheDocument();
    expect(screen.queryByText(/Video codec/)).not.toBeInTheDocument();
    expect(screen.getByText(/Audio track is MP3, but VP9 video needs Opus or Vorbis audio to play without transcoding\./)).toBeInTheDocument();
    expect(screen.getByText(/File container is video\/x-matroska, but VP9 video needs a WebM container\./)).toBeInTheDocument();
    fireEvent.click(badge);
    expect(screen.queryByText("Why transcoded?")).not.toBeInTheDocument();
  } finally {
    HTMLVideoElement.prototype.canPlayType = originalCanPlayType;
  }
});

test("video player shows no transcode badge when the video can be direct-played", async () => {
  const originalCanPlayType = HTMLVideoElement.prototype.canPlayType;
  HTMLVideoElement.prototype.canPlayType = () => "maybe";
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.folderEntries.mockResolvedValue({entries:[
      {id:100, name:"clip.mp4", relativePath:"clip.mp4", type:"media", media:{id:100, folderId:20, relativePath:"clip.mp4", name:"clip.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"12.34"}, streams:[{codec_type:"video", codec_name:"h264"},{codec_type:"audio", codec_name:"aac"}]}}, gps:"", takenAt:""}}
    ], chain: []});
    mockApi.videoThumbnails.mockResolvedValue([]);
    render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
    await screen.findByLabelText("Seek video");
    expect(screen.queryByRole("button", {name:/Transcoded/})).not.toBeInTheDocument();
  } finally {
    HTMLVideoElement.prototype.canPlayType = originalCanPlayType;
  }
});

test("map area selection fetches media inside the rectangle from the server and shows a right-side timeline panel", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const all = [
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-08-21T12:34:00Z"},
    {id:101, libraryId:1, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""},
    {id:102, libraryId:1, folderId:20, relativePath:"c.jpg", name:"c.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ];
  mockApi.map.mockImplementation(async (_libraryId, _folderId, bounds) => {
    if (bounds) return [all[0]];
    return all;
  });
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  const button = await screen.findByRole("button", {name:"Select area"});
  fireEvent.click(button);
  const container = document.querySelector(".leaflet-container");
  expect(container).not.toBeNull();
  fireEvent.mouseDown(container!, {clientX:-200, clientY:-200});
  fireEvent.mouseMove(container!, {clientX:200, clientY:200});
  fireEvent.mouseUp(container!, {clientX:200, clientY:200});
  expect(await screen.findByRole("complementary", {name:"Selected area"})).toBeInTheDocument();
  expect(mockApi.map).toHaveBeenCalledWith(undefined, undefined, expect.objectContaining({
    west: expect.any(Number), south: expect.any(Number), east: expect.any(Number), north: expect.any(Number)
  }));
  expect(screen.getByText("a.jpg")).toBeInTheDocument();
  expect(screen.queryByText("b.jpg")).not.toBeInTheDocument();
  expect(screen.queryByText("c.jpg")).not.toBeInTheDocument();
  expect(document.querySelector(".map-timeline-panel .timeline-grid")).not.toBeNull();
  fireEvent.click(screen.getByRole("button", {name:"Clear"}));
  expect(screen.queryByRole("complementary", {name:"Selected area"})).not.toBeInTheDocument();
});

test("map renders many markers progressively instead of all at once", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const many = Array.from({length:300}, (_, i) => ({
    id:1000+i, libraryId:1, folderId:20, relativePath:`m${i}.jpg`, name:`m${i}.jpg`, kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:`${i*5},${i*7}`, takenAt:"2020-08-21T12:34:00Z"
  }));
  mockApi.map.mockResolvedValue(many);
  vi.useFakeTimers();
  try {
    render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
    await act(async () => { await Promise.resolve(); });
    expect(document.querySelectorAll(".media-point-marker").length).toBe(200);
    expect(screen.getByRole("status")).toHaveTextContent("Rendering markers…");
    await act(async () => { vi.advanceTimersByTime(16); });
    expect(document.querySelectorAll(".media-point-marker").length).toBe(many.length);
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
  } finally {
    vi.useRealTimers();
  }
});

test("clicking a map cluster opens the same timeline panel instead of a popup", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.map.mockResolvedValue([
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-08-21T12:34:00Z"},
    {id:101, libraryId:1, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-08-21T13:00:00Z"}
  ]);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await screen.findByText("2");
  await waitFor(() => {
    const cluster = document.querySelector(".cluster-marker");
    if (!cluster) throw new Error("cluster marker not rendered");
    fireEvent.click(cluster);
    expect(screen.getByRole("complementary", {name:"Selected area"})).toBeInTheDocument();
  });
  expect(screen.getByText("a.jpg")).toBeInTheDocument();
  expect(screen.getByText("b.jpg")).toBeInTheDocument();
  expect(document.querySelectorAll(".map-timeline-panel .timeline-group").length).toBe(1);
  expect(document.querySelectorAll(".map-timeline-panel .timeline-group-date").length).toBe(1);
  fireEvent.click(screen.getByRole("button", {name:"Clear"}));
  expect(screen.queryByRole("complementary", {name:"Selected area"})).not.toBeInTheDocument();
});
