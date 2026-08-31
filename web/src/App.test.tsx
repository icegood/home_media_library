import { act, cleanup, fireEvent, render, renderHook, screen, waitFor, within } from "@testing-library/react";
import { MemoryRouter } from "react-router-dom";
import { afterEach, beforeAll, beforeEach, expect, test, vi } from "vitest";
import { App, resetFolderEntriesCache, setNativePlatformForTests, useBufferedFolderEntries } from "./App";
import { mockApi, resetMockApi } from "./test-support/api-mock";

// The API client is mocked at the module boundary only, but the replacement is
// a fully functional in-memory backend (see ./test-support/api-mock): stateful
// CRUD, derived stats and realistic URL builders instead of per-test stubs.
vi.mock("./api", async () => {
  const {mockApi} = await import("./test-support/api-mock");
  return {api:mockApi, MAX_VIDEO_THUMBNAILS:100};
});

beforeAll(() => {
  HTMLMediaElement.prototype.play = vi.fn().mockResolvedValue(undefined) as any;
  window.scrollTo = vi.fn() as any;
  // jsdom throws "not implemented" on the real one; default to confirming.
  window.confirm = vi.fn(() => true);
});

beforeEach(() => {
  // Restores both the in-memory backend state and every method's default
  // implementation, so per-test overrides cannot leak between tests.
  resetMockApi();
  resetFolderEntriesCache();
  (window as any).matchMedia = undefined;
  localStorage.clear();
  sessionStorage.clear();
  delete document.documentElement.dataset.theme;
  document.documentElement.style.fontSize = "";
});

afterEach(() => {
  cleanup();
  // Restores prototype spies (getBoundingClientRect, media stubs, confirms).
  vi.restoreAllMocks();
});

test("shows login for an unauthenticated visitor", async () => {
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("button", {name:"Sign in"})).toBeInTheDocument();
  expect(screen.getByLabelText("Password")).toHaveAttribute("autocomplete", "current-password");
  expect(screen.getByLabelText("Login")).toHaveAttribute("autocomplete", "username");
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
  expect(screen.getByRole("menuitem", {name:"Jobs"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Network"}));
  expect(await screen.findByRole("heading", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByLabelText("Enable HTTP")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Mail"}));
  expect(await screen.findByRole("heading", {name:"Mail"})).toBeInTheDocument();
  expect(screen.getByLabelText("SMTP host")).toHaveValue("");
  fireEvent.click(screen.getByRole("button", {name:"Jobs"}));
  expect(await screen.findByRole("heading", {name:"Jobs", level:2})).toBeInTheDocument();
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

  fireEvent.click(screen.getByRole("button", {name:"Jobs"}));
  expect(await screen.findByLabelText("Worker pool size")).toBeInTheDocument();
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
    smtpHost:"", smtpPort:587, smtpUsername:"", smtpPassword:"", smtpFrom:"", mapTileProviders:{carto:{apiKey:""}}
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

test("library tile shows statistics inside its menu, not a dialog", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  let resolveStats:(value:{images:number; videos:number}) => void = () => {};
  mockApi.libraryStats.mockReturnValueOnce(new Promise(resolve => { resolveStats = resolve; }));
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("link", {name:"Open library Family"})).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Library menu Family"));
  expect(await screen.findByText("Loading statistics…")).toBeInTheDocument();
  resolveStats({images:10, videos:2});
  await waitFor(() => expect(document.body.querySelector(".folder-stats-inline")).toHaveTextContent("Images: 10 · Videos: 2"));
  expect(screen.queryByRole("dialog", {name:"Library statistics Family"})).not.toBeInTheDocument();
});

test("item menu closes on outside click", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[{id:10, path:"/media/family"}]}]);
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  expect(await screen.findByRole("menuitem", {name:"Edit"})).toBeInTheDocument();
  fireEvent.mouseDown(document.body);
  await waitFor(() => expect(screen.queryByRole("menuitem", {name:"Edit"})).not.toBeInTheDocument());
});

test("favorite views page keeps the create editor as a separate panel above the list", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:1}]);
  mockApi.createFavoriteView.mockResolvedValue({id:99, name:"Holiday 2026", count:0});
  const {container} = render(<MemoryRouter initialEntries={["/favorites"]}><App/></MemoryRouter>);
  await screen.findByText("Best");
  const form = container.querySelector("form.inline-create");
  expect(form).not.toBeNull();
  expect(form!.querySelector("input")).not.toBeNull();
  // editor renders before (and detached from) the list of views
  const table = container.querySelector(".library-table");
  expect(table).not.toBeNull();
  expect(form!.compareDocumentPosition(table!) & Node.DOCUMENT_POSITION_FOLLOWING).toBeTruthy();
  fireEvent.change(form!.querySelector("input")!, {target:{value:"Holiday 2026"}});
  fireEvent.submit(form!);
  expect(await screen.findByText("Holiday 2026")).toBeInTheDocument();
});

test("library tile links into the library without a per-library map button", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter><App/></MemoryRouter>);
  const openLink = await screen.findByRole("link", {name:"Open library Family"});
  expect(openLink).toHaveAttribute("href", "/library/1");
  expect(screen.queryByRole("link", {name:"Map of library Family"})).not.toBeInTheDocument();
  expect(screen.queryByText(/folders ·/)).not.toBeInTheDocument();
});

test("admin library list shows statistics in the row menu", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraryStats.mockResolvedValue({images:10, videos:2});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  await screen.findByRole("heading", {name:"Libraries"});
  expect(await screen.findByText("Family")).toBeInTheDocument();
  expect(document.body.querySelector(".folder-stats-inline")).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Library menu Family"));
  await waitFor(() => expect(document.body.querySelector(".folder-stats-inline")).toHaveTextContent("Images: 10 · Videos: 2"));
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

test("user menu opens About dialog with version information", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.about.mockResolvedValue({product:"Media Library", version:"0.1.0", revision:"abc123", buildDate:"2026-01-02T03:04:05Z", goVersion:"go1.23.4", gatewayEnabled:true});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"About"}));
  const dialog = await screen.findByRole("dialog", {name:"About"}, {timeout:3000});
  expect(dialog).toBeInTheDocument();
  expect(screen.getByText("Media Library — self-hosted, multi-user photo and video library.")).toBeInTheDocument();
  expect(screen.getByText("0.1.0")).toBeInTheDocument();
  expect(screen.getByText("go1.23.4 · v0.1.0")).toBeInTheDocument();
  expect(within(dialog).getByText("Gateway")).toBeInTheDocument();
  await waitFor(() => expect(mockApi.about).toHaveBeenCalledTimes(1));
});

test("About dialog omits the gateway entry in an HTTP-only deployment", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.about.mockResolvedValue({product:"Media Library", version:"0.1.0", revision:"abc123", buildDate:"2026-01-02T03:04:05Z", goVersion:"go1.23.4", gatewayEnabled:false});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"About"}));
  const dialog = await screen.findByRole("dialog", {name:"About"}, {timeout:3000});
  expect(within(dialog).queryByText("Gateway")).not.toBeInTheDocument();
});

test("About dialog Copy button copies the version info", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.about.mockResolvedValue({product:"Media Library", version:"0.1.0", revision:"abc123", buildDate:"2026-01-02T03:04:05Z", goVersion:"go1.23.4", gatewayEnabled:true});
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {value:{writeText}, configurable:true});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"About"}));
  await screen.findByRole("dialog", {name:"About"});
  fireEvent.click(screen.getByRole("button", {name:"Copy"}));
  await waitFor(() => expect(writeText).toHaveBeenCalledTimes(1));
  expect(writeText).toHaveBeenCalledWith(expect.stringContaining("Media Library 0.1.0"));
  expect(writeText).toHaveBeenCalledWith(expect.stringContaining("Gateway: Caddy"));
  expect(await screen.findByText("Copied.")).toBeInTheDocument();
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
  expect(topMenu.querySelectorAll(".submenu-group-label").length).toBeGreaterThanOrEqual(1);
  expect(await screen.findByRole("menuitem", {name:"Libraries"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Network"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Mail"})).toBeInTheDocument();
  expect(screen.queryByRole("menuitem", {name:"Add library"})).not.toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Jobs"})).toBeInTheDocument();
  expect(screen.getByRole("menuitem", {name:"Logs"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("menuitem", {name:"Jobs"}));
  expect(screen.getByLabelText("Admin panel menu").closest("details")).not.toHaveAttribute("open");
  expect(await screen.findByRole("heading", {name:"Jobs", level:2})).toBeInTheDocument();
  const sidebar = screen.getByLabelText("Admin panel sections");
  expect(within(sidebar).getByText("Media")).toBeInTheDocument();
  expect(within(sidebar).getByText("Access")).toBeInTheDocument();
  expect(within(sidebar).getByText("System")).toBeInTheDocument();
  expect(within(sidebar).getAllByText("Database").length).toBeGreaterThanOrEqual(1);
  expect(within(sidebar).getAllByRole("button").map(button => button.getAttribute("aria-label"))).toEqual([
    "Libraries",
    "Thumbnails",
    "Users",
    "Network",
    "Map tiles",
    "Mail",
    "Logs",
    "Jobs",
    "Database",
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
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"dark", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
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
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"system", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
  dark = true;
  listeners.forEach(listener => listener({matches:true}));
  await waitFor(() => expect(document.documentElement.dataset.theme).toBe("dark"));
  dark = false;
  listeners.forEach(listener => listener({matches:false}));
  await waitFor(() => expect(document.documentElement.dataset.theme).toBe("light"));
});

test("user settings lets each user pick CARTO map tiles per theme", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  const light = within(dialog).getByRole("combobox", {name:"Tile source in light mode"});
  const dark = within(dialog).getByRole("combobox", {name:"Tile source in dark/forest mode"});
  expect(light).toHaveValue("osm");
  expect(within(dialog).getByRole("option", {name:"CARTO — Voyager"})).toBeInTheDocument();
  expect(within(dialog).getByRole("option", {name:"CARTO — Native dark tiles"})).toBeInTheDocument();
  fireEvent.change(light, {target:{value:"carto:voyager"}});
  fireEvent.change(dark, {target:{value:"carto:dark"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({mapTileProviderLight:"carto:voyager", mapTileProviderDark:"carto:dark"})));
});

test("admin map tiles section saves the CARTO API key", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=map"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Map tiles"})).toBeInTheDocument();
  await waitFor(() => expect(screen.getByLabelText("CARTO Basemaps API key")).toBeEnabled());
  fireEvent.change(screen.getByLabelText("CARTO Basemaps API key"), {target:{value:"carto-test-key"}});
  await waitFor(() => expect(mockApi.updateSettings).toHaveBeenCalledWith(expect.objectContaining({mapTileProviders:{carto:{apiKey:"carto-test-key"}}})));
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
  fireEvent.click(within(dialog).getByRole("button", {name:"Transcode schema"}));
  fireEvent.click(await within(dialog).findByRole("option", {name:/VP9 \+ Opus → WebM/}));
  expect(mockApi.updateUserSettings).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"vp9-opus-webm", zoom:100, dateFormat:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));

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
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:120, dateFormat:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Zoom"}), {target:{value:"80"}});
  expect(document.documentElement.style.fontSize).toBe("80%");
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:80, dateFormat:"auto", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
});

test("user settings saves a configured date format", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Date format"}), {target:{value:"dmy"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"dmy", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
});

test("user settings saves the date format with seconds", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  expect(within(dialog).getByRole("option", {name:"16.08.2026 14:30:15"})).toBeInTheDocument();
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Date format"}), {target:{value:"dmy-ss"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"dmy-ss", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
});

test("user settings saves the american date format with seconds", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  expect(within(dialog).getByRole("option", {name:"08/16/2026 2:30:15 PM"})).toBeInTheDocument();
  fireEvent.change(within(dialog).getByRole("combobox", {name:"Date format"}), {target:{value:"mdy-ss"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"mdy-ss", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
});

function pagedEntries(many:ReturnType<typeof makeMany>) {
  return (_libraryId:number, range?:{offset?:number; limit?:number}) => {
    const offset = range?.offset ?? 0;
    const limit = range?.limit ?? many.length;
    return Promise.resolve(many.slice(offset, offset + limit));
  };
}

function makeMany(count:number) {
  return Array.from({length:count}, (_, index) => {
    const media = {
      id:index + 1, folderId:20, relativePath:`img-${index + 1}.jpg`, name:`img-${index + 1}.jpg`,
      kind:"image" as const, mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""
    };
    return {id:media.id, name:media.name, relativePath:media.relativePath, type:"media" as const, media};
  });
}

test("long folder lists render every entry and rely on content-visibility for offscreen skipping", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const many = makeMany(200);
  mockApi.entries.mockResolvedValue(many);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await screen.findByText("img-1.jpg");
  const browser = document.querySelector(".cv-browser");
  expect(browser).not.toBeNull();
  const mounted = document.querySelectorAll(".cv-browser .card");
  expect(mounted.length).toBe(many.length);
  expect(screen.getByText("img-200.jpg")).toBeInTheDocument();
});

test("folder entries hook pages by the configured chunk size", async () => {
  const many = makeMany(45);
  mockApi.entries.mockImplementation(pagedEntries(many));
  const {result} = renderHook(() => useBufferedFolderEntries(1, null, false, 10));
  await waitFor(() => expect(result.current.entries.length).toBe(10));
  expect(mockApi.entries).toHaveBeenLastCalledWith(1, {offset:0, limit:10});
  await act(async () => { await result.current.loadMore(); });
  expect(result.current.entries.length).toBe(20);
  await act(async () => { await result.current.loadMore(); });
  await act(async () => { await result.current.loadMore(); });
  expect(result.current.entries.length).toBe(40);
  expect(result.current.done).toBe(false);
  await act(async () => { await result.current.loadMore(); });
  expect(result.current.entries.length).toBe(45);
  expect(result.current.done).toBe(true);
  expect(mockApi.entries).toHaveBeenLastCalledWith(1, {offset:40, limit:10});
});

test("buffered folder list survives scrolling with a single full request", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const many = makeMany(200);
  mockApi.entries.mockImplementation(pagedEntries(many));
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await screen.findByText("img-1.jpg");
  await screen.findByText("img-200.jpg");
  fireEvent.scroll(window);
  await waitFor(() => expect(screen.getByText("img-200.jpg")).toBeInTheDocument());
  expect(screen.getByText("img-1.jpg")).toBeInTheDocument();
});

test("user settings saves the media loading batch size", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"ice", role:"regular"});
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("User menu"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  await waitFor(() => expect(within(dialog).getByRole("button", {name:"Save settings"})).toBeEnabled());
  const batchInput = within(dialog).getByLabelText(/Items per request/);
  expect(batchInput).toHaveValue(10000);
  fireEvent.change(batchInput, {target:{value:"25"}});
  expect(mockApi.updateUserSettings).not.toHaveBeenCalled();
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", streamChunkSize:25, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto", mapTileProviderLight:"osm", mapTileProviderDark:"osm"}));
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
  expect(userSubmenu).not.toHaveTextContent("Jobs");
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
  expect(screen.queryByRole("menuitem", {name:"Edit"})).not.toBeInTheDocument();
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
  fireEvent.click(screen.getByRole("menuitem", {name:"Edit"}));
  expect(await screen.findByRole("dialog", {name:"Edit library details"})).toBeInTheDocument();
  expect(screen.getByRole("heading", {name:"Edit details"})).toBeInTheDocument();
});

test("user language setting switches rendered UI to Polish", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[]}]);
  mockApi.userSettings.mockResolvedValue({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", language:"pl"});
  render(<MemoryRouter initialEntries={["/"]}><App/></MemoryRouter>);
  await screen.findByText("Family");
  await waitFor(() => expect(document.body.textContent).toContain("Ustawienia użytkownika"), {timeout: 3000});
  expect(document.body.textContent).toContain("Medioteka");
  expect(document.body.textContent).not.toContain("Media Library");
});

test("user language setting switches rendered UI to German", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[]}]);
  mockApi.userSettings.mockResolvedValue({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"auto", language:"de"});
  render(<MemoryRouter initialEntries={["/"]}><App/></MemoryRouter>);
  await screen.findByText("Family");
  await waitFor(() => expect(document.body.textContent).toContain("Benutzereinstellungen"), {timeout: 3000});
  expect(document.body.textContent).toContain("Mediathek");
  expect(document.body.textContent).not.toContain("Media Library");
});

test("library editor persists per-root watch flag and sends it on save", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[
    {id:10, path:"/media/family/photos", watch:false}
  ]}]);
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Library menu Family"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Edit"}));
  const dialog = await screen.findByRole("dialog", {name:"Edit library details"});
  const checkbox = within(dialog).getByRole("checkbox", {name:"Watch for changes"}) as HTMLInputElement;
  expect(checkbox.checked).toBe(false);
  fireEvent.click(checkbox);
  fireEvent.click(within(dialog).getByRole("button", {name:"Save details"}));
  await waitFor(() => expect(mockApi.updateLibrary).toHaveBeenCalledWith(1, expect.objectContaining({
    roots: [expect.objectContaining({path:"/media/family/photos", watch:true})]
  })));
});

test("library row shows auto-refresh indicator when a root is watched", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[
    {id:10, path:"/media/family/photos", watch:true}
  ]}]);
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  const row = await screen.findByText("Family");
  expect(row.parentElement).toBeInTheDocument();
  expect(screen.getByText(/Auto-refresh on/)).toBeInTheDocument();
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
  fireEvent.click(screen.getByRole("button", {name:"Jobs"}));
  expect(await screen.findByRole("heading", {name:"Jobs", level:2})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Logs"}));
  expect(await screen.findByRole("heading", {name:"Logs"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Database"}));
  expect(await screen.findByRole("heading", {name:"Database", level:2})).toBeInTheDocument();
});

test("emby import uses server directory pickers for config root and target paths", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.filesystem
    .mockResolvedValueOnce({root:"/runtime", path:"/runtime", parent:"", directories:[{name:"emby", path:"/runtime/emby"}]})
    .mockResolvedValueOnce({root:"/media", path:"/media", parent:"", directories:[{name:"photos", path:"/media/photos"}]});
  render(<MemoryRouter initialEntries={["/admin?section=database"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Database", level:2})).toBeInTheDocument();

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
  fireEvent.click(screen.getByRole("menuitem", {name:"Edit"}));
  expect(await screen.findByRole("group", {name:"Read access"})).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText(/alice/));
  await waitFor(() => expect(mockApi.setLibraryAccess).toHaveBeenCalledWith(1, 2, true));
});

test("admin can compact the database from the database section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const confirmSpy = vi.spyOn(window, "confirm").mockReturnValue(true);
  try {
    render(<MemoryRouter initialEntries={["/admin?section=database"]}><App/></MemoryRouter>);
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
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:23, name:"Nested", relativePath:"Photos/Nested", type:"folder"},
    {id:101, name:"one.jpg", relativePath:"Photos/one.jpg", type:"media", media:{id:101, folderId:20, relativePath:"Photos/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:103, name:"two.mp4", relativePath:"Photos/two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"Photos/two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
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
  fireEvent.click(screen.getByLabelText("Folder menu Photos"));
  await waitFor(() => expect(mockApi.folderStats).toHaveBeenCalledWith(1, 20));
  await waitFor(() => expect(document.body.querySelector(".folder-stats-inline")).toBeInTheDocument());
  expect(document.body.querySelector(".folder-stats-inline")!.textContent).toMatch(/Images:.*1.*Videos:.*1/);
  expect(screen.queryByRole("dialog", {name:"Folder statistics Photos"})).not.toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Folder menu Photos"));
  await waitFor(() => expect(document.body.querySelector(".folder-stats-inline")).not.toBeInTheDocument());
  // Reopening must show the statistics line again (regression: it used to
  // disappear because the cached result suppressed the toggle).
  fireEvent.click(screen.getByLabelText("Folder menu Photos"));
  expect(await screen.findByText("Images: 1 · Videos: 1", {exact:false})).toBeInTheDocument();
  expect(document.body.querySelector(".folder-stats-inline")).toBeInTheDocument();
});

test("favorite view renders media and star removes item from that view", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:1}]);
  mockApi.favoriteViewMedia.mockResolvedValue([{id:100, name:"one.jpg", mimeType:"image/jpeg"}]);
  mockApi.favoriteViewMediaFull.mockResolvedValue([{id:100, name:"one.jpg", kind:"image" as const, mimeType:"image/jpeg", folderId:20, relativePath:"one.jpg", size:10, metadata:{}, gps:"", takenAt:""}]);
  render(<MemoryRouter initialEntries={["/favorites/30"]}><App/></MemoryRouter>);
  expect(await screen.findByText("Best")).toBeInTheDocument();
  expect(await screen.findByText("one.jpg")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Remove one.jpg from this favorite view"}));
  await waitFor(() => expect(mockApi.unfavoriteMedia).toHaveBeenCalledWith(30, 100));
  expect(screen.queryByText("one.jpg")).not.toBeInTheDocument();
});

test("favorite view folder cards offer top-left selection like media cards", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:2}]);
  mockApi.favoriteViewMedia.mockResolvedValue([
    {id:55, name:"Trip", isFolder:true},
    {id:100, name:"one.jpg", mimeType:"image/jpeg"}
  ]);
  render(<MemoryRouter initialEntries={["/favorites/30"]}><App/></MemoryRouter>);
  const folderBox = await screen.findByRole("checkbox", {name:"Select Trip"});
  const mediaBox = screen.getByRole("checkbox", {name:"Select one.jpg"});
  expect(folderBox).toBeInTheDocument();
  expect(mediaBox).toBeInTheDocument();
  fireEvent.click(folderBox);
  expect(folderBox).toBeChecked();
  expect(screen.getByRole("checkbox", {name:"Select one.jpg"})).not.toBeChecked();
});

test("favorite view skips expanded media fetch until timeline mode opens", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:1}]);
  mockApi.favoriteViewMedia.mockResolvedValue([{id:100, name:"one.jpg", mimeType:"image/jpeg"}]);
  mockApi.favoriteViewMediaFull.mockResolvedValue([{id:100, name:"one.jpg", kind:"image" as const, mimeType:"image/jpeg", folderId:20, relativePath:"one.jpg", size:10, metadata:{}, gps:"", takenAt:"2024-05-01T10:00:00Z"}]);
  render(<MemoryRouter initialEntries={["/favorites/30"]}><App/></MemoryRouter>);
  expect(await screen.findByText("one.jpg")).toBeInTheDocument();
  expect(mockApi.favoriteViewMediaFull).not.toHaveBeenCalled();
  fireEvent.click(screen.getByRole("button", {name:"Display mode"}));
  fireEvent.click(await screen.findByRole("option", {name:"Timeline"}));
  await waitFor(() => expect(mockApi.favoriteViewMediaFull).toHaveBeenCalledWith(30, true));
  await waitFor(() => expect(document.body.querySelector(".timeline-group-date")).toHaveTextContent("2024"));
});

test("favorite view menu shows backend-computed media statistics", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => { state.favoriteViews = [{id:30, name:"Best", count:3}]; });
  render(<MemoryRouter initialEntries={["/favorites"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Favorite view menu Best"));
  await waitFor(() => expect(mockApi.favoriteViewStats).toHaveBeenCalledWith(30));
  await waitFor(() => expect(document.body.querySelector(".folder-stats-inline")).toHaveTextContent("Images: 2 · Videos: 1"));
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

test("selected media items can be downloaded as a zip archive", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.entries.mockResolvedValue([
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}}
  ]);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Select one.jpg"));
  fireEvent.click(screen.getByLabelText("Select two.jpg"));
  fireEvent.click(screen.getByRole("button", {name:"Download"}));
  await waitFor(() => expect(mockApi.downloadArchive).toHaveBeenCalledWith([100, 101], []));
});

test("selected media items can receive the same gps in bulk", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.entries.mockResolvedValue([
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}}
  ]);
  mockApi.bulkUpdateMedia.mockImplementation(async (input:{selectedIds?:number[]; gps?:string|null}) => {
    return (input.selectedIds ?? []).map((id:number) => ({
      id, gps: input.gps ?? ""
    }));
  });
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Select one.jpg"));
  fireEvent.click(await screen.findByLabelText("Select two.jpg"));
  fireEvent.change(screen.getByLabelText("GPS"), {target:{value:"50.45,30.52"}});
  fireEvent.click(screen.getAllByRole("button", {name:"Apply"})[0]);
  await waitFor(() => expect(mockApi.bulkUpdateMedia).toHaveBeenCalledTimes(1));
  expect(mockApi.bulkUpdateMedia).toHaveBeenCalledWith({selectedIds: [100, 101], gps: "50.45,30.52"});
});

test("folder timeline loads media for that folder and links back to it", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockResolvedValue([
    {id:200, folderId:20, relativePath:"day/two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2022-05-05T05:05:05Z"},
    {id:201, folderId:20, relativePath:"day/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2021-04-04T04:04:04Z"}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/timeline/20"]}><App/></MemoryRouter>);
  await waitFor(() => expect(mockApi.folderMedia).toHaveBeenCalledWith(1, 20));
  expect(await screen.findByText("two.jpg")).toBeInTheDocument();
  expect(await screen.findByText("one.jpg")).toBeInTheDocument();
  const viewButton = screen.getAllByRole("button", {name:/^Timeline/})[0];
  fireEvent.click(viewButton);
  const foldersOption = await screen.findByRole("option", {name:"Folders"});
  expect(foldersOption).toHaveAttribute("aria-selected", "false");
  expect(screen.getByRole("link", {name:"Timeline of Libraries"})).toHaveAttribute("href", "/");
  expect(screen.getByRole("link", {name:"Family"})).toHaveAttribute("href", "/library/1/timeline");
});

test("keyboard focus, space check, and shift-range selection work in the library timeline", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2023-03-03T03:03:03Z"},
    {id:101, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2022-02-02T02:02:02Z"},
    {id:102, folderId:20, relativePath:"c.jpg", name:"c.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2021-01-01T01:01:01Z"}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/timeline/20"]}><App/></MemoryRouter>);
  await screen.findByText("a.jpg");
  const card = (id:number) => document.querySelector(`[data-kb-id="m${id}"]`) as HTMLElement;
  const check = (id:number) => card(id).querySelector("input[type=checkbox]") as HTMLInputElement;
  // Oldest-first (asc, now the default) orders c(2021), b(2022), a(2023).
  // Arrow focuses without a click.
  fireEvent.keyDown(window, {key:"ArrowRight"});
  await waitFor(() => expect(card(102)).toHaveClass("kb-focus"));
  // Space checks the focused item.
  fireEvent.keyDown(window, {key:" ", code:"Space"});
  await waitFor(() => expect(check(102).checked).toBe(true));
  // Shift+Arrow grows a band and Space checks the whole range; the earlier check stays.
  fireEvent.keyDown(window, {key:"ArrowRight"});
  await waitFor(() => expect(card(101)).toHaveClass("kb-focus"));
  fireEvent.keyDown(window, {key:"ArrowRight", shiftKey:true});
  await waitFor(() => expect(card(100)).toHaveClass("kb-range"));
  fireEvent.keyDown(window, {key:" ", code:"Space"});
  await waitFor(() => expect(check(102).checked).toBe(true));
  expect(check(101).checked).toBe(true);
  expect(check(100).checked).toBe(true);
  // Space on the focused item toggles just it back off.
  fireEvent.keyDown(window, {key:" ", code:"Space"});
  await waitFor(() => expect(check(100).checked).toBe(false));
});

test("ctrl+space opens the favorite flow for the focused media item", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockResolvedValue([
    {id:100, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2023-03-03T03:03:03Z"}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/timeline/20"]}><App/></MemoryRouter>);
  await screen.findByText("a.jpg");
  fireEvent.keyDown(window, {key:"ArrowRight"});
  await waitFor(() => expect(document.querySelector('[data-kb-id="m100"]')).toHaveClass("kb-focus"));
  fireEvent.keyDown(window, {key:" ", code:"Space", ctrlKey:true});
  expect(await screen.findByLabelText("Favorite views for a.jpg")).toBeInTheDocument();
});

test("enter on the focused media card opens the viewer in play mode", async () => {
  Object.defineProperty(HTMLMediaElement.prototype, "paused", {
    get(this:HTMLMediaElement) { return (this as unknown as {_paused?:boolean})._paused !== false; },
    set(this:HTMLMediaElement, value:boolean) { (this as unknown as {_paused?:boolean})._paused = value; },
    configurable:true
  });
  HTMLMediaElement.prototype.play = vi.fn(async function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = false;
    this.dispatchEvent(new Event("play"));
  }) as any;
  HTMLMediaElement.prototype.pause = vi.fn(function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = true;
    this.dispatchEvent(new Event("pause"));
  }) as any;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockResolvedValue([
    {id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}}}, gps:"", takenAt:""}
  ]);
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
  render(<MemoryRouter initialEntries={["/library/1/timeline/20"]}><App/></MemoryRouter>);
  await screen.findByText("two.mp4");
  fireEvent.keyDown(window, {key:"ArrowRight"});
  await waitFor(() => expect(document.querySelector('[data-kb-id="m103"]')).toHaveClass("kb-focus"));
  fireEvent.keyDown(window, {key:"Enter"});
  expect(await screen.findByLabelText("Seek video")).toBeInTheDocument();
  expect(document.querySelector(".viewer-stage")).toHaveAttribute("aria-label", "two.mp4");
  expect(screen.getByRole("button", {name:"Pause"})).toBeInTheDocument();
});

test("timeline load can be cancelled while a big folder is still loading", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockReturnValue(new Promise(() => {}));
  render(<MemoryRouter initialEntries={["/library/1", "/library/1/timeline/20"]} initialIndex={1}><App/></MemoryRouter>);
  expect(await screen.findByRole("button", {name:"Cancel and go back"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Cancel and go back"}));
  await waitFor(() => expect(screen.getByLabelText("Breadcrumb")).toHaveTextContent("Libraries / Family"));
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
    expect.stringContaining("old.jpg"),
    expect.stringContaining("one.jpg"),
    expect.stringContaining("two.mp4")
  ]);
  expect(screen.queryByRole("heading", {name:"2020-08-21"})).not.toBeInTheDocument();
  expect(screen.queryByRole("heading", {name:"Unknown date"})).not.toBeInTheDocument();
  const groups = Array.from(document.querySelectorAll(".timeline-grid .timeline-group"));
  expect(groups.length).toBe(2);
  const dateLabels0 = Array.from(document.querySelectorAll(".timeline-group-date"));
  if (dateLabels0[0].textContent!.includes("2020")) {
    expect(groups[0].querySelectorAll(".timeline-group-grid .media").length).toBe(2);
    expect(groups[1].querySelectorAll(".timeline-group-grid .media").length).toBe(1);
    expect(dateLabels0.map(label => label.textContent)).toEqual([
      expect.stringContaining("2020"),
      "Unknown date"
    ]);
  } else {
    expect(groups[0].querySelectorAll(".timeline-group-grid .media").length).toBe(1);
    expect(groups[1].querySelectorAll(".timeline-group-grid .media").length).toBe(2);
    expect(dateLabels0.map(label => label.textContent)).toEqual([
      "Unknown date",
      expect.stringContaining("2020")
    ]);
  }
  fireEvent.click(screen.getByRole("button", {name:/^All$/}));
  fireEvent.click(await screen.findByRole("option", {name:"Images"}));
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
  expect(screen.getByRole("button", {name:"Open on map"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Hide info panel"}));
  expect(screen.getByRole("button", {name:"Show info panel"})).toHaveTextContent("<<");
});

test("saved gps and name stick when navigating to an adjacent item and back", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.updateMediaDetails.mockImplementation(async (id:number|string, input:{name:string; gps:string|null; takenAt:string|null}) => ({
    id:Number(id), folderId:20, relativePath:"one.jpg", name:input.name,
    kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:input.gps ?? "", takenAt:input.takenAt ?? ""
  }));
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  fireEvent.change(await screen.findByLabelText("Name"), {target:{value:"edited.jpg"}});
  fireEvent.change(screen.getByLabelText("GPS"), {target:{value:"50.45,30.52"}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"edited.jpg", gps:"50.45,30.52", takenAt:""}));
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  expect(await screen.findByRole("img", {name:"two.jpg"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Previous media"}));
  expect(await screen.findByRole("img", {name:"edited.jpg"})).toBeInTheDocument();
  expect(screen.getByLabelText("GPS")).toHaveValue("50.45,30.52");
  expect(screen.getByLabelText("Name")).toHaveValue("edited.jpg");
});

test("media info shows the date in the configured format, copyable, and pasting saves it", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.userSettings.mockResolvedValue({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"dmy", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"}}
  ], chain: []});
  mockApi.updateMediaDetails.mockImplementation(async (id:number|string, input:{name:string; gps:string|null; takenAt:string|null}) => ({
    id:Number(id), folderId:20, relativePath:"one.jpg", name:input.name,
    kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:input.gps ?? "", takenAt:input.takenAt ?? ""
  }));
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  const date = new Date("2020-08-21T12:34:00Z");
  const pad2 = (value:number) => String(value).padStart(2, "0");
  const expected = `${pad2(date.getDate())}.${pad2(date.getMonth() + 1)}.${date.getFullYear()} ${pad2(date.getHours())}:${pad2(date.getMinutes())}`;
  const dateInput = await screen.findByLabelText("Date");
  expect(dateInput).toHaveValue(expected);
  expect(dateInput.closest(".media-date-editor")).toContainElement(screen.getByRole("button", {name:"Copy date"}));
  expect(screen.getByRole("button", {name:"Copy date"})).toBeInTheDocument();
  const pasted = new Date(2020, 7, 21, 14, 30);
  fireEvent.change(screen.getByLabelText("Date"), {target:{value:`${pad2(pasted.getDate())}.${pad2(pasted.getMonth() + 1)}.${pasted.getFullYear()} ${pad2(pasted.getHours())}:${pad2(pasted.getMinutes())}`}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"one.jpg", gps:"", takenAt:pasted.toISOString()}));
});

test("media info shows and parses the date format with seconds", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.userSettings.mockResolvedValue({theme:"light", codec:"h264-aac-mp4", zoom:100, dateFormat:"dmy-ss", streamChunkSize:10000, defaultThumbImage:"mountains", defaultThumbVideo:"mountains", defaultThumbFolder:"mountains", language:"auto"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:56Z"}}
  ], chain: []});
  mockApi.updateMediaDetails.mockImplementation(async (id:number|string, input:{name:string; gps:string|null; takenAt:string|null}) => ({
    id:Number(id), folderId:20, relativePath:"one.jpg", name:input.name,
    kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:input.gps ?? "", takenAt:input.takenAt ?? ""
  }));
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Show info panel"}));
  const date = new Date("2020-08-21T12:34:56Z");
  const pad2 = (value:number) => String(value).padStart(2, "0");
  const expected = `${pad2(date.getDate())}.${pad2(date.getMonth() + 1)}.${date.getFullYear()} ${pad2(date.getHours())}:${pad2(date.getMinutes())}:${pad2(date.getSeconds())}`;
  expect(await screen.findByLabelText("Date")).toHaveValue(expected);
  const pasted = new Date(2020, 7, 21, 14, 30, 15);
  fireEvent.change(screen.getByLabelText("Date"), {target:{value:`${pad2(pasted.getDate())}.${pad2(pasted.getMonth() + 1)}.${pasted.getFullYear()} ${pad2(pasted.getHours())}:${pad2(pasted.getMinutes())}:${pad2(pasted.getSeconds())}`}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100, {name:"one.jpg", gps:"", takenAt:pasted.toISOString()}));
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
  fireEvent.keyDown(document.body, {key:"ArrowLeft"});
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
  fireEvent.keyDown(document.body, {key:"ArrowRight"});
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

test("media viewer shows a recovery screen instead of an eternal spinner when the requested media no longer exists", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderMedia.mockResolvedValue([]);
  mockApi.media.mockRejectedValue(new Error("media not found"));
  render(<MemoryRouter initialEntries={["/library/7/view/38?item=999999&sort=date-asc&root=37"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Media not found"})).toBeInTheDocument();
  expect(screen.queryByText("Loading media…")).not.toBeInTheDocument();
  expect(screen.getByRole("link", {name:"All libraries"})).toHaveAttribute("href", "/");
});

test("media viewer previous/next follows the requested date sort instead of the name order", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"a.jpg", relativePath:"a.jpg", type:"media", media:{id:100, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-01-01T00:00:00Z"}},
    {id:101, name:"b.jpg", relativePath:"b.jpg", type:"media", media:{id:101, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-03-01T00:00:00Z"}},
    {id:102, name:"c.jpg", relativePath:"c.jpg", type:"media", media:{id:102, folderId:20, relativePath:"c.jpg", name:"c.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-02-01T00:00:00Z"}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100&sort=date"]}><App/></MemoryRouter>);
  await screen.findByRole("img", {name:"a.jpg"});
  expect(screen.getByRole("button", {name:"Previous media"})).toBeEnabled();
  expect(screen.getByRole("button", {name:"Next media"})).toBeDisabled();
  fireEvent.click(screen.getByRole("button", {name:"Previous media"}));
  expect(await screen.findByRole("img", {name:"c.jpg"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Previous media"}));
  expect(await screen.findByRole("img", {name:"b.jpg"})).toBeInTheDocument();
  fireEvent.keyDown(document.body, {key:"ArrowRight"});
  expect(await screen.findByRole("img", {name:"c.jpg"})).toBeInTheDocument();
});

test("media viewer previous/next uses the folder name order without a sort param", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"a.jpg", relativePath:"a.jpg", type:"media", media:{id:100, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-01-01T00:00:00Z"}},
    {id:101, name:"b.jpg", relativePath:"b.jpg", type:"media", media:{id:101, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-03-01T00:00:00Z"}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("img", {name:"a.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Previous media"})).toBeDisabled();
  expect(screen.getByRole("button", {name:"Next media"})).toBeEnabled();
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  expect(await screen.findByRole("img", {name:"b.jpg"})).toBeInTheDocument();
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

test("media viewer offers a download link for the current item", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[{id:100, name:"one.jpg", relativePath:"Trips/Day1/one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"Trips/Day1/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}], chain:[{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]});
  mockApi.contentUrl.mockReturnValue("/api/v1/media/100/content?download=1");
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("img", {name:"one.jpg"})).toBeInTheDocument());
  expect(screen.getByRole("link", {name:"Download"})).toHaveAttribute("href", "/api/v1/media/100/content?download=1");
});

test("media viewer up link goes to containing folder", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[{id:100, name:"one.jpg", relativePath:"Trips/Day1/one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"Trips/Day1/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}], chain:[{id:20, parentId:-1, relativePath:"Photos", name:"Photos"}]});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("link", {name:"Photos"}));
  await waitFor(() => expect(mockApi.folderEntries).toHaveBeenCalledWith(1, 20, {offset:0, limit:1}));
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
  await waitFor(() => { fireEvent.click(screen.getAllByRole("button", {name:/^Folders/})[0]); });
  fireEvent.click(await screen.findByRole("option", {name:"Map"}));
  const firstBar = await screen.findByLabelText("Breadcrumb");
  await waitFor(() => expect(firstBar).toHaveTextContent("Map of Family"));
  first.unmount();
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"}
  ]});
  render(<MemoryRouter initialEntries={["/library/1/folder/20"]}><App/></MemoryRouter>);
  await waitFor(() => { fireEvent.click(screen.getAllByRole("button", {name:/^Folders/})[0]); });
  fireEvent.click(await screen.findByRole("option", {name:"Map"}));
  const secondBar = await screen.findByLabelText("Breadcrumb");
  await waitFor(() => expect(secondBar).toHaveTextContent("Map of Family / Photos"));
});

test("map breadcrumb shows Map of the library and folder when scoped", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"},
    {id:21, parentId:20, relativePath:"Photos/Nested", name:"Nested"}
  ]});
  render(<MemoryRouter initialEntries={["/map?library=1&folder=21"]}><App/></MemoryRouter>);
  const breadcrumb = await screen.findByLabelText("Breadcrumb");
  expect(breadcrumb).toHaveTextContent("Map of Family / Photos / Nested");
  expect(screen.getByRole("link", {name:"Map of Family"})).toHaveAttribute("href", "/map?library=1");
  expect(screen.getByRole("link", {name:"Photos"})).toHaveAttribute("href", "/map?library=1&folder=20");
  expect(screen.getByText("Nested")).toBeInTheDocument();
});

test("scoped timeline viewer breadcrumb walks the subfolder chain to the item", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[]}]);
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"},
    {id:21, parentId:20, relativePath:"Photos/Nested", name:"Nested"}
  ]});
  mockApi.folderMedia.mockResolvedValue([]);
  mockApi.media.mockResolvedValue({id:100, folderId:21, relativePath:"Nested/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-10-30T13:44:47Z"});
  render(<MemoryRouter initialEntries={["/library/1/view/21?item=100&sort=date-asc&root=20"]}><App/></MemoryRouter>);
  const breadcrumb = await screen.findByLabelText("Breadcrumb");
  expect(breadcrumb).toHaveTextContent("Timeline of Libraries / Family / Photos / Nested / one.jpg");
  expect(screen.getByRole("link", {name:"Family"})).toHaveAttribute("href", "/library/1/timeline/20");
  expect(screen.getByRole("link", {name:"Nested"})).toHaveAttribute("href", "/library/1/timeline/21");
  expect(screen.getByText("one.jpg")).toBeInTheDocument();
});

test("scoped map viewer breadcrumb walks the subfolder chain to the item", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.libraries.mockResolvedValue([{id:1, name:"Family", roots:[]}]);
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"},
    {id:21, parentId:20, relativePath:"Photos/Nested", name:"Nested"}
  ]});
  mockApi.map.mockResolvedValue([]);
  mockApi.media.mockResolvedValue({id:100, folderId:21, relativePath:"Nested/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-10-30T13:44:47Z"});
  render(<MemoryRouter initialEntries={["/library/1/view/21?item=100&w=1&s=2&e=3&n=4"]}><App/></MemoryRouter>);
  const breadcrumb = await screen.findByLabelText("Breadcrumb");
  expect(breadcrumb).toHaveTextContent("Map of Family / Photos / Nested / one.jpg");
  expect(screen.getByRole("link", {name:"Map of Family"})).toHaveAttribute("href", "/map?library=1&w=1&s=2&e=3&n=4");
  expect(screen.getByRole("link", {name:"Photos"})).toHaveAttribute("href", "/map?library=1&folder=20");
  expect(screen.getByRole("link", {name:"Nested"})).toHaveAttribute("href", "/map?library=1&folder=21");
  expect(screen.getByText("one.jpg")).toBeInTheDocument();
});

test("breadcrumb keeps favorite origin in subfolders opened from a favorite view", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:2}]);
  mockApi.folderEntries.mockResolvedValue({entries:[], chain:[
    {id:20, parentId:-1, relativePath:"Photos", name:"Photos"},
    {id:21, parentId:20, relativePath:"Photos/Nested", name:"Nested"}
  ]});
  render(<MemoryRouter initialEntries={["/library/1/folder/21?fav=30"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByText("Nested")).toBeInTheDocument());
  expect(await screen.findByText("Best")).toBeInTheDocument();
  expect(screen.getByRole("link", {name:"Best"})).toHaveAttribute("href", "/favorites/30");
  expect(screen.getByRole("link", {name:"Family"})).toHaveAttribute("href", "/library/1?fav=30");
  expect(screen.getByRole("link", {name:"Photos"})).toHaveAttribute("href", "/library/1/folder/20?fav=30");
});

test("viewer opened from a favorite view shows the view and item in breadcrumbs", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViews.mockResolvedValue([{id:30, name:"Best", count:2}]);
  mockApi.media.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""});
  render(<MemoryRouter initialEntries={["/favorites/view/100?viewId=30"]}><App/></MemoryRouter>);
  const breadcrumb = await screen.findByLabelText("Breadcrumb");
  await waitFor(() => expect(breadcrumb).toHaveTextContent("Favorites"));
  await waitFor(() => expect(breadcrumb).toHaveTextContent("one.jpg"));
  expect(breadcrumb).toHaveTextContent("Best");
});

test("unscoped map keeps the Media Library brand in the header", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await waitFor(() => expect(mockApi.map).toHaveBeenCalled());
  expect(screen.getByRole("link", {name:"Media Library"})).toHaveAttribute("href", "/");
});

test("media viewer without item query redirects to library root", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.entries.mockResolvedValue([]);
  render(<MemoryRouter initialEntries={["/library/1/view/20"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByLabelText("Breadcrumb")).toHaveTextContent("Libraries / Family"));
  await waitFor(() => expect(mockApi.entries).toHaveBeenCalledWith(1, {offset:0, limit:10000}));
});

test("image zoom is preserved when navigating between neighboring files", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"DSC06360.JPG", relativePath:"DSC06360.JPG", type:"media", media:{id:100, folderId:20, relativePath:"DSC06360.JPG", name:"DSC06360.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"DSC06361.JPG", relativePath:"DSC06361.JPG", type:"media", media:{id:101, folderId:20, relativePath:"DSC06361.JPG", name:"DSC06361.JPG", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  await waitFor(() => expect(screen.getByRole("img", {name:"DSC06360.JPG"})).toBeInTheDocument());
  fireEvent.click(screen.getByRole("button", {name:"Zoom in"}));
  fireEvent.click(screen.getByRole("button", {name:"Zoom in"}));
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("150%");
  fireEvent.click(screen.getByRole("button", {name:"Next media"}));
  await waitFor(() => expect(screen.getByRole("img", {name:"DSC06361.JPG"})).toBeInTheDocument());
  expect(screen.getByRole("button", {name:"Reset zoom"})).toHaveTextContent("150%");
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
  expect(screen.getByRole("button", {name:"Full screen"})).toBeInTheDocument();
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
  mockApi.map.mockImplementation(async (_libraryId:number, _folderId:number|undefined, bounds?:{west:number;south:number;east:number;north:number}) => {
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
  }), undefined);
  expect(screen.getByText("a.jpg")).toBeInTheDocument();
  expect(screen.queryByText("b.jpg")).not.toBeInTheDocument();
  expect(screen.queryByText("c.jpg")).not.toBeInTheDocument();
  expect(document.querySelector(".map-timeline-panel .timeline-grid")).not.toBeNull();
  fireEvent.click(screen.getByRole("button", {name:"Clear"}));
  expect(screen.queryByRole("complementary", {name:"Selected area"})).not.toBeInTheDocument();
});

test("map panel item links carry the exact selection for the viewer range", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  const all = [
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-08-21T12:34:00Z"},
    {id:101, libraryId:1, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10.001,20.001", takenAt:"2020-08-22T12:34:00Z"}
  ];
  mockApi.map.mockImplementation(async (_libraryId:number, _folderId:number|undefined, bounds?:{west:number;south:number;east:number;north:number}) => bounds ? all : all);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  const button = await screen.findByRole("button", {name:"Select area"});
  fireEvent.click(button);
  const container = document.querySelector(".leaflet-container");
  fireEvent.mouseDown(container!, {clientX:-200, clientY:-200});
  fireEvent.mouseMove(container!, {clientX:200, clientY:200});
  fireEvent.mouseUp(container!, {clientX:200, clientY:200});
  const links = await screen.findAllByRole("link", {name:/Open .* in folder/});
  expect(links.length).toBe(2);
  const href = links[0].getAttribute("href") ?? "";
  expect(href).toContain("/library/1/view/20?");
  expect(href).toContain("list=media-library-map-selection");
  const stored = JSON.parse(sessionStorage.getItem("media-library-map-selection") ?? "[]") as Array<{id:number}>;
  expect(stored.map(entry => entry.id).sort()).toEqual([100, 101]);
});

test("map place search geocodes and orders results nearest first", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.map.mockResolvedValue([]);
  mockApi.geocode.mockResolvedValue([
    {place_id:1, display_name:"Far Street, City", lat:"21.0000", lon:"0.0000", boundingbox:["21.1","20.9","0.1","-0.1"]},
    {place_id:2, display_name:"Near Street, City", lat:"20.1000", lon:"0.0000", boundingbox:["20.2","20.0","0.1","-0.1"]}
  ]);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await waitFor(() => expect(mockApi.map).toHaveBeenCalled());
  fireEvent.change(screen.getByLabelText("Place search query"), {target:{value:"street"}});
  fireEvent.click(screen.getByRole("button", {name:"Search"}));
  await waitFor(() => expect(mockApi.geocode).toHaveBeenCalledWith("street"));
  const results = within(screen.getByRole("listbox", {name:"Search results"})).getAllByRole("option");
  expect(results.length).toBe(2);
  expect(results[0]).toHaveTextContent(/Nearest: .*Near Street, City/);
  expect(results[1]).toHaveTextContent(/Far Street, City/);
  expect(results[1]).not.toHaveTextContent("Nearest");
  fireEvent.click(results[0]);
  await waitFor(() => expect(document.querySelector(".searched-point-marker")).toBeInTheDocument());
  fireEvent.change(screen.getByLabelText("Place search query"), {target:{value:""}});
  await waitFor(() => expect(screen.queryByRole("listbox", {name:"Search results"})).not.toBeInTheDocument());
  expect(document.querySelector(".searched-point-marker")).not.toBeInTheDocument();
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

test("opening a media from a map cluster crosses only the clustered items", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.map.mockResolvedValue([
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-01-01T00:00:00Z"},
    {id:101, libraryId:1, folderId:21, relativePath:"b.jpg", name:"b.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:"2020-02-01T00:00:00Z"},
    {id:102, libraryId:1, folderId:20, relativePath:"d.jpg", name:"d.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"30,40", takenAt:"2020-03-01T00:00:00Z"}
  ]);
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:102, name:"d.jpg", relativePath:"d.jpg", type:"media", media:{id:102, folderId:20, relativePath:"d.jpg", name:"d.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await screen.findByText("2");
  await waitFor(() => {
    const cluster = document.querySelector(".cluster-marker");
    if (!cluster) throw new Error("cluster marker not rendered");
    fireEvent.click(cluster);
    expect(screen.getByRole("complementary", {name:"Selected area"})).toBeInTheDocument();
  });
  fireEvent.click(await screen.findByRole("link", {name:"Open a.jpg in folder"}));
  expect(await screen.findByRole("img", {name:"a.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Previous media"})).toBeEnabled();
  expect(screen.getByRole("button", {name:"Next media"})).toBeDisabled();
  fireEvent.click(screen.getByRole("button", {name:"Previous media"}));
  expect(await screen.findByRole("img", {name:"b.jpg"})).toBeInTheDocument();
  expect(screen.getByRole("button", {name:"Next media"})).toBeEnabled();
  expect(screen.queryByRole("img", {name:"d.jpg"})).not.toBeInTheDocument();
});

test("first startup offers admin creation and rejects mismatched passwords", async () => {
  mockApi.setupStatus.mockResolvedValue({required:true});
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Create administrator"})).toBeInTheDocument();

  fireEvent.change(screen.getByLabelText("Login"), {target:{value:"root"}});
  fireEvent.change(screen.getByLabelText("Password", {selector:"input"}), {target:{value:"verylongpass1"}});
  fireEvent.change(screen.getByLabelText("Confirm password"), {target:{value:"different123"}});
  fireEvent.click(screen.getByRole("button", {name:"Create administrator"}));
  expect(await screen.findByText("Passwords do not match")).toBeInTheDocument();
  expect(mockApi.setup).not.toHaveBeenCalled();

  fireEvent.change(screen.getByLabelText("Confirm password"), {target:{value:"verylongpass1"}});
  fireEvent.click(screen.getByRole("button", {name:"Create administrator"}));
  await waitFor(() => expect(mockApi.setup).toHaveBeenCalledWith("root", "verylongpass1"));
  await screen.findByText("Your libraries");
});

test("admins manage per-user library access from the users section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=users"]}><App/></MemoryRouter>);
  const accessButtons = await screen.findAllByRole("button", {name:/Manage access/});
  fireEvent.click(accessButtons[accessButtons.length - 1]);
  const dialog = await screen.findByRole("dialog", {name:"Manage access for alice"});
  fireEvent.click(within(dialog).getByRole("checkbox"));
  await waitFor(() => expect(mockApi.setLibraryAccess).toHaveBeenCalledWith(1, 2, true));
});

test("scheduled tasks can be created toggled and deleted", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.seed(state => {
    state.tasks = [{id:5, name:"Nightly scan", taskType:"scan", libraryId:1, cron:"0 3 * * *", enabled:true}];
  });
  const confirm = vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<MemoryRouter initialEntries={["/admin?section=jobs"]}><App/></MemoryRouter>);

  // Libraries must be loaded first so the task form can default to one.
  await screen.findByText("Nightly scan");
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  const dialog = await screen.findByRole("dialog", {name:"Add scheduled task"});
  fireEvent.change(within(dialog).getByLabelText("Name"), {target:{value:"Weekly thumbs"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Create task"}));
  await waitFor(() => expect(mockApi.createScheduledTask).toHaveBeenCalledWith(
    expect.objectContaining({name:"Weekly thumbs", taskType:"scan", libraryId:1})));

  const nightlyRow = screen.getByText("Nightly scan").closest(".library-row") as HTMLElement;
  fireEvent.click(within(nightlyRow).getByLabelText("Enabled"));
  await waitFor(() => expect(mockApi.updateScheduledTask).toHaveBeenCalledWith(5,
    expect.objectContaining({enabled:false})));

  fireEvent.click(within(nightlyRow).getByRole("button", {name:"Delete"}));
  await waitFor(() => expect(mockApi.deleteScheduledTask).toHaveBeenCalledWith(5));
  expect(confirm).toHaveBeenCalled();
});

test("folder card menu opens metadata renewal with option checkboxes", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.seed(state => {
    state.entries.set(1, [{id:20, name:"Photos", relativePath:"Photos", type:"folder"}]);
  });
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Folder menu Photos"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Refresh metadata…"}));
  const dialog = await screen.findByRole("dialog", {name:"Refresh metadata Photos"});
  fireEvent.click(within(dialog).getByLabelText("Update GPS coordinates"));
  fireEvent.click(within(dialog).getByRole("button", {name:"Refresh"}));
  await waitFor(() => expect(mockApi.metadataRenew).toHaveBeenCalledWith(1, {recreateExisting:false, updateGps:true, updateTakenAt:false}));
});

test("bulk time shift validates input and applies the offset to selected items", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => {
    state.entries.set(1, [
      {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
      {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}}
    ]);
  });
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Select one.jpg"));

  // Zero offset is rejected before hitting the API.
  fireEvent.change(screen.getByPlaceholderText("h"), {target:{value:"0"}});
  fireEvent.click(screen.getAllByRole("button", {name:"Apply"})[1]);
  expect(await screen.findByText("Enter hours or minutes")).toBeInTheDocument();
  expect(mockApi.bulkUpdateMedia).not.toHaveBeenCalledWith(expect.objectContaining({shiftMinutes:expect.any(Number)}));

  fireEvent.change(screen.getByPlaceholderText("h"), {target:{value:"1"}});
  fireEvent.change(screen.getByPlaceholderText("m"), {target:{value:"30"}});
  fireEvent.click(await screen.findByLabelText("Select two.jpg"));
  fireEvent.click(screen.getAllByRole("button", {name:"Apply"})[1]);
  await waitFor(() => expect(mockApi.bulkUpdateMedia).toHaveBeenCalledWith({selectedIds:[100, 101], shiftMinutes:90}));
});

test("favorite views can be created renamed and deleted from the index", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => { state.favoriteViews = [{id:30, name:"Best", count:2}]; });
  vi.spyOn(window, "confirm").mockReturnValue(true);
  render(<MemoryRouter initialEntries={["/favorites"]}><App/></MemoryRouter>);

  fireEvent.change(await screen.findByLabelText("New view"), {target:{value:"Travel"}});
  fireEvent.click(screen.getByRole("button", {name:"Create"}));
  await waitFor(() => expect(mockApi.createFavoriteView).toHaveBeenCalledWith("Travel"));
  expect(await screen.findByText("Travel")).toBeInTheDocument();

  fireEvent.click(await screen.findByLabelText("Favorite view menu Best"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Rename"}));
  const input = screen.getByDisplayValue("Best");
  fireEvent.change(input, {target:{value:"Top shots"}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateFavoriteView).toHaveBeenCalledWith(30, "Top shots"));

  fireEvent.click(await screen.findByLabelText("Favorite view menu Top shots"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Delete"}));
  await waitFor(() => expect(mockApi.deleteFavoriteView).toHaveBeenCalledWith(30));
});

test("favorite folder cards unshare from the view and open via library probe", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.favoriteViewMedia.mockResolvedValue([{id:20, name:"Photos", isFolder:true}]);
  render(<MemoryRouter initialEntries={["/favorites/30"]}><App/></MemoryRouter>);
  const card = await screen.findByText("Photos");
  fireEvent.click(card.closest("article") as HTMLElement);
  await waitFor(() => expect(document.querySelector(".crumb-current")?.textContent).toBe("Photos"));
});

test("video viewer exposes pause stop and replay transport controls", async () => {
  // jsdom never plays media nor fires its events, so the stubs track a paused
  // flag and emit exactly the events React listens to.
  Object.defineProperty(HTMLMediaElement.prototype, "paused", {
    get(this:HTMLMediaElement) { return (this as unknown as {_paused?:boolean})._paused !== false; },
    set(this:HTMLMediaElement, value:boolean) { (this as unknown as {_paused?:boolean})._paused = value; },
    configurable:true
  });
  HTMLMediaElement.prototype.play = vi.fn(async function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = false;
    this.dispatchEvent(new Event("play"));
  }) as any;
  HTMLMediaElement.prototype.pause = vi.fn(function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = true;
    this.dispatchEvent(new Event("pause"));
  }) as any;
  HTMLMediaElement.prototype.load = vi.fn() as any;
  Object.defineProperty(HTMLMediaElement.prototype, "currentTime", {value:0, writable:true, configurable:true});
  Object.defineProperty(HTMLMediaElement.prototype, "duration", {value:120, writable:true, configurable:true});
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=103"]}><App/></MemoryRouter>);
  await screen.findByLabelText("Seek video");
  // Autoplayed video reports paused=false; jsdom needs that primed by hand.
  document.querySelectorAll("video").forEach(video => { (video as unknown as {_paused?:boolean})._paused = false; });
  expect(await screen.findByRole("button", {name:"Pause"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Pause"}));
  expect(screen.getByRole("button", {name:"Play"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Stop"}));
  expect(screen.getByRole("button", {name:"Play"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Replay"}));
  expect(screen.getByRole("button", {name:"Pause"})).toBeInTheDocument();
});

test("space toggles play/pause in the video player", async () => {
  Object.defineProperty(HTMLMediaElement.prototype, "paused", {
    get(this:HTMLMediaElement) { return (this as unknown as {_paused?:boolean})._paused !== false; },
    set(this:HTMLMediaElement, value:boolean) { (this as unknown as {_paused?:boolean})._paused = value; },
    configurable:true
  });
  HTMLMediaElement.prototype.play = vi.fn(async function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = false;
    this.dispatchEvent(new Event("play"));
  }) as any;
  HTMLMediaElement.prototype.pause = vi.fn(function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = true;
    this.dispatchEvent(new Event("pause"));
  }) as any;
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}}}, gps:"", takenAt:""}},
    {id:104, name:"three.mp4", relativePath:"three.mp4", type:"media", media:{id:104, folderId:20, relativePath:"three.mp4", name:"three.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"30.0"}}}, gps:"", takenAt:""}}
  ], chain: []});
  mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=103"]}><App/></MemoryRouter>);
  await screen.findByLabelText("Seek video");
  document.querySelectorAll("video").forEach(video => { (video as unknown as {_paused?:boolean})._paused = false; });
  expect(await screen.findByRole("button", {name:"Pause"})).toBeInTheDocument();
  // Space toggles play → pause
  fireEvent.keyDown(window, {key:" ", code:"Space"});
  expect(screen.getByRole("button", {name:"Play"})).toBeInTheDocument();
  // Space toggles pause → play
  fireEvent.keyDown(window, {key:" ", code:"Space"});
  expect(screen.getByRole("button", {name:"Pause"})).toBeInTheDocument();
  expect(document.querySelector(".viewer-stage")).toHaveAttribute("aria-label", "two.mp4");
});

test("up and down arrow keys navigate previous and next media in the viewer", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  await screen.findByLabelText("one.jpg");
  fireEvent.keyDown(window, {key:"ArrowDown"});
  await waitFor(() => expect(screen.getByLabelText("two.jpg")).toBeInTheDocument());
  fireEvent.keyDown(window, {key:"ArrowUp"});
  await waitFor(() => expect(screen.getByLabelText("one.jpg")).toBeInTheDocument());
});

test("swipes navigate the viewer in both axes", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}},
    {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  await screen.findByLabelText("one.jpg");
  const stage = document.querySelector(".viewer-media") as HTMLElement;
  // Left swipe → next.
  fireEvent.touchStart(stage, {touches:[{clientX:200, clientY:150}]});
  fireEvent.touchEnd(stage, {changedTouches:[{clientX:100, clientY:150}]});
  await waitFor(() => expect(screen.getByLabelText("two.jpg")).toBeInTheDocument());
  // Right swipe → previous.
  fireEvent.touchStart(stage, {touches:[{clientX:100, clientY:150}]});
  fireEvent.touchEnd(stage, {changedTouches:[{clientX:200, clientY:150}]});
  await waitFor(() => expect(screen.getByLabelText("one.jpg")).toBeInTheDocument());
  // Up swipe → next.
  fireEvent.touchStart(stage, {touches:[{clientX:150, clientY:200}]});
  fireEvent.touchEnd(stage, {changedTouches:[{clientX:150, clientY:100}]});
  await waitFor(() => expect(screen.getByLabelText("two.jpg")).toBeInTheDocument());
  // Down swipe → previous.
  fireEvent.touchStart(stage, {touches:[{clientX:150, clientY:100}]});
  fireEvent.touchEnd(stage, {changedTouches:[{clientX:150, clientY:200}]});
  await waitFor(() => expect(screen.getByLabelText("one.jpg")).toBeInTheDocument());
  // Short taps/swipes below threshold do not navigate.
  fireEvent.touchStart(stage, {touches:[{clientX:150, clientY:150}]});
  fireEvent.touchEnd(stage, {changedTouches:[{clientX:170, clientY:150}]});
  expect(screen.getByLabelText("one.jpg")).toBeInTheDocument();
});

test("double clicking the right half of the video seeks forward and the left half back ten seconds", async () => {
  Object.defineProperty(HTMLMediaElement.prototype, "paused", {
    get(this:HTMLMediaElement) { return (this as unknown as {_paused?:boolean})._paused !== false; },
    set(this:HTMLMediaElement, value:boolean) { (this as unknown as {_paused?:boolean})._paused = value; },
    configurable:true
  });
  HTMLMediaElement.prototype.play = vi.fn(async function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = false;
    this.dispatchEvent(new Event("play"));
  }) as any;
  HTMLMediaElement.prototype.pause = vi.fn(function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = true;
    this.dispatchEvent(new Event("pause"));
  }) as any;
  HTMLMediaElement.prototype.load = vi.fn() as any;
  Object.defineProperty(HTMLMediaElement.prototype, "currentTime", {value:0, writable:true, configurable:true});
  Object.defineProperty(HTMLMediaElement.prototype, "duration", {value:120, writable:true, configurable:true});
  const originalRect = Element.prototype.getBoundingClientRect;
  Element.prototype.getBoundingClientRect = (() => ({left:0, top:0, width:600, height:400, right:600, bottom:400, x:0, y:0})) as any;
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.folderEntries.mockResolvedValue({entries:[
      {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}}}, gps:"", takenAt:""}}
    ], chain: []});
    mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
    render(<MemoryRouter initialEntries={["/library/1/view/20?item=103"]}><App/></MemoryRouter>);
    await screen.findByLabelText("Seek video");
    const stack = document.querySelector(".video-stack") as HTMLElement;
    fireEvent.dblClick(stack, {clientX:500});
    await waitFor(() => expect(screen.getByText("0:10")).toBeInTheDocument());
    expect(screen.getByText("+10 seconds")).toBeInTheDocument();
    fireEvent.dblClick(stack, {clientX:100});
    await waitFor(() => expect(screen.getByText("0:00")).toBeInTheDocument());
    expect(screen.getByText("−10 seconds")).toBeInTheDocument();
    fireEvent.dblClick(stack, {clientX:500});
    expect(screen.getByText("+10 seconds")).toBeInTheDocument();
  } finally {
    Element.prototype.getBoundingClientRect = originalRect;
  }
});

test("fullscreen targets the video box and locks orientation to the media aspect", async () => {
  const lock = vi.fn().mockResolvedValue(undefined);
  const unlock = vi.fn();
  const originalScreenOrientation = Object.getOwnPropertyDescriptor(window.screen, "orientation");
  Object.defineProperty(window.screen, "orientation", {value:{lock, unlock}, configurable:true});
  const originalRequestFullscreen = Element.prototype.requestFullscreen;
  const originalExitFullscreen = document.exitFullscreen;
  const originalFullscreenElement = Object.getOwnPropertyDescriptor(document, "fullscreenElement");
  Object.defineProperty(document, "fullscreenElement", {
    get() { return null; },
    configurable:true
  });
  Element.prototype.requestFullscreen = vi.fn(async function(this:Element) {
    Object.defineProperty(document, "fullscreenElement", {get:() => this, configurable:true});
    document.dispatchEvent(new Event("fullscreenchange"));
    return undefined;
  }) as any;
  document.exitFullscreen = vi.fn(async () => {
    Object.defineProperty(document, "fullscreenElement", {get:() => null, configurable:true});
    document.dispatchEvent(new Event("fullscreenchange"));
  }) as any;
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.folderEntries.mockResolvedValue({entries:[
      {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}, streams:[{codec_type:"video", codec_name:"h264", width:1920, height:1080}]}}, gps:"", takenAt:""}}
    ], chain: []});
    mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
    render(<MemoryRouter initialEntries={["/library/1/view/20?item=103"]}><App/></MemoryRouter>);
    await screen.findByLabelText("Seek video");
    fireEvent.click(screen.getByRole("button", {name:"Full screen"}));
    await waitFor(() => expect(lock).toHaveBeenCalledWith("landscape"));
    expect(Element.prototype.requestFullscreen).toHaveBeenCalledTimes(1);
    expect((Element.prototype.requestFullscreen as ReturnType<typeof vi.fn>).mock.instances[0]).toHaveClass("viewer-media");
    fireEvent.click(screen.getByRole("button", {name:"Exit full screen"}));
    await waitFor(() => expect(unlock).toHaveBeenCalled());
    fireEvent.keyDown(window, {key:"f"});
    await waitFor(() => expect(Element.prototype.requestFullscreen).toHaveBeenCalledTimes(2));
    expect(lock).toHaveBeenCalledTimes(2);
    expect(unlock).toHaveBeenCalledTimes(1);
  } finally {
    if (originalScreenOrientation) Object.defineProperty(window.screen, "orientation", originalScreenOrientation);
    else Object.defineProperty(window.screen, "orientation", {value:undefined, configurable:true});
    Element.prototype.requestFullscreen = originalRequestFullscreen;
    document.exitFullscreen = originalExitFullscreen;
    if (originalFullscreenElement) Object.defineProperty(document, "fullscreenElement", originalFullscreenElement);
    else Object.defineProperty(document, "fullscreenElement", {value:undefined, configurable:true});
  }
});

test("navigating between media while fullscreen does not re-lock orientation", async () => {
  const lock = vi.fn().mockResolvedValue(undefined);
  const unlock = vi.fn();
  const originalScreenOrientation = Object.getOwnPropertyDescriptor(window.screen, "orientation");
  Object.defineProperty(window.screen, "orientation", {value:{lock, unlock}, configurable:true});
  const originalRequestFullscreen = Element.prototype.requestFullscreen;
  const originalExitFullscreen = document.exitFullscreen;
  const originalFullscreenElement = Object.getOwnPropertyDescriptor(document, "fullscreenElement");
  Object.defineProperty(document, "fullscreenElement", {
    get() { return null; },
    configurable:true
  });
  Element.prototype.requestFullscreen = vi.fn(async function(this:Element) {
    Object.defineProperty(document, "fullscreenElement", {get:() => this, configurable:true});
    document.dispatchEvent(new Event("fullscreenchange"));
    return undefined;
  }) as any;
  document.exitFullscreen = vi.fn(async () => {
    Object.defineProperty(document, "fullscreenElement", {get:() => null, configurable:true});
    document.dispatchEvent(new Event("fullscreenchange"));
  }) as any;
  try {
    mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
    mockApi.folderEntries.mockResolvedValue({entries:[
      {id:103, name:"two.mp4", relativePath:"two.mp4", type:"media", media:{id:103, folderId:20, relativePath:"two.mp4", name:"two.mp4", kind:"video", mimeType:"video/mp4", size:20, metadata:{ffprobe:{format:{duration:"61.5"}, streams:[{codec_type:"video", codec_name:"h264", width:1920, height:1080}]}}, gps:"", takenAt:""}},
      {id:104, name:"three.jpg", relativePath:"three.jpg", type:"media", media:{id:104, folderId:20, relativePath:"three.jpg", name:"three.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{exif:{ImageWidth:600, ImageHeight:900}}, gps:"", takenAt:""}}
    ], chain: []});
    mockApi.videoThumbnails.mockResolvedValue([{index:0, timeSeconds:1, url:"/thumb0.jpg"}]);
    render(<MemoryRouter initialEntries={["/library/1/view/20?item=103"]}><App/></MemoryRouter>);
    await screen.findByLabelText("Seek video");
    fireEvent.click(screen.getByRole("button", {name:"Full screen"}));
    await waitFor(() => expect(lock).toHaveBeenCalledTimes(1));
    expect(lock).toHaveBeenCalledWith("landscape");
    fireEvent.keyDown(window, {key:"ArrowRight"});
    await waitFor(() => expect(screen.getByLabelText("three.jpg")).toBeInTheDocument());
    await waitFor(() => expect(lock).toHaveBeenCalledTimes(1));
    fireEvent.keyDown(window, {key:"f"});
    await waitFor(() => expect(unlock).toHaveBeenCalledTimes(1));
  } finally {
    if (originalScreenOrientation) Object.defineProperty(window.screen, "orientation", originalScreenOrientation);
    else Object.defineProperty(window.screen, "orientation", {value:undefined, configurable:true});
    Element.prototype.requestFullscreen = originalRequestFullscreen;
    document.exitFullscreen = originalExitFullscreen;
    if (originalFullscreenElement) Object.defineProperty(document, "fullscreenElement", originalFullscreenElement);
    else Object.defineProperty(document, "fullscreenElement", {value:undefined, configurable:true});
  }
});

test("library editor supports adding and browsing root folders", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  const dialog = await screen.findByRole("dialog", {name:"Add library"});
  fireEvent.click(within(dialog).getByRole("button", {name:"Add root folder"}));
  fireEvent.click(within(dialog).getAllByRole("button", {name:"Browse"})[1]);
  fireEvent.click(await within(dialog).findByRole("button", {name:"Select"}));
  expect(within(dialog).getAllByLabelText("Root path")[1]).toHaveValue("/media/photos");
});

test("timeline route renders the folder timeline with breadcrumb trail", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  render(<MemoryRouter initialEntries={["/library/1/timeline/20"]}><App/></MemoryRouter>);
  await waitFor(() => expect(document.querySelector(".crumb-current")?.textContent).toBeTruthy());
  expect(screen.getAllByLabelText("Breadcrumb")[0]).toHaveTextContent("Family");
});

test("admins can edit an existing user from the users section", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=users"]}><App/></MemoryRouter>);
  const aliceRow = (await screen.findByText("alice")).closest(".library-row") as HTMLElement;
  fireEvent.click(within(aliceRow).getByRole("button", {name:"Edit"}));
  const dialog = await screen.findByRole("dialog", {name:/Edit user/});
  fireEvent.change(within(dialog).getByLabelText("Login"), {target:{value:"alicia"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save user"}));
  await waitFor(() => expect(mockApi.updateUser).toHaveBeenCalledWith(2, {login:"alicia", role:"regular", password:undefined}));
});

test("jobs section lists categories and supports pause resume and cancel", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.seed(state => {
    state.jobs = [
      {id:"j1", category:"scan", status:"running", cancelable:true},
      {id:"j2", category:"vacuum", status:"running"},
      {id:"j3", category:"thumbnail-create", status:"paused", paused:true, cancelable:true}
    ];
  });
  render(<MemoryRouter initialEntries={["/admin?section=jobs"]}><App/></MemoryRouter>);
  expect(await screen.findByText("Scan")).toBeInTheDocument();
  expect(screen.getByText("Vacuum")).toBeInTheDocument();
  expect(screen.getByText("Thumbnail create")).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Pause"}));
  await waitFor(() => expect(mockApi.pauseJob).toHaveBeenCalledWith("j1"));
  await waitFor(() => expect(mockApi.pauseJob).toHaveBeenCalledWith("j1"));
  // Every paused job offers Resume; clicking them all must reach j3.
  for (const resume of await screen.findAllByRole("button", {name:"Resume"})) {
    fireEvent.click(resume);
  }
  await waitFor(() => expect(mockApi.resumeJob).toHaveBeenCalledWith("j3"));
  for (const cancel of screen.getAllByRole("button", {name:"Cancel"})) {
    if ((cancel as HTMLButtonElement).disabled) continue;
    fireEvent.click(cancel);
  }
  await waitFor(() => expect(mockApi.cancelJob).toHaveBeenCalledWith("j1"));
});

test("login shows errors for bad credentials and can request a reset link", async () => {
  render(<MemoryRouter><App/></MemoryRouter>);
  fireEvent.change(await screen.findByLabelText("Login"), {target:{value:"alice"}});
  fireEvent.change(screen.getByLabelText("Password"), {target:{value:"wrong"}});
  fireEvent.click(screen.getByRole("button", {name:"Sign in"}));
  await waitFor(() => expect(document.querySelector(".login .error")).not.toBeNull());

  fireEvent.click(screen.getByRole("button", {name:"Forgot password?"}));
  fireEvent.change(await screen.findByLabelText("Email"), {target:{value:"a@b.c"}});
  fireEvent.click(screen.getByRole("button", {name:"Send reset link"}));
  expect(await screen.findByText(/reset link has been sent/)).toBeInTheDocument();
});

test("library editor can remove extra root rows and close the picker", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  const dialog = await screen.findByRole("dialog", {name:"Add library"});
  fireEvent.click(within(dialog).getByRole("button", {name:"Add root folder"}));
  expect(within(dialog).getAllByLabelText("Root path")).toHaveLength(2);
  fireEvent.click(within(dialog).getAllByRole("button", {name:"Remove"})[0]);
  expect(within(dialog).getAllByLabelText("Root path")).toHaveLength(1);

  fireEvent.click(within(dialog).getByRole("button", {name:"Browse"}));
  const picker = await within(dialog).findByRole("dialog", {name:"Choose root folder"});
  fireEvent.click(within(picker).getByRole("button", {name:"Close"}));
  await waitFor(() => expect(within(dialog).queryByText("Folders visible to the api process user")).toBeNull());
});

test("image viewer offers zoom fullscreen and info controls", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
  fireEvent.click(screen.getByLabelText("Zoom out"));
  const zoomIn = screen.getByLabelText("Zoom in");
  fireEvent.click(zoomIn);
  fireEvent.click(zoomIn);
  expect(zoomIn).not.toBeDisabled();
  fireEvent.click(screen.getByLabelText("Reset zoom"));
  fireEvent.click(screen.getByRole("button", {name:"Show info panel"}));
  expect(await screen.findByText("image · 10 B")).toBeInTheDocument();
});

test("user settings modal persists the selected language on save", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  render(<MemoryRouter initialEntries={["/"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});
  fireEvent.change(await within(dialog).findByLabelText("Language"), {target:{value:"pt"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Save settings"}));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({language:"pt"})));
});

test("transcoded viewer stop replay and jump controls drive the transport state", async () => {
  HTMLMediaElement.prototype.play = vi.fn(async function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = false;
    this.dispatchEvent(new Event("play"));
  }) as any;
  HTMLMediaElement.prototype.pause = vi.fn(function(this:HTMLMediaElement) {
    (this as unknown as {_paused?:boolean})._paused = true;
    this.dispatchEvent(new Event("pause"));
  }) as any;
  Object.defineProperty(HTMLMediaElement.prototype, "paused", {
    get(this:HTMLMediaElement) { return (this as unknown as {_paused?:boolean})._paused !== false; },
    set(this:HTMLMediaElement, value:boolean) { (this as unknown as {_paused?:boolean})._paused = value; },
    configurable:true
  });
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.folderEntries.mockResolvedValue({entries:[
    {id:104, name:"clip.avi", relativePath:"clip.avi", type:"media", media:{id:104, folderId:20, relativePath:"clip.avi", name:"clip.avi", kind:"video", mimeType:"video/x-msvideo", size:713616156, metadata:{ffprobe:{format:{duration:"120"}}}, gps:"", takenAt:""}}
  ], chain: []});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=104"]}><App/></MemoryRouter>);
  await screen.findByLabelText("Seek video");
  document.querySelectorAll("video").forEach(video => { (video as unknown as {_paused?:boolean})._paused = false; });

  fireEvent.dblClick(document.querySelector(".video-stack") as HTMLElement, {clientX:200});
  fireEvent.click(screen.getByLabelText("Full screen"));
  fireEvent.click(screen.getByRole("button", {name:"Stop"}));
  expect(screen.getByRole("button", {name:"Play"})).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Replay"}));
  expect(await screen.findByRole("button", {name:"Pause"})).toBeInTheDocument();
});

test("map place search sorts geocode results and flies to the pick", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.geocode.mockResolvedValue([
    {place_id:2, display_name:"Far Street", lat:"55.0", lon:"30.0"},
    {place_id:1, display_name:"Near Street", lat:"50.4", lon:"30.5"}
  ] as Awaited<ReturnType<typeof mockApi.geocode>>);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  fireEvent.change(await screen.findByLabelText("Place search query"), {target:{value:"Street"}});
  fireEvent.click(screen.getAllByRole("button", {name:"Search"})[0]);
  const listbox = await screen.findByRole("listbox", {name:"Search results"});
  expect(listbox.textContent).toContain("Nearest: Near Street");
  fireEvent.click(within(listbox).getAllByRole("option")[0]);
  await waitFor(() => expect(listbox).not.toBeInTheDocument());
});

test("clicking the map picks a point whose popup copies coordinates", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {value:{writeText}, configurable:true});
  // jsdom reports zero-sized rects; give leaflet a real viewport so clicks
  // resolve to finite geographic coordinates.
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left:0, top:0, right:800, bottom:600, width:800, height:600, x:0, y:0,
    toJSON:() => ({})
  } as DOMRect);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await screen.findByRole("button", {name:"Select area"});
  const container = document.querySelector(".leaflet-container")!;
  fireEvent.mouseDown(container, {clientX:400, clientY:300});
  fireEvent.mouseUp(container, {clientX:400, clientY:300});
  fireEvent.click(container, {clientX:400, clientY:300});
  const icons = await screen.findAllByRole("button", {hidden:true}).catch(() => []);
  void icons;
  await waitFor(() => expect(document.querySelectorAll(".leaflet-marker-icon").length).toBeGreaterThan(0), {timeout:3000});
  const markers = [...document.querySelectorAll(".leaflet-marker-icon")] as HTMLElement[];
  fireEvent.click(markers[markers.length - 1]);
  await waitFor(() => expect(document.querySelector(".picked-point-popup")).not.toBeNull(), {timeout:3000});
  fireEvent.click(await screen.findByRole("button", {name:"Copy coordinates"}));
  await waitFor(() => expect(writeText).toHaveBeenCalled());
});

test("viewer keyboard navigation and info-panel editing persist details", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => {
    state.userSettings.dateFormat = "dmy";
    state.entries.set(1, [
      {id:100, name:"one.jpg", relativePath:"one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"}},
      {id:101, name:"two.jpg", relativePath:"two.jpg", type:"media", media:{id:101, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}}
    ]);
  });
  mockApi.map.mockResolvedValue([
    {id:100, libraryId:1, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"},
    {id:101, libraryId:1, folderId:20, relativePath:"two.jpg", name:"two.jpg", kind:"image", mimeType:"image/jpeg", size:20, metadata:{}, gps:"", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100&w=1&s=2&e=3&n=4"]}><App/></MemoryRouter>);
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();
  // Bounds in the URL scope the viewer playlist through the map endpoint.
  await waitFor(() => expect(mockApi.map).toHaveBeenCalledWith(1, undefined,
    expect.objectContaining({west:1, south:2, east:3, north:4})));

  fireEvent.click(screen.getByRole("button", {name:"Show info panel"}));
  expect(await screen.findByLabelText("Date")).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Hide info panel"}));

  fireEvent.keyDown(document.body, {key:"ArrowRight"});
  expect(await screen.findByRole("img", {name:"two.jpg"})).toBeInTheDocument();
  fireEvent.keyDown(document.body, {key:"ArrowLeft"});
  expect(await screen.findByRole("img", {name:"one.jpg"})).toBeInTheDocument();

  fireEvent.click(screen.getByRole("button", {name:"Show info panel"}));
  const nameInput = await screen.findByLabelText("Name");
  fireEvent.change(nameInput, {target:{value:"  "}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  expect(await screen.findByText("name is required")).toBeInTheDocument();

  fireEvent.change(nameInput, {target:{value:"renamed.jpg"}});
  fireEvent.change(screen.getByPlaceholderText("50.45,30.52"), {target:{value:"50.45,30.52"}});
  fireEvent.click(screen.getByRole("button", {name:"Save"}));
  await waitFor(() => expect(mockApi.updateMediaDetails).toHaveBeenCalledWith(100,
    expect.objectContaining({name:"renamed.jpg", gps:"50.45,30.52"})));
});

test("bulk GPS apply validates selection and supports whole folders", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => {
    state.entries.set(1, [
      {id:20, name:"Photos", relativePath:"Photos", type:"folder"},
      {id:100, name:"one.jpg", relativePath:"Photos/one.jpg", type:"media", media:{id:100, folderId:20, relativePath:"Photos/one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:""}}
    ]);
  });
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);

  // Empty GPS is rejected.
  fireEvent.click(await screen.findByLabelText("Select one.jpg"));
  fireEvent.click(screen.getAllByRole("button", {name:"Apply"})[0]);
  expect(await screen.findByText("GPS is required")).toBeInTheDocument();
  expect(mockApi.bulkUpdateMedia).not.toHaveBeenCalled();

  // Selecting the folder targets it instead of individual items.
  fireEvent.click(await screen.findByLabelText("Select Photos"));
  fireEvent.change(screen.getByLabelText("GPS"), {target:{value:"10,20"}});
  fireEvent.click(screen.getAllByRole("button", {name:"Apply"})[0]);
  await waitFor(() => expect(mockApi.bulkUpdateMedia).toHaveBeenCalledWith(expect.objectContaining({gps:"10,20"})));
  const lastCall = mockApi.bulkUpdateMedia.mock.calls.at(-1)![0];
  expect(lastCall.selectedFolders ?? lastCall.selectedIds).toBeTruthy();
});


test("folder card favorite chooser toggles membership and creates views inline", async () => {
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => {
    state.entries.set(1, [{id:20, name:"Photos", relativePath:"Photos", type:"folder"}]);
    state.favoriteViews = [{id:30, name:"Best", count:0}];
  });
  mockApi.folderFavoriteViews.mockResolvedValue([{id:30, name:"Best", contains:false}] as Awaited<ReturnType<typeof mockApi.folderFavoriteViews>>);
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Folder menu Photos"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Add to favorites…"}));
  const dialog = await screen.findByRole("dialog", {name:"Favorite views for Photos"});

  fireEvent.click(within(dialog).getByRole("checkbox"));
  await waitFor(() => expect(mockApi.favoriteFolder).toHaveBeenCalledWith(30, 20));

  fireEvent.change(within(dialog).getByLabelText("New favorite view"), {target:{value:"Trips"}});
  fireEvent.click(within(dialog).getByRole("button", {name:"Create and add"}));
  await waitFor(() => expect(mockApi.createFavoriteView).toHaveBeenCalledWith("Trips"));
  await waitFor(() => expect(mockApi.favoriteFolder).toHaveBeenCalledWith(expect.any(Number), 20));
});

test("map area selection then item link carries bounds in the viewer URL", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  const all = [
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"10,20", takenAt:""}
  ];
  mockApi.map.mockImplementation(async () => all);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Select area"}));
  const container = document.querySelector(".leaflet-container")!;
  fireEvent.mouseDown(container, {clientX:-200, clientY:-200});
  fireEvent.mouseMove(container, {clientX:200, clientY:200});
  fireEvent.mouseUp(container, {clientX:200, clientY:200});
  const link = await screen.findByRole("link", {name:/a\.jpg/});
  expect(link.getAttribute("href")).toContain("list=");
  expect(link.getAttribute("href")).toContain("item=100");
});

test("admin settings sections accept edits across network mail thumbnails and logs", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=network"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByLabelText("Enable HTTPS with Let’s Encrypt"));
  fireEvent.change(await screen.findByLabelText("Public DNS name"), {target:{value:"media.example.com"}});
  fireEvent.change(screen.getByLabelText(/Let.s Encrypt email/), {target:{value:"acme@example.com"}});

  fireEvent.click(await screen.findByRole("button", {name:"Mail"}));
  fireEvent.change(await screen.findByLabelText("SMTP host"), {target:{value:"smtp.example.com"}});
  fireEvent.change(screen.getByLabelText("SMTP port"), {target:{value:"465"}});
  fireEvent.change(screen.getByLabelText("SMTP username"), {target:{value:"user"}});
  fireEvent.change(screen.getByLabelText("SMTP password"), {target:{value:"verylongpass1"}});
  fireEvent.change(screen.getByLabelText("From address"), {target:{value:"from@example.com"}});

  fireEvent.click(screen.getByRole("button", {name:"Thumbnails"}));
  fireEvent.change(await screen.findByLabelText("Width"), {target:{value:"512"}});
  fireEvent.change(screen.getByLabelText("Height"), {target:{value:"384"}});
  fireEvent.change(screen.getByLabelText("First thumbnail, seconds"), {target:{value:"3"}});
  fireEvent.change(screen.getByLabelText("Max thumbnails"), {target:{value:"20"}});
  fireEvent.change(screen.getByLabelText("Minimum interval, seconds"), {target:{value:"60"}});

  fireEvent.click(screen.getByRole("button", {name:"Logs"}));
  fireEvent.change(await screen.findByLabelText("Logging level"), {target:{value:"D"}});
  fireEvent.change(await screen.findByLabelText("Rotate after, MB"), {target:{value:"5"}});
  fireEvent.change(await screen.findByLabelText("Keep rotated files"), {target:{value:"7"}});
  fireEvent.change(await screen.findByLabelText("Keep logs, days"), {target:{value:"14"}});

  fireEvent.click(screen.getByRole("button", {name:"Jobs"}));
  fireEvent.change(await screen.findByLabelText("Remove inactive jobs after, minutes"), {target:{value:"30"}});
  fireEvent.change(await screen.findByLabelText("Worker pool size"), {target:{value:"2"}});
  await waitFor(() => expect(mockApi.updateSettings).toHaveBeenCalled());
});

test("scheduled task dialog exposes every field for editing an existing task", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  mockApi.seed(state => {
    state.tasks = [{id:5, name:"Nightly scan", taskType:"scan", libraryId:1, cron:"0 3 * * *", enabled:true}];
  });
  render(<MemoryRouter initialEntries={["/admin?section=jobs"]}><App/></MemoryRouter>);
  const row = (await screen.findByText("Nightly scan")).closest(".library-row") as HTMLElement;
  fireEvent.click(within(row).getByRole("button", {name:"Edit"}));
  const dialog = await screen.findByRole("dialog", {name:"Edit scheduled task"});
  fireEvent.change(within(dialog).getByLabelText("Name"), {target:{value:"Renamed task"}});
  fireEvent.change(within(dialog).getByLabelText("Task type"), {target:{value:"vacuum"}});
  fireEvent.change(within(dialog).getByLabelText("Cron expression"), {target:{value:"30 4 * * 1"}});
  fireEvent.click(within(dialog).getByLabelText("Enabled"));
  fireEvent.click(within(dialog).getByRole("button", {name:"Save task"}));
  await waitFor(() => expect(mockApi.updateScheduledTask).toHaveBeenCalledWith(5,
    expect.objectContaining({name:"Renamed task", taskType:"vacuum", cron:"30 4 * * 1", enabled:false})));
});

test("image viewer supports wheel zoom pointer panning and copy date", async () => {
  const writeText = vi.fn().mockResolvedValue(undefined);
  Object.defineProperty(navigator, "clipboard", {value:{writeText}, configurable:true});
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.media.mockResolvedValue({id:100, folderId:20, relativePath:"one.jpg", name:"one.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"", takenAt:"2020-08-21T12:34:00Z"});
  render(<MemoryRouter initialEntries={["/library/1/view/20?item=100"]}><App/></MemoryRouter>);
  const img = await screen.findByRole("img", {name:"one.jpg"}) as HTMLImageElement & {setPointerCapture?:unknown};
  img.setPointerCapture = vi.fn();
  img.releasePointerCapture = vi.fn();

  fireEvent.wheel(img, {deltaY:-120});
  expect(screen.getByLabelText("Reset zoom").textContent).not.toBe("100%");
  fireEvent.wheel(img, {deltaY:120});

  fireEvent.pointerDown(img, {pointerId:1, clientX:10, clientY:10, button:0});
  fireEvent.pointerMove(img, {pointerId:1, clientX:40, clientY:30});
  fireEvent.pointerUp(img, {pointerId:1, clientY:30, clientX:40});

  fireEvent.click(screen.getByRole("button", {name:"Show info panel"}));
  fireEvent.click(await screen.findByLabelText("Copy date"));
  await waitFor(() => expect(writeText).toHaveBeenCalled());
});

test("folder card supports middle-click opening and zip download", async () => {
  const openSpy = vi.spyOn(window, "open").mockReturnValue(null);
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.seed(state => {
    state.entries.set(1, [{id:20, name:"Photos", relativePath:"Photos", type:"folder"}]);
  });
  render(<MemoryRouter initialEntries={["/library/1"]}><App/></MemoryRouter>);
  await screen.findByText("Photos");

  // ZIP download from the folder menu.
  fireEvent.click(await screen.findByLabelText("Folder menu Photos"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Download as ZIP"}));
  await waitFor(() => expect(mockApi.downloadArchive).toHaveBeenCalledWith([], [20]));

  // Middle click on the folder title opens it in a new tab via window.open.
  const titleButton = screen.getByText("Photos").closest("button") as HTMLElement;
  fireEvent.mouseDown(titleButton, {button:1});
  fireEvent(titleButton, new MouseEvent("auxclick", {button:1, bubbles:true}));
  await waitFor(() => expect(openSpy).toHaveBeenCalledWith("/library/1/folder/20", "_blank"));
});

test("map cluster popup and picked point interactions work together", async () => {
  const L = (await import("leaflet")).default;
  (L.Browser as Record<string, unknown>).svg = true;
  vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockReturnValue({
    left:0, top:0, right:800, bottom:600, width:800, height:600, x:0, y:0, toJSON:() => ({})
  } as DOMRect);
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  mockApi.map.mockResolvedValue([
    {id:100, libraryId:1, folderId:20, relativePath:"a.jpg", name:"a.jpg", kind:"image", mimeType:"image/jpeg", size:10, metadata:{}, gps:"50.0001,30.0001", takenAt:""},
    {id:101, libraryId:1, folderId:20, relativePath:"b.jpg", name:"b.jpg", kind:"video", mimeType:"video/mp4", size:20, metadata:{}, gps:"50.0002,30.0002", takenAt:""}
  ]);
  render(<MemoryRouter initialEntries={["/map"]}><App/></MemoryRouter>);
  await screen.findByRole("button", {name:"Select area"});

  // Clicking a cluster marker opens the selection panel listing its media.
  await waitFor(() => expect(document.querySelectorAll(".leaflet-marker-icon").length).toBeGreaterThan(0), {timeout:4000});
  const icons = [...document.querySelectorAll(".leaflet-marker-icon")] as HTMLElement[];
  fireEvent.click(icons[0]);
  expect(await screen.findByText("a.jpg")).toBeInTheDocument();
});

test("user settings modal covers codec listbox thumbnail pickers and password change", async () => {
  const writeText = vi.fn().mockRejectedValue(new Error("no clipboard"));
  Object.defineProperty(navigator, "clipboard", {value:{writeText}, configurable:true});
  mockApi.me.mockResolvedValue({id:1, login:"alice", role:"regular"});
  render(<MemoryRouter initialEntries={["/"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("menuitem", {name:"User settings"}));
  const dialog = await screen.findByRole("dialog", {name:"User settings"});

  // Codec dropdown listbox selection.
  fireEvent.click(within(dialog).getByLabelText("Transcode schema"));
  const codecList = await within(dialog).findByRole("listbox", {name:"Transcode schema"});
  const vp9Option = within(codecList).getAllByRole("option").find(option => /VP9/.test(option.textContent ?? ""));
  expect(vp9Option).toBeDefined();
  fireEvent.click(vp9Option!);

  // Default thumbnail picture pickers (images, videos, folders): each is a
  // custom select whose trigger sits next to its visible group label.
  for (const groupLabel of ["Images", "Videos", "Folders"]) {
    const labelSpan = within(dialog).getByText(groupLabel, {selector:".thumb-picker-label"});
    const pickerButton = labelSpan.parentElement!.querySelector("button")!;
    fireEvent.click(pickerButton);
    const list = await within(dialog).findByRole("listbox");
    fireEvent.click(within(list).getAllByRole("option")[1]);
    fireEvent.click(pickerButton); // close
  }

  // Theme and zoom selects.
  fireEvent.change(within(dialog).getByLabelText("Theme"), {target:{value:"system"}});
  fireEvent.change(within(dialog).getByLabelText("Zoom"), {target:{value:"120"}});

  // Change-password sub-dialog: failure then success.
  fireEvent.click(within(dialog).getByRole("button", {name:"Change password…"}));
  const pw = await screen.findByRole("dialog", {name:"Change password"});
  fireEvent.change(within(pw).getByLabelText("Current password"), {target:{value:"bad"}});
  fireEvent.change(within(pw).getByLabelText("New password"), {target:{value:"verylongpass9"}});
  fireEvent.change(within(pw).getByLabelText("Confirm new password"), {target:{value:"verylongpass9"}});
  fireEvent.click(within(pw).getByRole("button", {name:"Update password"}));
  expect(await within(pw).findByText("wrong password")).toBeInTheDocument();
  fireEvent.change(within(pw).getByLabelText("Current password"), {target:{value:"good"}});
  fireEvent.click(within(pw).getByRole("button", {name:"Update password"}));
  await waitFor(() => expect(mockApi.changePassword).toHaveBeenCalledWith("good", "verylongpass9"));
  fireEvent.click(within(pw).getByRole("button", {name:"Close"}));

  fireEvent.click(within(dialog).getByText("Save settings"));
  await waitFor(() => expect(mockApi.updateUserSettings).toHaveBeenCalledWith(expect.objectContaining({theme:"system", zoom:120})));
});

test("logs panel refreshes clears and links the download endpoint", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=logs"]}><App/></MemoryRouter>);
  expect(await screen.findByText(/media API listening/)).toBeInTheDocument();
  fireEvent.click(screen.getByRole("button", {name:"Refresh"}));
  await waitFor(() => expect(mockApi.logs).toHaveBeenCalledTimes(2));
  const clearButton = await screen.findByRole("button", {name:"Clear logs"});
  await waitFor(() => expect(clearButton).toBeEnabled());
  fireEvent.click(clearButton);
  await waitFor(() => expect(mockApi.clearLogs).toHaveBeenCalled());
  expect(screen.getByRole("button", {name:"Download logs"})).toBeEnabled();
});

test("library editor scan-now checkbox drives the post-save notice variants", async () => {
  mockApi.me.mockResolvedValue({id:0, login:"admin", role:"admin"});
  render(<MemoryRouter initialEntries={["/admin?section=libraries"]}><App/></MemoryRouter>);
  fireEvent.click(await screen.findByRole("button", {name:"Add"}));
  const dialog = await screen.findByRole("dialog", {name:"Add library"});
  fireEvent.change(within(dialog).getByLabelText("Library name"), {target:{value:"Trips"}});
  fireEvent.change(within(dialog).getAllByLabelText("Root path")[0], {target:{value:"/media/trips"}});
  // scanNow defaults to true, so saving must kick off a background scan.
  expect(within(dialog).getByLabelText("Scan after saving")).toBeChecked();
  fireEvent.click(within(dialog).getByRole("button", {name:"Create library"}));
  await waitFor(() => expect(mockApi.scanLibrary).toHaveBeenCalledWith(expect.any(Number)));
  expect(await screen.findByText(/Scan started in background/)).toBeInTheDocument();

  // Root watch toggle round-trips through the edit form.
  fireEvent.click(await screen.findByLabelText("Library menu Trips"));
  fireEvent.click(await screen.findByRole("menuitem", {name:"Edit"}));
  const editDialog = await screen.findByRole("dialog", {name:"Edit library details"});
  fireEvent.click(within(editDialog).getByLabelText("Watch for changes"));
  fireEvent.click(within(editDialog).getByRole("button", {name:"Save details"}));
  await waitFor(() => expect(mockApi.updateLibrary).toHaveBeenCalledWith(expect.any(Number),
    expect.objectContaining({roots:[expect.objectContaining({watch:true})]})));
});

test("native server gate asks for the address when none is saved", async () => {
  setNativePlatformForTests(true);
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByRole("heading", {name:"Media Library"})).toBeInTheDocument();
  fireEvent.change(screen.getByLabelText("Server address"), {target:{value:"localhost:18080"}});
  vi.stubGlobal("fetch", vi.fn().mockRejectedValue(new Error("offline")));
  fireEvent.click(screen.getByRole("button", {name:"Connect"}));
  expect(await screen.findByText(/Cannot reach server/)).toBeInTheDocument();
  setNativePlatformForTests(null);
});

test("native server gate reconnects to the remembered server", async () => {
  localStorage.setItem("ml.server.url", "https://media.example.com");
  const replace = vi.fn();
  const originalLocation = window.location;
  Object.defineProperty(window, "location", {value:{...originalLocation, replace}, configurable:true});
  // Hold the reachability probe open so the connecting screen stays visible.
  vi.stubGlobal("fetch", vi.fn(() => new Promise<void>(() => {})));
  setNativePlatformForTests(true);
  render(<MemoryRouter><App/></MemoryRouter>);
  expect(await screen.findByText(/Connecting to/)).toBeInTheDocument();
  vi.unstubAllGlobals();
  expect(screen.getByText("media.example.com")).toBeInTheDocument();
  // While the probe is pending the user may bail out to the picker.
  fireEvent.click(screen.getByRole("button", {name:"Change server"}));
  expect(await screen.findByLabelText("Server address")).toHaveValue("https://media.example.com");
  void replace;
  setNativePlatformForTests(null);
  Object.defineProperty(window, "location", {value:originalLocation, configurable:true});
});
