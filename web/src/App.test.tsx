import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeEach, expect, test, vi } from "vitest";
import { App } from "./App";

const mockApi = vi.hoisted(() => ({
  setupStatus: vi.fn(),
  me: vi.fn(),
  userSettings: vi.fn(),
  updateUserSettings: vi.fn(),
  libraries: vi.fn(),
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
  pauseJob: vi.fn(),
  resumeJob: vi.fn(),
  cancelJob: vi.fn(),
  cleanupOrphanThumbnails: vi.fn(),
  shutdown: vi.fn(),
  importEmby: vi.fn(),
  filesystem: vi.fn()
}));

vi.mock("./api", () => ({ api: mockApi }));

beforeEach(() => {
  vi.resetAllMocks();
  localStorage.clear();
  delete document.documentElement.dataset.theme;
  mockApi.setupStatus.mockResolvedValue({required:false});
  mockApi.me.mockRejectedValue(new Error("unauthorized"));
  mockApi.userSettings.mockResolvedValue({theme:"light"});
  mockApi.updateUserSettings.mockResolvedValue({theme:"dark"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", stats:{folders:3, files:12, images:10, videos:2}, roots:[
    {id:10, path:"/media/family/photos"}
  ]}]);
  mockApi.map.mockResolvedValue([]);
  mockApi.settings.mockResolvedValue({
    transcodeCodec:"h264", httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"",
    thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:10, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
    sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30
  });
  mockApi.updateSettings.mockResolvedValue({
    transcodeCodec:"h264", httpEnabled:true, httpsEnabled:false, publicDns:"", acmeEmail:"", httpsCertificateExpiresAt:"",
    thumbnailWidth:480, thumbnailHeight:360, videoThumbnailFirstSeconds:5, videoThumbnailMaxCount:10, videoThumbnailMinIntervalSeconds:120, workerPoolSize:4,
    sessionMaxAgeHours:720, finishedJobRetentionMinutes:10, logLevel:"I", logRotateMaxSizeMB:10, logRotateMaxBackups:5, logRotateMaxAgeDays:30
  });
  mockApi.createLibrary.mockResolvedValue({id:1, name:"Family"});
  mockApi.updateLibrary.mockResolvedValue({id:1, name:"Family"});
  mockApi.deleteLibrary.mockResolvedValue(undefined);
  mockApi.logout.mockResolvedValue(undefined);
  mockApi.scanLibrary.mockResolvedValue({id:"job-1", category:"scan", status:"running"});
  mockApi.createThumbnails.mockResolvedValue({id:"job-3", category:"thumbnail-create", status:"running"});
  mockApi.cleanupOrphanThumbnails.mockResolvedValue({id:"job-4", category:"orphan-thumbnail-cleanup", status:"running"});
  mockApi.users.mockResolvedValue([{id:0, login:"admin", role:"admin"}, {id:2, login:"alice", role:"regular"}]);
  mockApi.createUser.mockResolvedValue({id:3, login:"bob", role:"regular"});
  mockApi.updateUser.mockResolvedValue({id:2, login:"alice", role:"regular"});
  mockApi.libraryAccess.mockResolvedValue([{user:{id:0, login:"admin", role:"admin"}, allowed:true}, {user:{id:2, login:"alice", role:"regular"}, allowed:false}]);
  mockApi.setLibraryAccess.mockResolvedValue(undefined);
  mockApi.entries.mockResolvedValue([]);
  mockApi.folder.mockResolvedValue({id:20, parentId:-1, relativePath:"Photos"});
  mockApi.folderEntries.mockResolvedValue([]);
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
  fireEvent.click(await screen.findByLabelText("Settings menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Thumbnails"}));
  expect(await screen.findByRole("heading", {name:"Settings"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Thumbnails"})).toBeInTheDocument();
  expect(screen.getByText("Video thumbnails")).toBeInTheDocument();
  expect(screen.getByLabelText("Max thumbnails")).toHaveValue(10);
  expect(screen.getByLabelText("Minimum interval, seconds")).toHaveValue(120);
});

test("settings navigation exposes network encoding and timeout subsections", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Settings menu"));
  expect(await screen.findByRole("menuitem", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Encoding"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Timeouts"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Network"}));
  expect(await screen.findByRole("heading", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByLabelText("Enable HTTP")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Encoding"}));
  expect(await screen.findByRole("heading", {name:"Encoding"})).toBeInTheDocument();
  expect(screen.getByLabelText("Fallback video codec")).toHaveValue("h264");
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

  fireEvent.click(screen.getByRole("button", {name:"Encoding"}));
  expect(await screen.findByLabelText("Fallback video codec")).toBeInTheDocument();

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

test("settings navigation opens libraries section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Settings menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Libraries"}));
  expect(await screen.findByRole("heading", {name:"Settings"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByText("Family")).toBeInTheDocument();
});

test("libraries view permanently shows library statistics", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByText("Family")).toBeInTheDocument();
  expect(screen.getByText("Folders")).toBeInTheDocument();
  expect(screen.getByText("Files")).toBeInTheDocument();
  expect(screen.getByText("Images")).toBeInTheDocument();
  expect(screen.getByText("Videos")).toBeInTheDocument();
  expect(screen.getByText("12")).toBeInTheDocument();
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
  const settingsMenu = await screen.findByLabelText("Settings menu");
  fireEvent.click(settingsMenu);
  const topMenu = settingsMenu.closest("details")?.querySelector('[role="menu"]');
  if (!(topMenu instanceof HTMLElement)) throw new Error("Settings submenu was not rendered");
  expect(within(topMenu).getByText("Media")).toBeInTheDocument();
  expect(within(topMenu).getByText("System")).toBeInTheDocument();
  expect(within(topMenu).getByText("Import")).toBeInTheDocument();
  expect(await screen.findByRole("menuitem", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Encoding"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Timeouts"})).toBeInTheDocument();
  expect(screen.queryByRole("menuitem", {name:"Add library"})).not.toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Background jobs"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Logs"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Emby import"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Theme: light"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Background jobs"}));
  expect(screen.getByLabelText("Settings menu").closest("details")).not.toHaveAttribute("open");
  expect(await screen.findByRole("heading", {name:"Background jobs"})).toBeInTheDocument();
  const sidebar = screen.getByLabelText("Settings sections");
  expect(within(sidebar).getByText("Media")).toBeInTheDocument();
  expect(within(sidebar).getByText("Access")).toBeInTheDocument();
  expect(within(sidebar).getByText("System")).toBeInTheDocument();
  expect(within(sidebar).getByText("Import")).toBeInTheDocument();
  expect(within(sidebar).getAllByRole("button").map(button => button.getAttribute("aria-label"))).toEqual([
    "Libraries",
    "Thumbnails",
    "Encoding",
    "Users",
    "Network",
    "Timeouts",
    "Background jobs",
    "Logs",
    "Emby import",
  ]);
});

test("top menus close when another menu or page content is clicked", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const settingsMenu = await screen.findByLabelText("Settings menu");
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

test("regular settings menu hides admin sections and can toggle theme", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Settings menu"));
  expect(screen.queryByRole("menuitem", {name:"Libraries"})).not.toBeInTheDocument();
  expect(screen.queryByRole("menuitem", {name:"Background jobs"})).not.toBeInTheDocument();
  expect(screen.queryByRole("menuitem", {name:"Logs"})).not.toBeInTheDocument();
  const themeToggle = screen.getByRole("menuitem", {name:"Theme: light"});
  fireEvent.click(themeToggle);
  expect(screen.getByLabelText("Settings menu").closest("details")).not.toHaveAttribute("open");
  expect(document.documentElement.dataset.theme).toBe("dark");
  expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"dark"});
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
  mockApi.folderEntries.mockResolvedValueOnce([
    {id:23, name:"Nested", relativePath:"Photos/Nested", type:"folder"},
    {id:101, name:"one.jpg", relativePath:"Photos/one.jpg", type:"media", media:{id:101, folderId:20, relativePath:"Photos/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:103, name:"two.mp4", relativePath:"Photos/two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"Photos/two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"", takenAt:""}}
  ]);
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

test("library media card asks which favorite view before adding", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:0}, {id:31, name:"Travel", count:0}]);
  mockApi.entries.mockResolvedValue([{id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add one.jpg to favorite view"}));
  expect(await screen.findByRole("dialog", {name:"Add one.jpg to favorite view"})).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Favorite view"), {target:{value:"31"}});
  fireEvent.click(screen.getByRole("button", {name:"Add here"}));
  await waitFor(() => expect(mockApi.favoriteMedia).toHaveBeenCalledWith(31, 100));
});

test("favorite add dialog can create a new favorite view first", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([]);
  mockApi.createFavoriteView.mockResolvedValue({id:44, name:"New one", count:0});
  mockApi.entries.mockResolvedValue([{id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add one.jpg to favorite view"}));
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

test("library timeline groups media by editable date inside one library", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"2020/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"},
    {id:102, folderId:20, relativePath:"2020/two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"", takenAt:"2020-08-21T13:34:00Z"},
    {id:101, folderId:21, relativePath:"old.jpg", name:"old.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/timeline"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"2020-08-21"})).toBeInTheDocument();
  expect(screen.getByText("one.jpg")).toBeInTheDocument();
  expect(screen.getByText("two.mp4")).toBeInTheDocument();
  expect(screen.getByRole("heading", {name:"Unknown date"})).toBeInTheDocument();
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
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  const name = await screen.findByLabelText("Name");
  fireEvent.change(name, {target:{value:"renamed.jpg"}});
  fireEvent.change(screen.getByLabelText("GPS"), {target:{value:"50.45,30.52"}});
  expect(mockApi.updateMediaDetails).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"renamed.jpg", gps:"50.45,30.52", takenAt:null}));
  expect(await screen.findByText("Item saved.")).toBeInTheDocument();
  expect(screen.getByRole("link", {name:"GPS: 50.45,30.52"})).toHaveAttribute("href", "/map?item=100");
  fireEvent.click(screen.getByRole("button", {name:"Hide info panel"}));
  expect(screen.getByRole("button", {name:"Show info panel"})).toHaveTextContent("<<");
});

test("media info shows full stored metadata as JSON under root nodes", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.libraryMedia.mockResolvedValue([{id:100, folderId:20, relativePath:"DSC_5743.jpg", name:"DSC_5743.jpg", kind:"image", mimeType:"image/jpeg", size:10, gps:"", takenAt:"2010-08-23T17:54:08Z", metadata:{exif:{Make:"NIKON CORPORATION", Model:"NIKON D50", DateTimeOriginal:"2010:08:23 17:54:08", FNumber:3.5, ExposureTime:0.03333333333, ISO:"0 1600", FocalLength:18, HistoryParams:"darktable-noise", BlueTRC:"(Binary data)", FileModifyDate:"2026:07:29 06:12:16+00:00", MIMEType:"image/jpeg"}, ffprobe:{streams:[{codec_type:"video", codec_name:"mjpeg"}]}}}]);
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
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""},
    {id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("link", {name:"↑ Up"})).toHaveAttribute("href", "/library/1/folder/20");
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

test("media viewer up link goes to containing folder", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue([{id:100, name:"one.jpg", relativePath:"Trips/Day1/one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"Trips/Day1/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}]);
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"Trips/Day1/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""},
    {id:101, folderId:20, relativePath:"Trips/Day1/two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("link", {name:"↑ Up"}));
  await waitFor(() => expect(mockApi.folderEntries).toHaveBeenCalledWith(1, 20));
});

test("folder up link goes to parent folder or library root", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folder.mockResolvedValueOnce({id:21, parentId:20, relativePath:"Photos/Nested"});
  render(<MemoryRouter initialEntries={["/library/1/folder/21"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("link", {name:"↑ Up"})).toHaveAttribute("href", "/library/1/folder/20"));
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
  mockApi.libraryMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"DSC06360.JPG", name:"DSC06360.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{exif:{FileName:"DSC06360.JPG"}}, gps:"", takenAt:""},
    {id:101, folderId:20, relativePath:"DSC06361.JPG", name:"DSC06361.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{exif:{FileName:"DSC06361.JPG"}}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Show info panel"));
  expect(await screen.findByRole("heading", {name:"DSC06360.JPG"})).toBeInTheDocument();
  expect(screen.getByText(/"FileName": "DSC06360.JPG"/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  expect(await screen.findByRole("heading", {name:"DSC06361.JPG"})).toBeInTheDocument();
  expect(screen.queryByText(/"FileName": "DSC06360.JPG"/)).not.toBeInTheDocument();
  expect(screen.getByText(/"FileName": "DSC06361.JPG"/)).toBeInTheDocument();
});

test("old admin settings route redirects to the settings panel", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin/settings"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("heading", {name:"Settings"})).toBeInTheDocument());
});
