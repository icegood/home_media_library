import { FormEvent, MouseEvent, PointerEvent as ReactPointerEvent, SyntheticEvent, WheelEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Link, Navigate, Route, Routes, useLocation, useNavigate, useParams, useSearchParams } from "react-router-dom";
import L from "leaflet";
import { MapContainer, Marker, Popup, ScaleControl, TileLayer, useMap, useMapEvents } from "react-leaflet";
import { api, MAX_VIDEO_THUMBNAILS, type UserSettings as UserSettingsPayload } from "./api";
import type { EmbyImportResult, Entry, FavoriteView, FilesystemListing, ID, JobStatus, Library, LibraryUserAccess, LogTail, MapMedia, Media, MediaFolder, Role, ScheduledTask, User, VideoThumbnail } from "./types";

export function App() {
  const [user, setUser] = useState<User|null>(null);
  const [setupRequired, setSetupRequired] = useState(false);
  const [ready, setReady] = useState(false);
  const [theme, setTheme] = useState<"light"|"dark"|"system">("light");
  const [systemDark, setSystemDark] = useState(() => window.matchMedia?.("(prefers-color-scheme: dark)").matches ?? false);
  const [zoom, setZoom] = useState(100);
  const [userSettingsOpen, setUserSettingsOpen] = useState(false);
  const shellRef = useRef<HTMLDivElement|null>(null);
  useEffect(() => {
    api.setupStatus()
      .then(async status => {
        setSetupRequired(status.required);
        if (!status.required) setUser(await api.me().catch(() => null));
      })
      .finally(() => setReady(true));
  }, []);
  useEffect(() => {
    const media = window.matchMedia?.("(prefers-color-scheme: dark)");
    if (!media) return;
    const onChange = (event:MediaQueryListEvent) => setSystemDark(event.matches);
    media.addEventListener("change", onChange);
    return () => media.removeEventListener("change", onChange);
  }, []);
  const resolvedTheme = theme === "system" ? (systemDark ? "dark" : "light") : theme;
  useEffect(() => {
    document.documentElement.dataset.theme = resolvedTheme;
  }, [resolvedTheme]);
  useEffect(() => {
    document.documentElement.style.fontSize = `${zoom}%`;
  }, [zoom]);
  useEffect(() => {
    if (!user) return;
    api.userSettings().then(settings => { setTheme(settings.theme); setZoom(settings.zoom); syncUserDefaultThumbs(settings); }).catch(() => undefined);
  }, [user?.id]);
  useEffect(() => {
    function closeTopMenus(event:PointerEvent) {
      const target = event.target instanceof Node ? event.target : null;
      shellRef.current?.querySelectorAll<HTMLDetailsElement>("header details[open]").forEach(details => {
        if (!target || !details.contains(target)) details.removeAttribute("open");
      });
    }
    document.addEventListener("pointerdown", closeTopMenus);
    return () => document.removeEventListener("pointerdown", closeTopMenus);
  }, []);
  const crumbs = useBreadcrumb();
  const location = useLocation();
  const viewerMode = /^\/library\/[^/]+\/view\/[^/]+$/.test(location.pathname) || /^\/favorites\/view\/[^/]+$/.test(location.pathname);
  const [topMenuOpen, setTopMenuOpen] = useState(false);
  useEffect(() => {
    setTopMenuOpen(false);
  }, [viewerMode, location.pathname]);
  if (!ready) return <main className="center">Loading…</main>;
  if (setupRequired) return <FirstSetup onComplete={user => { setSetupRequired(false); setUser(user); }}/>;
  if (!user) return location.pathname.startsWith("/reset") ? <ResetPassword/> : <Login onLogin={setUser}/>;
  async function handleLogout() {
    await api.logout().catch(() => undefined);
    setUser(null);
  }
  function closeOtherTopMenus(event:SyntheticEvent<HTMLDetailsElement>) {
    const opened = event.currentTarget;
    if (!opened.open) return;
    shellRef.current?.querySelectorAll<HTMLDetailsElement>("header details[open]").forEach(details => {
      if (details !== opened) details.removeAttribute("open");
    });
  }
  return <div className={`shell ${viewerMode ? "viewer-shell" : ""} ${topMenuOpen ? "top-menu-open" : ""}`} ref={shellRef}>
    {viewerMode && <button type="button" className="top-menu-handle" aria-label={topMenuOpen ? "Hide main menu" : "Show main menu"} onClick={() => setTopMenuOpen(value => !value)}>{topMenuOpen ? "^^" : "vv"}</button>}
    <header>{crumbs ? <div className="brand header-crumbs" aria-label="Breadcrumb">{crumbs.map((crumb, index) => <span className="crumb" key={crumb.to ?? crumb.label}>{index > 0 && <span className="crumb-sep" aria-hidden="true"> / </span>}{crumb.current || !crumb.to ? <span className="crumb-current">{crumb.label}</span> : <Link to={crumb.to}>{crumb.label}</Link>}</span>)}</div> : <Link to="/" className="brand">Media Library</Link>}<nav><Link to="/">Library</Link><Link to="/favorites">Favorites</Link><Link to="/map">Map</Link>
      {user.role === "admin" && <details className="nav-menu" onPointerDown={closeOtherTopMenus} onToggle={closeOtherTopMenus}>
        <summary className="menu-trigger" aria-label="Admin panel menu">Admin panel</summary>
        <div className="nav-submenu" role="menu" onClick={closeParentDetails}>
          {settingsGroups.map(group => <div className="nav-submenu-group" key={group.label}>
            <span className="submenu-group-label">{group.label}</span>
            {group.items.map(item => <Link key={item.id} role="menuitem" to={`/admin${item.id === "libraries" ? "" : `?section=${item.id}`}`}>{item.label}</Link>)}
          </div>)}
        </div>
      </details>}</nav><details className="user-menu" onPointerDown={closeOtherTopMenus} onToggle={closeOtherTopMenus}>
        <summary className="menu-trigger" aria-label="User menu">{user.login}</summary>
        <div className="user-submenu" role="menu" onClick={closeParentDetails}>
          <button type="button" className="submenu-button" role="menuitem" onClick={() => setUserSettingsOpen(true)}>User settings</button>
          <button type="button" className="logout-button" role="menuitem" onClick={handleLogout}>Logout</button>
        </div>
      </details></header>
    {userSettingsOpen && <UserSettingsModal user={user} theme={theme} zoom={zoom} resolvedTheme={resolvedTheme} onThemeChange={setTheme} onZoomChange={setZoom} onUserChanged={setUser} onClose={() => setUserSettingsOpen(false)}/>}
    <Routes>
      <Route path="/" element={<Libraries/>}/>
      <Route path="/library/:id" element={<Browser/>}/>
      <Route path="/library/:id/folder/:folderId" element={<Browser/>}/>
      <Route path="/library/:id/timeline" element={<LibraryTimeline/>}/>
      <Route path="/library/:id/view/:folderId" element={<MediaViewerPage/>}/>
      <Route path="/favorites" element={<Favorites/>}/>
      <Route path="/favorites/:viewId" element={<FavoriteViewPage/>}/>
      <Route path="/favorites/view/:mediaId" element={<FavoriteMediaViewerPage/>}/>
      <Route path="/map" element={<GeoMap theme={resolvedTheme}/>}/>
      <Route path="/reset" element={<ResetPassword/>}/>
      <Route path="/admin" element={user.role === "admin" ? <AdminPanel/> : <Navigate to="/"/>}/>
      <Route path="/admin/settings" element={<Navigate to="/admin"/>}/>
      <Route path="*" element={<NotFound/>}/>
    </Routes>
  </div>;
}

function NotFound() {
  const location = useLocation();
  return <main className="center not-found" role="main">
    <div className="card settings">
      <p className="eyebrow">404</p>
      <h1>Page not found</h1>
      <p className="muted">No page exists for <code>{location.pathname}</code>.</p>
      <div className="action-row">
        <Link className="button-like active" to="/">Open libraries</Link>
      </div>
    </div>
  </main>;
}

interface Crumb { label:string; to:string|null; current?:boolean }

function folderCrumbName(folder:MediaFolder) {
  if (folder.name) return folder.name;
  const parts = folder.relativePath.split("/").filter(Boolean);
  return parts[parts.length - 1] || folder.relativePath || `Folder ${folder.id}`;
}

function useBreadcrumb(): Crumb[] | null {
  const location = useLocation();
  const [crumbs, setCrumbs] = useState<Crumb[]|null>(null);
  const pathname = location.pathname;
  useEffect(() => {
    let cancelled = false;
    setCrumbs(null);
    const favoritesMatch = pathname.match(/^\/favorites\/view\/[^/]+$/);
    if (favoritesMatch) {
      setCrumbs([{label:"Libraries", to:"/"}, {label:"Favorites", to:"/favorites"}]);
      return;
    }
    const rootMatch = pathname.match(/^\/library\/([^/]+)$/);
    const folderMatch = pathname.match(/^\/library\/([^/]+)\/folder\/([^/]+)$/);
    const viewerMatch = pathname.match(/^\/library\/([^/]+)\/view\/([^/]+)$/);
    const libraryID = rootMatch ? Number(rootMatch[1]) : folderMatch ? Number(folderMatch[1]) : viewerMatch ? Number(viewerMatch[1]) : NaN;
    if (!Number.isFinite(libraryID)) return;
    const folderID = folderMatch ? Number(folderMatch[2]) : viewerMatch ? Number(viewerMatch[2]) : null;
    (async () => {
      try {
        const libraries = await api.libraries();
        if (cancelled) return;
        const library = libraries.find(item => item.id === libraryID);
        const base: Crumb[] = [{label:"Libraries", to:"/"}];
        if (library) base.push({label:library.name, to:`/library/${libraryID}`});
        if (folderID != null && Number.isFinite(folderID)) {
          const chain: MediaFolder[] = [];
          const seen = new Set<number>();
          let currentID: number = folderID;
          while (currentID >= 0 && !seen.has(currentID)) {
            seen.add(currentID);
            const folder = await api.folder(libraryID, currentID);
            if (cancelled) return;
            chain.unshift(folder);
            currentID = folder.parentId;
          }
          base.push(...chain.map(folder => ({label:folderCrumbName(folder), to:`/library/${libraryID}/folder/${folder.id}`})));
        }
        if (cancelled) return;
        if ((rootMatch || folderMatch) && base.length > 0) base[base.length - 1] = {...base[base.length - 1], current:true};
        setCrumbs(base);
      } catch {
        // Leave null so the header falls back to the home brand.
      }
    })();
    return () => { cancelled = true; };
  }, [pathname]);
  return crumbs;
}

function closeParentDetails(event:MouseEvent<HTMLElement>) {
  event.currentTarget.closest("details")?.removeAttribute("open");
}

function closeOnBackdropClick(event:MouseEvent<HTMLElement>, close:()=>void) {
  if (event.target === event.currentTarget) close();
}

type SettingsSection = "network" | "mail" | "timeouts" | "thumbnails" | "libraries" | "users" | "jobs" | "scheduled" | "logs" | "emby";

const settingsSections: Record<SettingsSection, {label:string; description:string}> = {
  libraries: {label:"Libraries", description:"Add, rename, delete, scan, and refresh thumbnails"},
  users: {label:"Users", description:"Create users and manage roles"},
  thumbnails: {label:"Thumbnails", description:"Image and video thumbnail settings"},
  mail: {label:"Mail", description:"Outbound email for password resets"},
  network: {label:"Network", description:"HTTP, HTTPS, DNS, and certificates"},
  timeouts: {label:"Timeouts", description:"Login and background job retention timeouts"},
  jobs: {label:"Background jobs", description:"Track scans and thumbnail work"},
  scheduled: {label:"Scheduled tasks", description:"Automate scans, thumbnails, and maintenance"},
  logs: {label:"Logs", description:"Configure and view application logs"},
  emby: {label:"Emby import", description:"Import Emby data into the library"},
};

const settingsGroups: Array<{label:string; items:Array<{id:SettingsSection; label:string; description:string}>}> = [
  {label:"Media", items:(["libraries", "thumbnails"] as SettingsSection[]).map(settingsItem)},
  {label:"Access", items:(["users"] as SettingsSection[]).map(settingsItem)},
  {label:"System", items:(["network", "mail", "timeouts", "jobs", "scheduled", "logs"] as SettingsSection[]).map(settingsItem)},
  {label:"Import", items:(["emby"] as SettingsSection[]).map(settingsItem)},
];

function settingsItem(id:SettingsSection) {
  return {id, ...settingsSections[id]};
}

function AdminPanel() {
  const [searchParams, setSearchParams] = useSearchParams();
  const requestedSection = searchParams.get("section");
  const initialSection: SettingsSection = isSettingsSection(requestedSection) ? requestedSection : "libraries";
  const [section, setSection] = useState<SettingsSection>(initialSection);
  useEffect(() => {
    if (isSettingsSection(requestedSection)) {
      setSection(requestedSection);
    }
  }, [requestedSection]);
  function selectSection(next: SettingsSection) {
    setSection(next);
    setSearchParams(next === "libraries" ? {} : {section: next});
  }
  return <main><h1>Admin panel</h1><div className="settings-layout">
    <aside className="settings-menu" aria-label="Admin panel sections">
      <div className="settings-menu-header">
        <h2>Sections</h2>
        <p>Grouped by media, system, and import tasks</p>
      </div>
      {settingsGroups.map(group => <div className="settings-menu-group" key={group.label}>
        <h3>{group.label}</h3>
        {group.items.map(item => <button key={item.id} className={section === item.id ? "active" : ""} aria-label={item.label} onClick={() => selectSection(item.id)}>
          <span>{item.label}</span>
          <small>{item.description}</small>
        </button>)}
      </div>)}
    </aside>
    <section className="settings-content">
      <StopServerButton/>
      {section === "scheduled"
        ? <ScheduledTaskManager/>
        : section === "network" || section === "mail" || section === "timeouts" || section === "thumbnails" || section === "jobs"
          ? <AdminSettings section={section}/>
          : <LibraryManagement activeSection={section}/>}
    </section>
  </div></main>;
}

function StopServerButton() {
  const [stopping, setStopping] = useState<""|"docker"|"signal">("");
  const [error, setError] = useState("");
  async function stop(mode:"docker"|"signal") {
    const message = mode === "docker"
      ? "Stop the Docker container now? Restart it later with sh deploy/start.sh local-build or docker compose -f deploy/compose.yaml up -d api."
      : "Stop only the server process now? In Docker with restart policy, this can start again automatically.";
    if (!window.confirm(message)) return;
    setStopping(mode); setError("");
    try { await api.shutdown(mode); } catch (cause) { setError((cause as Error).message); setStopping(""); }
  }
  return <div className="settings-top-actions">
    <div>
      <strong>Server lifecycle</strong>
      <small>Docker stop is for Compose deployment. Process stop is for container-less deployment or restart testing.</small>
      {error && <p className="error">{error}</p>}
    </div>
    <div className="settings-stop-buttons">
      <button type="button" className="danger" disabled={stopping !== ""} onClick={() => void stop("docker")}>{stopping === "docker" ? "Stopping container…" : "Stop Docker container"}</button>
      <button type="button" className="secondary" disabled={stopping !== ""} onClick={() => void stop("signal")}>{stopping === "signal" ? "Stopping process…" : "Stop server process"}</button>
    </div>
  </div>;
}

function isSettingsSection(value:string|null): value is SettingsSection {
  return value === "network" || value === "mail" || value === "timeouts" || value === "thumbnails" || value === "libraries" || value === "users" || value === "jobs" || value === "scheduled" || value === "logs" || value === "emby";
}

function LibraryManagement({activeSection}:{activeSection:SettingsSection}) {
  const navigate = useNavigate();
  const [libraries, setLibraries] = useState<Library[]>([]);
  const [editing, setEditing] = useState<Library|null>(null);
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [roots, setRoots] = useState([{path:""}]);
  const [scanNow, setScanNow] = useState(true);
  const [pickingRoot, setPickingRoot] = useState<number|null>(null);
  const [filesystem, setFilesystem] = useState<FilesystemListing|null>(null);
  const [filesystemError, setFilesystemError] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [deleting, setDeleting] = useState<Library|null>(null);
  const [statsLibrary, setStatsLibrary] = useState<Library|null>(null);
  const [refreshingThumbnails, setRefreshingThumbnails] = useState<{id:ID; name:string}|null>(null);
  useEffect(() => { loadLibraries(); }, []);
  async function loadLibraries() {
    const items = await api.libraries();
    setLibraries(items);
  }
  function startAdd() {
    setAdding(true); setEditing(null); setName(""); setRoots([{path:""}]); setError(""); setNotice(""); setScanNow(true);
  }
  function startEdit(library:Library) {
    setEditing(library); setAdding(false); setName(library.name);
    setRoots((library.roots ?? []).map(root => ({path:root.path ?? ""})).concat((library.roots ?? []).length ? [] : [{path:""}]));
    setError(""); setNotice(""); setScanNow(false);
  }
  function closeModal() {
    setAdding(false); setEditing(null); setError(""); setNotice(""); closePicker();
  }
  function startDelete(library:Library) {
    setDeleting(library); setError(""); setNotice("");
  }
  function updateRoot(index:number, value:string) {
    setRoots(current => current.map((root, i) => i === index ? {...root, path:value} : root));
  }
  function addRoot() {
    setRoots(current => [...current, {path:""}]);
  }
  function removeRoot(index:number) {
    setRoots(current => current.filter((_, i) => i !== index));
  }
  async function openPicker(index:number, path = roots[index]?.path ?? "") {
    setPickingRoot(index); setFilesystemError("");
    try {
      setFilesystem(await api.filesystem(path));
    } catch (cause) {
      setFilesystem(null); setFilesystemError((cause as Error).message);
    }
  }
  function closePicker() {
    setPickingRoot(null); setFilesystem(null); setFilesystemError("");
  }
  function selectPickerPath(path:string) {
    if (pickingRoot == null) return;
    updateRoot(pickingRoot, path);
    closePicker();
  }
  async function libraryAction(action:"refresh"|"thumbs", library:Library, options:{recreateExisting?:boolean} = {}) {
    setBusy(true); setError(""); setNotice("");
    try {
      if (action === "refresh") await api.scanLibrary(library.id);
      else if (action === "thumbs") await api.createThumbnails(library.id, {recreateExisting: !!options.recreateExisting});
      setNotice(action === "refresh" ? "Scan started in background. Thumbnails will start after scan." :
        options.recreateExisting ? "Thumbnail recreation started in background." : "Thumbnail creation for missing thumbnails started in background.");
      setRefreshingThumbnails(null);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function confirmDelete() {
    if (!deleting) return;
    setBusy(true); setError("");
    try {
      await api.deleteLibrary(deleting.id);
      setNotice(`Library "${deleting.name}" deleted. Unshared items were removed from the database.`);
      setDeleting(null);
      await loadLibraries();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setError("");
    const cleanedRoots = roots.map(root => ({path:root.path.trim()})).filter(root => root.path);
    try {
      const library = editing
        ? await api.updateLibrary(editing.id, {name:name.trim(), roots:cleanedRoots})
        : await api.createLibrary({name:name.trim(), roots:cleanedRoots});
      if (scanNow) void api.scanLibrary(library.id).catch(cause => setError((cause as Error).message));
      await loadLibraries();
      closeModal();
      setNotice(scanNow ? "Library saved. Scan started in background; thumbnails will start after scan." : editing ? "Library saved." : "Library added.");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return <div className="card library-page">
    {activeSection === "libraries" && <>
      <div className="panel-title"><div><h2>Libraries</h2><p>Review your collections and open a library directly.</p></div><button onClick={startAdd}>Add</button></div>
      {notice && <p className="success">{notice}</p>}
      <div className="library-table">{libraries.map(library =>
        <div className="library-row admin-library" key={library.id}>
          <button className="library-glyph" aria-label={`Open library ${library.name}`} onClick={() => navigate(`/library/${library.id}`)}><span className="folder">▰</span></button>
          <button type="button" className="library-name-button" onClick={() => setStatsLibrary(library)}>
            <strong>{library.name}</strong>
            <small>{(library.roots ?? []).map(root => rootLabel(root.path)).join(", ") || "No roots"}</small>
          </button>
          <details className="item-menu">
            <summary aria-label={`Library menu ${library.name}`}>⋮</summary>
            <div className="item-submenu" role="menu" onClick={closeParentDetails}>
              <button type="button" role="menuitem" disabled={busy} onClick={() => startEdit(library)}>Rename</button>
              <button type="button" role="menuitem" disabled={busy} onClick={() => libraryAction("refresh", library)}>Refresh content</button>
              <button type="button" role="menuitem" disabled={busy} onClick={() => setRefreshingThumbnails({id:library.id, name:library.name})}>Refresh thumbnails…</button>
              <button type="button" role="menuitem" className="danger" disabled={busy} onClick={() => startDelete(library)}>Delete</button>
            </div>
          </details>
        </div>)}
        {libraries.length === 0 && <div className="empty-state"><h2>No libraries yet</h2><p>Use the Add button above to create the first library.</p></div>}
      </div>
    </>}
    {activeSection === "users" && <UserManagement/>}
    {activeSection === "logs" && <><AdminSettings section="logs"/><LogViewer/></>}
    {activeSection === "emby" && <EmbyImportPanel onImported={loadLibraries}/>} 
    {refreshingThumbnails && <ThumbnailRefreshModal title={refreshingThumbnails.name} busy={busy} error={error} onClose={() => setRefreshingThumbnails(null)} onRefresh={recreateExisting => libraryAction("thumbs", {id:refreshingThumbnails.id, name:refreshingThumbnails.name, roots:[]}, {recreateExisting})}/>}
    {statsLibrary && <LibraryStatsModal library={statsLibrary} onClose={() => setStatsLibrary(null)}/>}
    {deleting && <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Delete library ${deleting.name}`}>
      <div className="card settings modal">
        <div className="panel-title"><h2>Delete library</h2><button type="button" className="secondary" onClick={() => setDeleting(null)}>Close</button></div>
        <p>Delete <strong>{deleting.name}</strong>?</p>
        <p className="muted">Items and folders that are not reachable from other libraries will be removed from the database. Files on disk are not deleted.</p>
        {error && <p className="error">{error}</p>}
        <div className="action-row">
          <button type="button" className="danger" disabled={busy} onClick={confirmDelete}>Delete library</button>
          <button type="button" className="secondary" disabled={busy} onClick={() => setDeleting(null)}>Cancel</button>
        </div>
      </div>
    </div>}
    {(adding || editing) && <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={editing ? "Edit library details" : "Add library"} onClick={event => closeOnBackdropClick(event, closeModal)}>
      <form className="card settings modal" onSubmit={submit}>
        <div className="panel-title"><h2>{editing ? "Edit details" : "Add library"}</h2><button type="button" className="secondary" onClick={closeModal}>Close</button></div>
        <label>Library name<input value={name} onChange={event => setName(event.target.value)} placeholder="Family photos" required/></label>
        <div className="root-list">{roots.map((root, index) =>
          <div className="root-row" key={index}>
            <label>Root path<input value={root.path} onChange={event => updateRoot(index, event.target.value)} placeholder="family/photos" required/></label>
            <button type="button" className="secondary" onClick={() => openPicker(index)}>Browse</button>
            {roots.length > 1 && <button type="button" className="secondary" onClick={() => removeRoot(index)}>Remove</button>}
          </div>)}</div>
        <button type="button" className="secondary" onClick={addRoot}>Add root folder</button>
        <label className="check"><input type="checkbox" checked={scanNow} onChange={event => setScanNow(event.target.checked)}/> Scan after saving</label>
        {editing && <div className="action-row">
          <button type="button" className="secondary" disabled={busy} onClick={() => libraryAction("refresh", editing)}>Refresh content</button>
          <button type="button" className="secondary" disabled={busy} onClick={() => setRefreshingThumbnails({id:editing.id, name:editing.name})}>Refresh thumbnails…</button>
        </div>}
        {editing && <LibraryAccessEditor library={editing}/>}
        {error && <p className="error">{error}</p>}
        <button disabled={busy}>{busy ? "Saving…" : editing ? "Save details" : "Create library"}</button>
      </form>
      {pickingRoot != null && <DirectoryPickerModal title="Choose root folder" filesystem={filesystem} error={filesystemError} onOpen={path => openPicker(pickingRoot, path)} onSelect={selectPickerPath} onClose={closePicker}/>}
    </div>}
  </div>;
}

function DirectoryPickerModal({title,filesystem,error,onOpen,onSelect,onClose}:{title:string; filesystem:FilesystemListing|null; error:string; onOpen:(path:string)=>void; onSelect:(path:string)=>void; onClose:()=>void}) {
  return <div className="modal-backdrop nested" role="dialog" aria-modal="true" aria-label={title} onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal file-picker">
      <div className="panel-title"><div><h2>{title}</h2><p>Folders visible to the api process user</p></div><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      {error && <p className="error">{error}</p>}
      {filesystem && <>
        <div className="picker-path"><span>{filesystem.path}</span><button type="button" onClick={() => onSelect(filesystem.path)}>Select this folder</button></div>
        <div className="file-list">
          {filesystem.parent && <button type="button" className="file-row" onClick={() => onOpen(filesystem.parent)}>../</button>}
          {filesystem.directories.map(directory =>
            <div className="file-row" key={directory.path}>
              <button type="button" className="file-main" onClick={() => onOpen(directory.path)}>▰ {directory.name}</button>
              <button type="button" className="secondary" onClick={() => onSelect(directory.path)}>Select</button>
            </div>)}
          {filesystem.directories.length === 0 && <p className="muted">No subfolders here.</p>}
        </div>
      </>}
    </div>
  </div>;
}

function UserManagement() {
  const [users, setUsers] = useState<User[]>([]);
  const [login, setLogin] = useState("");
  const [role, setRole] = useState<Role>("regular");
  const [password, setPassword] = useState("");
  const [editing, setEditing] = useState<User|null>(null);
  const [editLogin, setEditLogin] = useState("");
  const [editRole, setEditRole] = useState<Role>("regular");
  const [editPassword, setEditPassword] = useState("");
  const [accessUser, setAccessUser] = useState<User|null>(null);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  function load() {
    api.users().then(setUsers).catch(cause => setError((cause as Error).message));
  }
  useEffect(load, []);
  function startEdit(user:User) {
    setEditing(user); setEditLogin(user.login); setEditRole(user.role); setEditPassword(""); setError("");
  }
  async function create(event:FormEvent) {
    event.preventDefault();
    setBusy(true); setError("");
    try {
      await api.createUser({login, role, password});
      setLogin(""); setPassword(""); setRole("regular");
      load();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function saveEdit(event:FormEvent) {
    event.preventDefault();
    if (!editing) return;
    setBusy(true); setError("");
    try {
      await api.updateUser(editing.id, {login: editLogin, role: editRole, password: editPassword || undefined});
      setEditing(null);
      load();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return <div className="user-admin">
    <div className="panel-title"><div><h2>Users</h2><p>Create users and assign admin or regular role. Library read access is edited inside each library.</p></div></div>
    <form className="card settings inline-user-form" onSubmit={create}>
      <label>Login<input value={login} onChange={event => setLogin(event.target.value)} minLength={3} required/></label>
      <label>Role<select value={role} onChange={event => setRole(event.target.value as Role)}><option value="regular">Regular</option><option value="admin">Admin</option></select></label>
      <label>Password<input type="password" value={password} onChange={event => setPassword(event.target.value)} minLength={12} required/></label>
      <button disabled={busy}>Add user</button>
    </form>
    {error && <p className="error">{error}</p>}
    <div className="library-table">{users.map(user => <div className="library-row" key={user.id}>
      <div className="library-main"><span className="folder">👤</span><span><strong>{user.login}</strong><small>{user.role}{user.role === "admin" ? " · reads every library" : ""}</small></span></div>
      <button type="button" className="secondary" disabled={busy || user.role === "admin"} onClick={() => setAccessUser(user)}>Manage access</button>
      <button type="button" className="secondary" disabled={busy} onClick={() => startEdit(user)}>Edit</button>
    </div>)}</div>
    {accessUser && <UserAccessEditor user={accessUser} onClose={() => setAccessUser(null)}/>}
    {editing && <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Edit user ${editing.login}`} onClick={event => closeOnBackdropClick(event, () => setEditing(null))}>
      <form className="card settings modal" onSubmit={saveEdit}>
        <div className="panel-title"><h2>Edit user</h2><button type="button" className="secondary" onClick={() => setEditing(null)}>Close</button></div>
        <label>Login<input value={editLogin} onChange={event => setEditLogin(event.target.value)} minLength={3} required/></label>
        <label>Role<select value={editRole} onChange={event => setEditRole(event.target.value as Role)}><option value="regular">Regular</option><option value="admin">Admin</option></select></label>
        <label>New password<input type="password" value={editPassword} onChange={event => setEditPassword(event.target.value)} minLength={12} placeholder="Leave empty to keep current password"/></label>
        <button disabled={busy}>{busy ? "Saving…" : "Save user"}</button>
      </form>
    </div>}
  </div>;
}

function UserAccessEditor({user,onClose}:{user:User; onClose:()=>void}) {
  const [libraries, setLibraries] = useState<Library[]>([]);
  const [access, setAccess] = useState<Record<ID,boolean>>({});
  const [busyLibrary, setBusyLibrary] = useState<ID|null>(null);
  const [error, setError] = useState("");
  useEffect(() => {
    api.libraries().then(setLibraries).catch(cause => setError((cause as Error).message));
  }, []);
  useEffect(() => {
    if (libraries.length === 0) return;
    let cancelled = false;
    Promise.all(libraries.map(library => api.libraryAccess(library.id).then(rows => ({id:library.id, allowed: rows.find(row => row.user.id === user.id)?.allowed ?? false}))))
      .then(results => { if (!cancelled) setAccess(Object.fromEntries(results.map(result => [result.id, result.allowed]))); })
      .catch(cause => { if (!cancelled) setError((cause as Error).message); });
    return () => { cancelled = true; };
  }, [libraries, user.id]);
  async function toggle(libraryId:ID, allowed:boolean) {
    setBusyLibrary(libraryId); setError("");
    setAccess(current => ({...current, [libraryId]:allowed}));
    try {
      await api.setLibraryAccess(libraryId, user.id, allowed);
    } catch (cause) {
      setError((cause as Error).message);
      setAccess(current => ({...current, [libraryId]:!allowed}));
    } finally {
      setBusyLibrary(null);
    }
  }
  return <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Manage access for ${user.login}`} onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal">
      <div className="panel-title"><h2>Library access</h2><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      <p>Grant <strong>{user.login}</strong> read access to libraries.</p>
      {error && <p className="error">{error}</p>}
      {libraries.length === 0 ? <p className="muted">No libraries yet.</p> :
        <fieldset className="library-access"><legend>Read access</legend>
          {libraries.map(library => <label className="check" key={library.id}>
            <input type="checkbox" checked={access[library.id] ?? false} disabled={busyLibrary === library.id} onChange={event => void toggle(library.id, event.target.checked)}/>
            {library.name}
          </label>)}
        </fieldset>}
    </div>
  </div>;
}

function LibraryAccessEditor({library}:{library:Library}) {
  const [access, setAccess] = useState<LibraryUserAccess[]>([]);
  const [error, setError] = useState("");
  const [busyUser, setBusyUser] = useState<ID|null>(null);
  function load() {
    api.libraryAccess(library.id).then(setAccess).catch(cause => setError((cause as Error).message));
  }
  useEffect(load, [library.id]);
  async function toggle(item:LibraryUserAccess, allowed:boolean) {
    if (item.user.role === "admin") return;
    setBusyUser(item.user.id); setError("");
    setAccess(current => current.map(row => row.user.id === item.user.id ? {...row, allowed} : row));
    try {
      await api.setLibraryAccess(library.id, item.user.id, allowed);
    } catch (cause) {
      setError((cause as Error).message);
      load();
    } finally {
      setBusyUser(null);
    }
  }
  return <fieldset className="library-access"><legend>Read access</legend>
    <small>Admins always read every library. Enable regular users that should see this library.</small>
    {error && <p className="error">{error}</p>}
    {access.length === 0 ? <p className="muted">No users yet.</p> : access.map(item => <label className="check" key={item.user.id}>
      <input type="checkbox" checked={item.allowed} disabled={item.user.role === "admin" || busyUser === item.user.id} onChange={event => void toggle(item, event.target.checked)}/>
      {item.user.login} <span className="muted">({item.user.role}{item.user.role === "admin" ? ", always allowed" : ""})</span>
    </label>)}
  </fieldset>;
}

function JobMonitor() {
  const [jobs, setJobs] = useState<JobStatus[]>([]);
  const [error, setError] = useState("");
  async function control(id:string, action:"pause"|"resume"|"cancel") {
    try {
      const updated = action === "pause" ? await api.pauseJob(id) : action === "resume" ? await api.resumeJob(id) : await api.cancelJob(id);
      setJobs(current => current.map(job => job.id === id ? updated : job));
    } catch (cause) {
      setError((cause as Error).message);
    }
  }
  useEffect(() => {
    let cancelled = false;
    async function load() {
      try {
        const items = await api.jobs();
        if (!cancelled) {
          setJobs(items);
          setError("");
        }
      } catch (cause) {
        if (!cancelled) setError((cause as Error).message);
      }
    }
    void load();
    const interval = window.setInterval(load, 1500);
    return () => { cancelled = true; window.clearInterval(interval); };
  }, []);
  const categories = ["scan", "thumbnail-create"];
  const activeJobs = jobs.filter(isActiveJob);
  const completedJobs = jobs.filter(job => !isActiveJob(job)).slice(0, 10);
  return <section className="jobs-panel">
    <div className="panel-title"><h2>Job instances</h2><small>{activeJobs.length} active instances</small></div>
    {error && <p className="error">{error}</p>}
    {jobs.length === 0 && <p className="muted">No jobs yet.</p>}
    {activeJobs.length === 0 && jobs.length > 0 && <p className="muted">No active jobs. Completed jobs are shown below for a short time.</p>}
    {activeJobs.length > 0 && <div className="job-categories">{categories.map(category => {
      const instances = activeJobs.filter(job => jobCategory(job) === category);
      return <section className="job-category" key={category}>
        <div className="job-category-title"><h3>{categoryLabel(category)}</h3><small>{instances.length} active</small></div>
        {instances.length === 0 && <p className="muted">No instances.</p>}
        <div className="jobs-list">{instances.map(job => <JobInstanceRow job={job} key={job.id} onControl={control}/>)}</div>
      </section>;
    })}</div>}
    {completedJobs.length > 0 && <details className="completed-jobs">
      <summary>Recent completed jobs ({completedJobs.length})</summary>
      <div className="jobs-list">{completedJobs.map(job => <JobInstanceRow job={job} key={job.id} onControl={control}/>)}</div>
    </details>}
  </section>;
}

function isActiveJob(job:JobStatus) {
  return job.status === "running" || job.status === "paused" || job.status === "cancelling";
}

function JobInstanceRow({job,onControl}:{job:JobStatus; onControl:(id:string, action:"pause"|"resume"|"cancel")=>void}) {
  const percent = job.total > 0 ? Math.min(100, Math.round(job.processed * 100 / job.total)) : 0;
  return <article className="job-row">
    <div><strong>{job.libraryName}</strong><small>{job.status} · instance {job.id.slice(0, 8)}</small></div>
    <div className="job-progress"><span style={{width:`${percent}%`}}/></div>
    <small>{job.total > 0 ? `${job.processed}/${job.total}` : `${job.processed} paths`} · {job.currentPath || job.rootPath || "no root"}</small>
    {job.cancelable && <div className="job-controls">
      {job.paused || job.status === "paused"
        ? <button type="button" className="secondary" onClick={() => onControl(job.id, "resume")}>Resume</button>
        : <button type="button" className="secondary" onClick={() => onControl(job.id, "pause")}>Pause</button>}
      <button type="button" className="secondary danger" onClick={() => onControl(job.id, "cancel")}>Cancel</button>
    </div>}
    {job.error && <p className="error">{job.error}</p>}
  </article>;
}

function jobCategory(job:JobStatus) {
  return job.category ?? job.type ?? "unknown";
}

function categoryLabel(category:string) {
  if (category === "thumbnail-create") return "Thumbnail create";
  if (category === "scan") return "Scan";
  return category;
}

function ScheduledTaskManager() {
  const [tasks, setTasks] = useState<ScheduledTask[]>([]);
  const [libraries, setLibraries] = useState<Library[]>([]);
  const [editing, setEditing] = useState<ScheduledTask|null>(null);
  const [adding, setAdding] = useState(false);
  const [name, setName] = useState("");
  const [taskType, setTaskType] = useState<"scan"|"thumbnail-create"|"vacuum">("scan");
  const [libraryId, setLibraryId] = useState<ID>(0);
  const [cron, setCron] = useState("0 3 * * *");
  const [enabled, setEnabled] = useState(true);
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  function load() {
    api.scheduledTasks().then(setTasks).catch(cause => setError((cause as Error).message));
  }
  useEffect(() => {
    load();
    api.libraries().then(setLibraries).catch(() => undefined);
  }, []);
  function startAdd() {
    setAdding(true); setEditing(null); setName(""); setTaskType("scan"); setLibraryId(libraries[0]?.id ?? 0); setCron("0 3 * * *"); setEnabled(true); setError(""); setNotice("");
  }
  function startEdit(task:ScheduledTask) {
    setEditing(task); setAdding(false); setName(task.name); setTaskType(task.taskType); setLibraryId(task.libraryId); setCron(task.cron); setEnabled(task.enabled); setError(""); setNotice("");
  }
  function closeModal() {
    setAdding(false); setEditing(null); setError(""); setNotice("");
  }
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setError(""); setNotice("");
    try {
      const input = {name:name.trim(), taskType, libraryId: taskType === "vacuum" ? 0 : libraryId, cron:cron.trim(), enabled};
      if (editing) {
        await api.updateScheduledTask(editing.id, input);
      } else {
        await api.createScheduledTask(input);
      }
      closeModal();
      load();
      setNotice(editing ? "Scheduled task updated." : "Scheduled task created.");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function remove(task:ScheduledTask) {
    if (!window.confirm(`Delete scheduled task "${task.name}"?`)) return;
    setBusy(true); setError("");
    try {
      await api.deleteScheduledTask(task.id);
      load();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  async function toggleEnabled(task:ScheduledTask) {
    setBusy(true); setError("");
    try {
      await api.updateScheduledTask(task.id, {name:task.name, taskType:task.taskType, libraryId:task.libraryId, cron:task.cron, enabled:!task.enabled});
      load();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  const libraryName = (id:ID) => libraries.find(library => library.id === id)?.name ?? `Library ${id}`;
  return <div className="card settings scheduled-tasks">
    <div className="panel-title"><div><h2>Scheduled tasks</h2><p>Automate library scans, thumbnail creation, and database maintenance.</p></div><button onClick={startAdd}>Add</button></div>
    {notice && <p className="success">{notice}</p>}
    {error && <p className="error">{error}</p>}
    {tasks.length === 0 ? <div className="empty-state"><p>No scheduled tasks yet. Use the Add button to create one.</p></div> :
      <div className="library-table">{tasks.map(task =>
        <div className="library-row" key={task.id}>
          <div className="library-main"><span className="folder">⏰</span><span><strong>{task.name}</strong><small>{task.taskType}{task.taskType !== "vacuum" ? ` · ${libraryName(task.libraryId)}` : ""} · {task.cron} · next {task.nextRunAt ? new Date(task.nextRunAt).toLocaleString() : "—"}</small></span></div>
          <label className="check"><input type="checkbox" checked={task.enabled} disabled={busy} onChange={() => void toggleEnabled(task)}/> Enabled</label>
          <button type="button" className="secondary" disabled={busy} onClick={() => startEdit(task)}>Edit</button>
          <button type="button" className="secondary danger" disabled={busy} onClick={() => void remove(task)}>Delete</button>
        </div>)}</div>}
    {(adding || editing) && <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={editing ? "Edit scheduled task" : "Add scheduled task"} onClick={event => closeOnBackdropClick(event, closeModal)}>
      <form className="card settings modal" onSubmit={submit}>
        <div className="panel-title"><h2>{editing ? "Edit scheduled task" : "Add scheduled task"}</h2><button type="button" className="secondary" onClick={closeModal}>Close</button></div>
        <label>Name<input value={name} onChange={event => setName(event.target.value)} placeholder="Nightly scan" required/></label>
        <label>Task type<select value={taskType} onChange={event => { setTaskType(event.target.value as typeof taskType); if (event.target.value === "vacuum") setLibraryId(0); }}>
          <option value="scan">Scan / refresh content</option>
          <option value="thumbnail-create">Create thumbnails</option>
          <option value="vacuum">Vacuum database</option>
        </select></label>
        {taskType !== "vacuum" && <label>Library<select value={libraryId} onChange={event => setLibraryId(Number(event.target.value))} required>
          {libraries.map(library => <option key={library.id} value={library.id}>{library.name}</option>)}
        </select></label>}
        <label>Cron expression<input value={cron} onChange={event => setCron(event.target.value)} placeholder="0 3 * * *" required/></label>
        <small>Format: minute hour day-of-month month day-of-week. Example: <code>0 3 * * *</code> runs daily at 03:00.</small>
        <label className="check"><input type="checkbox" checked={enabled} onChange={event => setEnabled(event.target.checked)}/> Enabled</label>
        {error && <p className="error">{error}</p>}
        <button disabled={busy}>{busy ? "Saving…" : editing ? "Save task" : "Create task"}</button>
      </form>
    </div>}
  </div>;
}

function LogViewer() {
  const [tail, setTail] = useState<LogTail>({path:"", lines:[]});
  const [limit, setLimit] = useState(300);
  const [loading, setLoading] = useState(false);
  const [clearing, setClearing] = useState(false);
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  async function load() {
    setLoading(true); setError("");
    try {
      setTail(await api.logs(limit));
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setLoading(false);
    }
  }
  async   function clear() {
    if (!window.confirm("Clear the application log file? This cannot be undone.")) return;
    setClearing(true); setError(""); setMessage("");
    try {
      const result = await api.clearLogs();
      setTail({path:result.path, lines:[]});
      setMessage("Logs cleared.");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setClearing(false);
    }
  }
  function download() {
    const anchor = document.createElement("a");
    anchor.href = api.logsDownloadUrl();
    anchor.download = "app.log";
    document.body.appendChild(anchor);
    anchor.click();
    anchor.remove();
  }
  useEffect(() => { void load(); }, []);
  return <section className="logs-panel">
    <div className="panel-title">
      <div><h2>Logs</h2><p>Recent application log lines from the API container.</p></div>
      <button type="button" className="secondary" disabled={loading} onClick={load}>{loading ? "Refreshing…" : "Refresh"}</button>
      <button type="button" className="secondary" disabled={tail.lines.length === 0 && !tail.path} onClick={download}>Download logs</button>
      <button type="button" className="danger secondary" disabled={clearing} onClick={clear}>{clearing ? "Clearing…" : "Clear logs"}</button>
    </div>
    <div className="logs-toolbar">
      <label>Lines<input type="number" min="1" max="2000" value={limit} onChange={event => setLimit(Number(event.target.value))}/></label>
      {tail.path && <small>File: <code>{tail.path}</code></small>}
    </div>
    {error && <p className="error">{error}</p>}
    {message && <p className="success">{message}</p>}
    {tail.lines.length === 0 && !error && <p className="muted">No log lines yet.</p>}
    {tail.lines.length > 0 && <pre className="log-output" aria-label="Application logs">{tail.lines.join("\n")}</pre>}
  </section>;
}

function EmbyImportPanel({onImported}:{onImported:()=>Promise<void>}) {
  const [configRoot, setConfigRoot] = useState("");
  const [pathMappings, setPathMappings] = useState([{from:"", to:""}]);
  const [picking, setPicking] = useState<"configRoot"|{mappingIndex:number}|null>(null);
  const [filesystem, setFilesystem] = useState<FilesystemListing|null>(null);
  const [filesystemError, setFilesystemError] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [result, setResult] = useState<EmbyImportResult|null>(null);
  function updateMapping(index:number, key:"from"|"to", value:string) {
    setPathMappings(current => current.map((mapping, i) => i === index ? {...mapping, [key]:value} : mapping));
  }
  async function openPicker(target:"configRoot"|{mappingIndex:number}, path = "") {
    setPicking(target); setFilesystemError("");
    const currentPath = path || (target === "configRoot" ? configRoot : pathMappings[target.mappingIndex]?.to) || "";
    try {
      setFilesystem(await api.filesystem(currentPath));
    } catch (cause) {
      setFilesystem(null); setFilesystemError((cause as Error).message);
    }
  }
  function closePicker() {
    setPicking(null); setFilesystem(null); setFilesystemError("");
  }
  function selectPickerPath(path:string) {
    if (!picking) return;
    if (picking === "configRoot") setConfigRoot(path);
    else updateMapping(picking.mappingIndex, "to", path);
    closePicker();
  }
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setBusy(true); setError(""); setResult(null);
    try {
      const imported = await api.importEmby({
        configRoot: configRoot.trim(),
        pathMappings: pathMappings.filter(mapping => mapping.from.trim() && mapping.to.trim()).map(mapping => ({from:mapping.from.trim(), to:mapping.to.trim()}))
      });
      setResult(imported);
      await onImported();
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return <form className="card settings emby-import" onSubmit={submit}>
    <h2>Emby import</h2>
    <p className="muted">Imports Emby libraries, users, passwords, and library-user read links. Mount the Emby config root into the API container and enter that one root folder. The importer reads <code>data/library.db</code>, <code>data/users.db</code>, and <code>config/users</code> below it. Legacy Emby SHA1 passwords are upgraded to bcrypt after each user’s first successful login.</p>
    <div className="root-row">
      <label>Emby config root inside API container<input value={configRoot} readOnly placeholder="/path/in/container/emby_config" required/></label>
      <button type="button" className="secondary" onClick={() => openPicker("configRoot")}>Browse</button>
    </div>
    <fieldset><legend>Path mappings</legend>
      {pathMappings.map((mapping, index) => <div className="root-row" key={index}>
        <label>Emby path<input value={mapping.from} onChange={event => updateMapping(index, "from", event.target.value)} placeholder="Path stored in Emby"/></label>
        <label>Docker path<input value={mapping.to} readOnly placeholder="Same folder path inside this API container"/></label>
        <button type="button" className="secondary" onClick={() => openPicker({mappingIndex:index})}>Browse</button>
      </div>)}
      <button type="button" className="secondary" onClick={() => setPathMappings(current => [...current, {from:"", to:""}])}>Add mapping</button>
    </fieldset>
    {error && <p className="error">{error}</p>}
    {result && <div className="import-result">
      <p className="success">Imported {result.libraries.length} libraries, {result.users.length} users, {result.access.length} access links.</p>
      {result.users.some(user => user.temporaryPassword) && <details open><summary>Temporary passwords for users without importable Emby passwords</summary>
        <ul>{result.users.filter(user => user.temporaryPassword).map(user =>
          <li key={user.user.id}><code>{user.user.login}</code>: <code>{user.temporaryPassword}</code></li>)}</ul>
      </details>}
    </div>}
    <button disabled={busy}>{busy ? "Importing…" : "Import from Emby"}</button>
    {picking && <DirectoryPickerModal title={picking === "configRoot" ? "Choose Emby config root" : "Choose Docker target folder"} filesystem={filesystem} error={filesystemError} onOpen={path => openPicker(picking, path)} onSelect={selectPickerPath} onClose={closePicker}/>}
  </form>;
}

function rootLabel(value = "") {
  const cleaned = value.replace(/\/+$/, "");
  if (!cleaned || cleaned === ".") return "root";
  return cleaned.split("/").pop() ?? cleaned;
}

function AdminSettings({section}:{section:"network"|"mail"|"timeouts"|"thumbnails"|"jobs"|"logs"}) {
  const [httpEnabled, setHTTPEnabled] = useState(true);
  const [httpsEnabled, setHTTPSEnabled] = useState(false);
  const [publicDns, setPublicDns] = useState("");
  const [acmeEmail, setAcmeEmail] = useState("");
  const [httpsCertificateExpiresAt, setHTTPSCertificateExpiresAt] = useState("");
  const [thumbnailWidth, setThumbnailWidth] = useState(480);
  const [thumbnailHeight, setThumbnailHeight] = useState(360);
  const [videoThumbnailFirstSeconds, setVideoThumbnailFirstSeconds] = useState(5);
  const [videoThumbnailMaxCount, setVideoThumbnailMaxCount] = useState(MAX_VIDEO_THUMBNAILS);
  const [videoThumbnailMinIntervalSeconds, setVideoThumbnailMinIntervalSeconds] = useState(120);
  const [workerPoolSize, setWorkerPoolSize] = useState(4);
  const [sessionMaxAgeHours, setSessionMaxAgeHours] = useState(720);
  const [finishedJobRetentionMinutes, setFinishedJobRetentionMinutes] = useState(10);
  const [logLevel, setLogLevel] = useState<"D"|"I"|"W"|"E">("I");
  const [logRotateMaxSizeMB, setLogRotateMaxSizeMB] = useState(10);
  const [logRotateMaxBackups, setLogRotateMaxBackups] = useState(5);
  const [logRotateMaxAgeDays, setLogRotateMaxAgeDays] = useState(30);
  const [smtpHost, setSMTPHost] = useState("");
  const [smtpPort, setSMTPPort] = useState(587);
  const [smtpUsername, setSMTPUsername] = useState("");
  const [smtpPassword, setSMTPPassword] = useState("");
  const [smtpFrom, setSMTPFrom] = useState("");
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const [httpsGatewayEnabled, setHTTPSGatewayEnabled] = useState(false);
  useEffect(() => { api.settings().then(value => {
    setHTTPEnabled(value.httpEnabled); setHTTPSEnabled(value.httpsEnabled);
    setPublicDns(value.publicDns); setAcmeEmail(value.acmeEmail);
    setHTTPSCertificateExpiresAt(value.httpsCertificateExpiresAt);
    setHTTPSGatewayEnabled(value.httpsGatewayEnabled);
    setThumbnailWidth(value.thumbnailWidth); setThumbnailHeight(value.thumbnailHeight);
    setVideoThumbnailFirstSeconds(value.videoThumbnailFirstSeconds);
    setVideoThumbnailMaxCount(value.videoThumbnailMaxCount);
    setVideoThumbnailMinIntervalSeconds(value.videoThumbnailMinIntervalSeconds);
    setWorkerPoolSize(value.workerPoolSize);
    setSessionMaxAgeHours(value.sessionMaxAgeHours);
    setFinishedJobRetentionMinutes(value.finishedJobRetentionMinutes);
    setLogLevel(value.logLevel);
    setLogRotateMaxSizeMB(value.logRotateMaxSizeMB);
    setLogRotateMaxBackups(value.logRotateMaxBackups);
    setLogRotateMaxAgeDays(value.logRotateMaxAgeDays);
    setSMTPHost(value.smtpHost); setSMTPPort(value.smtpPort);
    setSMTPUsername(value.smtpUsername); setSMTPFrom(value.smtpFrom);
    setLoaded(true);
  }).catch(cause => setError((cause as Error).message)); }, []);
  useEffect(() => {
    if (!loaded) return;
    let cancelled = false;
    setSaving(true); setSaved(false); setError("");
    const timeout = window.setTimeout(() => {
      api.updateSettings({httpEnabled,httpsEnabled,publicDns,acmeEmail,httpsGatewayEnabled,httpsCertificateExpiresAt,thumbnailWidth,thumbnailHeight,videoThumbnailFirstSeconds,videoThumbnailMaxCount,videoThumbnailMinIntervalSeconds,workerPoolSize,sessionMaxAgeHours,finishedJobRetentionMinutes,logLevel,logRotateMaxSizeMB,logRotateMaxBackups,logRotateMaxAgeDays,smtpHost,smtpPort,smtpUsername,smtpFrom,smtpPassword:smtpPassword || undefined})
        .then(value => { if (!cancelled) { setHTTPSCertificateExpiresAt(value.httpsCertificateExpiresAt); setSaved(true); } })
        .catch(cause => { if (!cancelled) setError((cause as Error).message); })
        .finally(() => { if (!cancelled) setSaving(false); });
    }, 600);
    return () => { cancelled = true; window.clearTimeout(timeout); };
  }, [loaded,httpEnabled,httpsEnabled,publicDns,acmeEmail,thumbnailWidth,thumbnailHeight,videoThumbnailFirstSeconds,videoThumbnailMaxCount,videoThumbnailMinIntervalSeconds,workerPoolSize,sessionMaxAgeHours,finishedJobRetentionMinutes,logLevel,logRotateMaxSizeMB,logRotateMaxBackups,logRotateMaxAgeDays,smtpHost,smtpPort,smtpUsername,smtpPassword,smtpFrom]);
  return <div className="card settings">
    <h2>{settingsSectionTitle(section)}</h2>
    {!loaded ? <p className="muted">Loading settings…</p> : <>
    {section === "network" && <fieldset><legend>Network</legend>
      <label className="check"><input type="checkbox" checked={httpEnabled} onChange={event => setHTTPEnabled(event.target.checked)}/> Enable HTTP</label>
      {httpsGatewayEnabled ? <>
        <label className="check"><input type="checkbox" checked={httpsEnabled} onChange={event => setHTTPSEnabled(event.target.checked)}/> Enable HTTPS with Let’s Encrypt</label>
        {httpsEnabled && <>
          <label>Public DNS name<input value={publicDns} onChange={event => setPublicDns(event.target.value)} placeholder="media.example.com" required/></label>
          <label>Let’s Encrypt email<input type="email" value={acmeEmail} onChange={event => setAcmeEmail(event.target.value)} required/></label>
          <label>Certificate expires<input value={httpsCertificateExpiresAt || "Not installed yet"} readOnly aria-readonly="true"/></label>
          <details className="help-box"><summary>How to prepare your domain for Let’s Encrypt</summary>
            <p>Let’s Encrypt must be able to reach your host on public ports 80 and 443 at the name you enter, or no certificate is issued. Choose the way that fits your setup:</p>
            <ul>
              <li><strong>Public domain (recommended):</strong> point the domain’s A/AAAA record at your public IP and forward TCP 80 and 443 to this host.</li>
              <li><strong>Tailscale:</strong> a <code>*.ts.net</code> name can never get a Let’s Encrypt certificate. Keep HTTP enabled, leave HTTPS off, and expose the app with <code>tailscale serve</code> or <code>tailscale funnel</code> — Tailscale provides the certificate itself.</li>
              <li><strong>Tunnel / CDN:</strong> put Cloudflare Tunnel (or similar) in front of HTTP mode; the tunnel terminates TLS, so no ports need to be opened.</li>
              <li><strong>Own reverse proxy:</strong> if you already run a public proxy or VPS, serve HTTPS there and keep the app in HTTP mode.</li>
            </ul>
            <small>Full guide with step-by-step commands: <code>docs/https-domain-setup.md</code> in the repository.</small>
          </details>
        </>}
        <small>At least one protocol must remain enabled. HTTPS changes are applied automatically.</small>
      </> : <small>The optional gateway container is not enabled in this deployment, so the app serves plain HTTP only. To make HTTPS available, set <code>COMPOSE_PROFILES=https</code> in <code>deploy/.env</code> and restart the stack.</small>}
    </fieldset>}
    {section === "mail" && <fieldset><legend>Outbound email</legend>
      <label>SMTP host<input value={smtpHost} onChange={event => setSMTPHost(event.target.value)} placeholder="smtp.example.com"/></label>
      <label>SMTP port<input type="number" min="1" max="65535" value={smtpPort} onChange={event => setSMTPPort(Number(event.target.value))} required/></label>
      <label>SMTP username<input value={smtpUsername} onChange={event => setSMTPUsername(event.target.value)} autoComplete="off"/></label>
      <label>SMTP password<input type="password" value={smtpPassword} onChange={event => setSMTPPassword(event.target.value)} autoComplete="new-password" placeholder={smtpPassword ? "" : "Unchanged"} onFocus={event => { if (!smtpPassword) event.currentTarget.value = ""; }}/></label>
      <label>From address<input type="email" value={smtpFrom} onChange={event => setSMTPFrom(event.target.value)} placeholder="media@example.com"/></label>
      <small>Used to send password reset links. Leave the host empty to disable outbound email. Port 465 uses implicit TLS; other ports use STARTTLS.</small>
    </fieldset>}
    {section === "thumbnails" && <><fieldset><legend>Image thumbnails</legend>
      <label>Width<input type="number" min="64" max="4096" value={thumbnailWidth} onChange={event => setThumbnailWidth(Number(event.target.value))} required/></label>
      <label>Height<input type="number" min="64" max="4096" value={thumbnailHeight} onChange={event => setThumbnailHeight(Number(event.target.value))} required/></label>
      <small>Global thumbnail tile size. Existing thumbnail files are regenerated after cleanup or refresh.</small>
    </fieldset>
    <fieldset><legend>Video thumbnails</legend>
      <label>First thumbnail, seconds<input type="number" min="0" value={videoThumbnailFirstSeconds} onChange={event => setVideoThumbnailFirstSeconds(Number(event.target.value))} required/></label>
      <label>Max thumbnails<input type="number" min="1" max={MAX_VIDEO_THUMBNAILS} value={videoThumbnailMaxCount} onChange={event => setVideoThumbnailMaxCount(Number(event.target.value))} required/></label>
      <label>Minimum interval, seconds<input type="number" min="1" value={videoThumbnailMinIntervalSeconds} onChange={event => setVideoThumbnailMinIntervalSeconds(Number(event.target.value))} required/></label>
      <small>Thumbnails are capped by max count and never closer than the minimum interval.</small>
    </fieldset></>}
    {section === "jobs" && <><fieldset><legend>Job worker pool</legend>
      <label>Worker pool size<input type="number" min="1" max="64" value={workerPoolSize} onChange={event => setWorkerPoolSize(Number(event.target.value))} required/></label>
      <small>Shared parallel worker count for scan/import and thumbnail creation jobs.</small>
    </fieldset><JobMonitor/></>}
    {section === "timeouts" && <><fieldset><legend>Login timeout</legend>
      <label>Remember login, hours<input type="number" min="1" max="8760" value={sessionMaxAgeHours} onChange={event => setSessionMaxAgeHours(Number(event.target.value))} required/></label>
      <small>Controls the HttpOnly login cookie and JWT expiration. Default is 720 hours, about 30 days.</small>
    </fieldset>
    <fieldset><legend>Background job timeout</legend>
      <label>Remove inactive jobs after, minutes<input type="number" min="1" max="10080" value={finishedJobRetentionMinutes} onChange={event => setFinishedJobRetentionMinutes(Number(event.target.value))} required/></label>
      <small>Done, failed, and cancelled jobs are removed from the admin job list after this time. Active and paused jobs are kept.</small>
    </fieldset></>}
    {section === "logs" && <fieldset><legend>Logs</legend>
      <label>Logging level<select value={logLevel} onChange={event => setLogLevel(event.target.value as typeof logLevel)}>
        <option value="D">D — debug</option>
        <option value="I">I — info</option>
        <option value="W">W — warning</option>
        <option value="E">E — error only</option>
      </select></label>
      <label>Rotate after, MB<input type="number" min="1" max="1024" value={logRotateMaxSizeMB} onChange={event => setLogRotateMaxSizeMB(Number(event.target.value))} required/></label>
      <label>Keep rotated files<input type="number" min="1" max="100" value={logRotateMaxBackups} onChange={event => setLogRotateMaxBackups(Number(event.target.value))} required/></label>
      <label>Keep logs, days<input type="number" min="1" max="3650" value={logRotateMaxAgeDays} onChange={event => setLogRotateMaxAgeDays(Number(event.target.value))} required/></label>
      <small>Application logs use Android-style prefixes D/I/W/E. Level is applied immediately; rotate rules are saved for the configured log sink.</small>
    </fieldset>}
    </>}
    {error && <p className="error">{error}</p>}{saving && <p className="muted">Saving…</p>}{saved && !saving && <p className="success">Settings saved.</p>}
  </div>;
}

function settingsSectionTitle(section:"network"|"mail"|"timeouts"|"thumbnails"|"jobs"|"logs") {
  if (section === "network") return "Network";
  if (section === "mail") return "Mail";
  if (section === "timeouts") return "Timeouts";
  if (section === "thumbnails") return "Thumbnails";
  if (section === "jobs") return "Background jobs";
  return "Log settings";
}

function FirstSetup({onComplete}:{onComplete:(user:User)=>void}) {
  const [error, setError] = useState("");
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const data = new FormData(event.currentTarget);
    const login = String(data.get("login"));
    const password = String(data.get("password"));
    if (password !== String(data.get("confirmPassword"))) {
      setError("Passwords do not match");
      return;
    }
    try {
      await api.setup(login, password);
      onComplete(await api.login(login, password));
    } catch (cause) {
      setError(loginErrorMessage(cause));
    }
  }
  return <main className="center"><form className="card login" onSubmit={submit}>
    <h1>Create administrator</h1>
    <p>This is the first startup. Create the account that will manage users, libraries, and access.</p>
    <label>Login<input name="login" autoComplete="username" minLength={3} maxLength={64} pattern="[A-Za-z0-9._-]+" required/></label>
    <label>Password<input name="password" type="password" minLength={12} autoComplete="new-password" required/></label>
    <label>Confirm password<input name="confirmPassword" type="password" minLength={12} autoComplete="new-password" required/></label>
    {error && <p className="error">{error}</p>}<button type="submit">Create administrator</button>
  </form></main>;
}

function Login({onLogin}:{onLogin:(user:User)=>void}) {
  const [error, setError] = useState("");
  const [forgot, setForgot] = useState(false);
  const [forgotEmail, setForgotEmail] = useState("");
  const [forgotMessage, setForgotMessage] = useState("");
  const [forgotBusy, setForgotBusy] = useState(false);
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault(); const data = new FormData(event.currentTarget);
    try { onLogin(await api.login(String(data.get("login")), String(data.get("password")))); }
    catch (e) { setError(loginErrorMessage(e)); }
  }
  async function requestReset(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setForgotBusy(true); setForgotMessage(""); setError("");
    try {
      const result = await api.forgotPassword(forgotEmail);
      if (result.sent) setForgotMessage("If an account exists for this email, a reset link has been sent.");
      else if (result.reason === "smtpNotConfigured") setForgotMessage("Password reset is not available because outbound email is not configured. Contact an administrator.");
      else setForgotMessage("If an account exists for this email, a reset link has been sent.");
      setForgot(false);
    } catch (cause) {
      setError(loginErrorMessage(cause));
    } finally {
      setForgotBusy(false);
    }
  }
  return <main className="center"><form className="card login" onSubmit={submit}><h1>Media Library</h1>
    <label>Login<input name="login" autoComplete="username" required/></label>
    <label>Password<input name="password" type="password" required/></label>
    {error && <p className="error">{error}</p>}<button type="submit">Sign in</button>
    <p className="muted"><button type="button" className="link-button" onClick={() => { setForgot(value => !value); setError(""); }}>Forgot password?</button></p>
    {forgot && <fieldset><legend>Password reset</legend>
      <label>Email<input type="email" value={forgotEmail} onChange={event => setForgotEmail(event.target.value)} autoComplete="email" required/></label>
      <button type="button" className="secondary" disabled={forgotBusy} onClick={event => void requestReset(event as unknown as FormEvent<HTMLFormElement>)}>Send reset link</button>
    </fieldset>}
    {forgotMessage && <p className="success">{forgotMessage}</p>}
  </form></main>;
}

function ResetPassword() {
  const [searchParams] = useSearchParams();
  const token = searchParams.get("token") ?? "";
  const [password, setPassword] = useState("");
  const [confirm, setConfirm] = useState("");
  const [error, setError] = useState("");
  const [message, setMessage] = useState("");
  const [busy, setBusy] = useState(false);
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(""); setMessage("");
    if (password !== confirm) { setError("Passwords do not match"); return; }
    setBusy(true);
    try {
      await api.resetPassword(token, password);
      setMessage("Your password has been reset. You can now sign in.");
    } catch (cause) {
      setError(loginErrorMessage(cause));
    } finally {
      setBusy(false);
    }
  }
  return <main className="center"><form className="card login" onSubmit={submit}><h1>Reset password</h1>
    {!token ? <p className="error">This reset link is missing its token.</p> : <>
      <label>New password<input type="password" value={password} onChange={event => setPassword(event.target.value)} minLength={12} autoComplete="new-password" required/></label>
      <label>Confirm password<input type="password" value={confirm} onChange={event => setConfirm(event.target.value)} minLength={12} autoComplete="new-password" required/></label>
      {error && <p className="error">{error}</p>}
      <button type="submit" disabled={busy}>Reset password</button>
    </>}
    {message && <p className="success">{message}</p>}
    {message && <p className="muted"><Link to="/">Sign in</Link></p>}
  </form></main>;
}

function UserSettingsModal({user, theme, zoom, resolvedTheme, onThemeChange, onZoomChange, onUserChanged, onClose}:{
  user:User; theme:"light"|"dark"|"system"; zoom:number; resolvedTheme:"light"|"dark";
  onThemeChange:(theme:"light"|"dark"|"system")=>void;
  onZoomChange:(zoom:number)=>void;
  onUserChanged:(user:User)=>void; onClose:()=>void;
}) {
  const [draftTheme, setDraftTheme] = useState<"light"|"dark"|"system">(theme);
  const [draftZoom, setDraftZoom] = useState(zoom);
  const [codec, setCodec] = useState<UserSettingsPayload["codec"]>("h264-aac-mp4");
  const [defaultThumbImage, setDefaultThumbImage] = useState("mountains");
  const [defaultThumbVideo, setDefaultThumbVideo] = useState("mountains");
  const [defaultThumbFolder, setDefaultThumbFolder] = useState("mountains");
  const [loaded, setLoaded] = useState(false);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  const [email, setEmail] = useState(user.email ?? "");
  const [emailError, setEmailError] = useState("");
  const [emailSaved, setEmailSaved] = useState(false);
  const [passwordModalOpen, setPasswordModalOpen] = useState(false);
  useEffect(() => {
    api.userSettings().then(settings => {
      setCodec(normalizeSchemaId(settings.codec));
      setDefaultThumbImage(settings.defaultThumbImage || "mountains");
      setDefaultThumbVideo(settings.defaultThumbVideo || "mountains");
      setDefaultThumbFolder(settings.defaultThumbFolder || "mountains");
      setLoaded(true);
    }).catch(() => undefined);
  }, [user.id]);
  useEffect(() => {
    document.documentElement.style.fontSize = `${draftZoom}%`;
  }, [draftZoom]);
  function close() {
    document.documentElement.dataset.theme = resolvedTheme;
    document.documentElement.style.fontSize = `${zoom}%`;
    onClose();
  }
  function notify(cause:unknown) { return (cause as Error).message; }
  async function saveSettings() {
    setSaving(true); setError(""); setSaved(false);
    try {
      await api.updateUserSettings({theme: draftTheme, codec, zoom: draftZoom, defaultThumbImage, defaultThumbVideo, defaultThumbFolder});
      syncUserDefaultThumbs({theme: draftTheme, codec, zoom: draftZoom, defaultThumbImage, defaultThumbVideo, defaultThumbFolder});
      onThemeChange(draftTheme);
      onZoomChange(draftZoom);
      setSaved(true);
    } catch (cause) { setError(notify(cause)); } finally { setSaving(false); }
  }
  async function saveEmail() {
    const trimmed = email.trim();
    if (trimmed === (user.email ?? "").trim()) return;
    setEmailError(""); setEmailSaved(false);
    try {
      await api.updateEmail(trimmed);
      onUserChanged({...user, email: trimmed});
      setEmailSaved(true);
    } catch (cause) { setEmailError(notify(cause)); }
  }
  return <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label="User settings" onClick={event => closeOnBackdropClick(event, close)}>
    <div className="card settings modal user-settings-modal">
      <div className="panel-title"><h2>User settings</h2><button type="button" className="secondary" onClick={close}>Close</button></div>
      <p className="muted">Preferences for {user.login}. These apply only to your account.</p>
      <fieldset><legend>Appearance</legend>
        <label>Theme<select value={draftTheme} onChange={event => { setDraftTheme(event.target.value as "light"|"dark"|"system"); setSaved(false); }}>
          <option value="light">Light</option>
          <option value="dark">Dark</option>
          <option value="system">System — match device</option>
        </select></label>
        <small>System follows your browser or operating system setting.</small>
      </fieldset>
      <fieldset><legend>Zoom</legend>
        <label>Zoom<select value={draftZoom} onChange={event => { setDraftZoom(Number(event.target.value)); setSaved(false); }}>
          {[80, 90, 100, 110, 120, 130, 140].map(step =>
            <option key={step} value={step}>{step}%</option>
          )}
        </select></label>
        <small>Scales the whole interface, including folder and file names.</small>
      </fieldset>
      <fieldset><legend>Video fallback profile</legend>
        <label>Transcode schema<select value={codec} disabled={!loaded} onChange={event => { setCodec(event.target.value as UserSettingsPayload["codec"]); setSaved(false); }}>
          {(Object.keys(TRANSCODE_SCHEMAS) as UserSettingsPayload["codec"][]).map(id => {
            const schema = TRANSCODE_SCHEMAS[id];
            return <option key={id} value={id}>{schema.videoLabel} + {schema.audioLabel} → {schema.containerLabel} — {schema.compressionLabel} compression — browser: {schema.supportLabel}</option>;
          })}
        </select></label>
        <small>Used when your browser cannot play the original video. Choose the profile your devices support best.</small>
        <small>Your browser plays directly without transcoding: {supportedVideoFormats().join(", ") || "none"}.</small>
      </fieldset>
      <fieldset><legend>Default pictures</legend>
        <small>Shown while a thumbnail has not been generated yet, so you can see which items are not covered. You can pick a different picture for images, videos, and folders.</small>
        {(["image","video","folder"] as const).map(kind => {
          const value = kind === "image" ? defaultThumbImage : kind === "video" ? defaultThumbVideo : defaultThumbFolder;
          const onChange = kind === "image" ? setDefaultThumbImage : kind === "video" ? setDefaultThumbVideo : setDefaultThumbFolder;
          return <div className="thumb-picker" key={kind}>
            <span className="thumb-picker-label">{kind === "image" ? "Images" : kind === "video" ? "Videos" : "Folders"}</span>
            <div className="thumb-picker-grid" role="radiogroup" aria-label={`Default picture for ${kind === "image" ? "images" : kind === "video" ? "videos" : "folders"}`}>
              {DEFAULT_THUMB_PICTURES.map(picture => (
                <button key={picture.id} type="button" role="radio" aria-checked={value === picture.id} title={picture.name}
                  className={`thumb-picker-option${value === picture.id ? " selected" : ""}`}
                  onClick={() => { onChange(picture.id); setSaved(false); }}>
                  <span className="thumb-default-picture" style={{backgroundImage:`url("${svgDataUri(picture.svg)}")`}}/>
                  {kind !== "image" && <span className={`thumb-kind-badge thumb-kind-badge-sm ${kind === "folder" ? "thumb-kind-folder" : "thumb-kind-video"}`}>{kind === "video" ? "▶" : "▰"}</span>}
                </button>
              ))}
            </div>
          </div>;
        })}
      </fieldset>
      <fieldset><legend>Email</legend>
        <label>Email address<input type="email" value={email} onChange={event => setEmail(event.target.value)} onBlur={() => void saveEmail()} autoComplete="email" placeholder="you@example.com"/></label>
        <small>Required to receive password reset links when you forget your password. Saved automatically when you leave the field.</small>
        {emailError && <p className="error">{emailError}</p>}
        {emailSaved && <p className="success">Email saved.</p>}
      </fieldset>
      <fieldset><legend>Security</legend>
        <button type="button" className="secondary" onClick={() => setPasswordModalOpen(true)}>Change password…</button>
        <small>Requires your current password. Use a separate strong password.</small>
      </fieldset>
      {error && <p className="error">{error}</p>}
      {saved && <p className="success">Settings saved.</p>}
      <button type="button" className="settings-save" onClick={() => void saveSettings()} disabled={saving || !loaded}>{saving ? "Saving…" : "Save settings"}</button>
    </div>
    {passwordModalOpen && <ChangePasswordModal onClose={() => setPasswordModalOpen(false)}/>}
  </div>;
}

function ChangePasswordModal({onClose}:{onClose:()=>void}) {
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const [saved, setSaved] = useState(false);
  async function submit(event:FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setError(""); setSaved(false);
    if (newPassword !== confirmPassword) { setError("Passwords do not match"); return; }
    setBusy(true);
    try {
      await api.changePassword(currentPassword, newPassword);
      setSaved(true);
      setCurrentPassword(""); setNewPassword(""); setConfirmPassword("");
    } catch (cause) { setError((cause as Error).message); } finally { setBusy(false); }
  }
  return <div className="modal-backdrop nested" role="dialog" aria-modal="true" aria-label="Change password" onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal user-settings-modal">
      <div className="panel-title"><h2>Change password</h2><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      <form onSubmit={submit}>
        <label>Current password<input type="password" value={currentPassword} onChange={event => setCurrentPassword(event.target.value)} autoComplete="current-password" required/></label>
        <label>New password<input type="password" value={newPassword} onChange={event => setNewPassword(event.target.value)} minLength={12} autoComplete="new-password" required/></label>
        <label>Confirm new password<input type="password" value={confirmPassword} onChange={event => setConfirmPassword(event.target.value)} minLength={12} autoComplete="new-password" required/></label>
        {error && <p className="error">{error}</p>}
        {saved && <p className="success">Password updated.</p>}
        <button type="submit" disabled={busy}>{busy ? "Saving…" : "Update password"}</button>
      </form>
    </div>
  </div>;
}

function Libraries() {
  const [items, setItems] = useState<Library[]>([]);
  useEffect(() => { api.libraries().then(setItems); }, []);
  return <main className="browser-page"><h1>Your libraries</h1><div className="grid">{items.map(item =>
    <LibraryTile key={item.id} item={item}/>
  )}</div></main>;
}

function LibraryTile({item}:{item:Library}) {
  const [showStats, setShowStats] = useState(false);
  return <div className="card library library-tile">
    <Link className="folder-thumb-button" aria-label={`Open library ${item.name}`} to={`/library/${item.id}`}><span className="folder">▰</span></Link>
    <button type="button" className="folder-title-button" onClick={() => setShowStats(true)}><h2>{item.name}</h2></button>
    {showStats && <LibraryStatsModal library={item} onClose={() => setShowStats(false)}/>}
  </div>;
}

function LibraryStatsModal({library, onClose}:{library:{id:ID; name:string}; onClose:()=>void}) {
  const [stats, setStats] = useState<Library["stats"]|null>(null);
  const [statsError, setStatsError] = useState("");
  const [calculating, setCalculating] = useState(true);
  useEffect(() => {
    let cancelled = false;
    setCalculating(true); setStatsError("");
    api.libraryStats(library.id)
      .then(result => { if (!cancelled) setStats(result); })
      .catch(cause => { if (!cancelled) setStatsError((cause as Error).message); })
      .finally(() => { if (!cancelled) setCalculating(false); });
    return () => { cancelled = true; };
  }, [library.id]);
  return <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Library statistics ${library.name}`} onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal folder-stats">
      <div className="panel-title"><h2>{library.name}</h2><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      {calculating && <p className="muted">Calculating…</p>}
      {statsError && <p className="error">{statsError}</p>}
      {stats && <div className="stats-grid">
        <div><strong>{stats.folders}</strong><span>Folders</span></div>
        <div><strong>{stats.files}</strong><span>Files</span></div>
        <div><strong>{stats.images}</strong><span>Images</span></div>
        <div><strong>{stats.videos}</strong><span>Videos</span></div>
      </div>}
    </div>
  </div>;
}

function ThumbnailRefreshModal({title,busy,error,onClose,onRefresh}:{title:string; busy:boolean; error:string; onClose:()=>void; onRefresh:(recreateExisting:boolean)=>void}) {
  return <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Refresh thumbnails ${title}`} onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal">
      <div className="panel-title"><h2>Refresh thumbnails</h2><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      <p><strong>{title}</strong></p>
      <p className="muted">Choose whether existing thumbnails should be kept or regenerated.</p>
      {error && <p className="error">{error}</p>}
      <div className="action-row">
        <button type="button" disabled={busy} onClick={() => onRefresh(false)}>Missing only</button>
        <button type="button" className="secondary" disabled={busy} onClick={() => onRefresh(true)}>Recreate existing</button>
      </div>
    </div>
  </div>;
}

function Browser() {
  const {id="", folderId} = useParams(); const navigate = useNavigate();
  const libraryId = Number(id);
  const currentFolderId = folderId == null ? null : Number(folderId);
  const [entries, setEntries] = useState<Entry[]>([]);
  const [view, setView] = useState<"tile"|"list">("tile");
  const [selected, setSelected] = useState<ID[]>([]);
  useEffect(() => {
    if (!Number.isFinite(libraryId)) return;
    const load = currentFolderId == null ? api.entries(libraryId) : api.folderEntries(libraryId, currentFolderId);
    load.then(setEntries);
  }, [libraryId,currentFolderId]);
  const mediaItems = entries.flatMap(entry => entry.type === "media" && entry.media ? [entry.media] : []);
  function applyBulkGPS(updated:Media[]) {
    setEntries(currentEntries => currentEntries.map(entry => {
      if (entry.type !== "media" || !entry.media) return entry;
      const item = updated.find(media => media.id === entry.media?.id);
      return item ? {...entry, media:item} : entry;
    }));
    setSelected([]);
  }
  return <main className="browser-page"><div className="browser-bar">
    <div className="view-toggle"><Link className="button-like active" to={currentFolderId == null ? `/library/${id}` : `/library/${id}/folder/${currentFolderId}`}>Folders</Link><Link className="button-like" to={`/library/${id}/timeline`}>Timeline</Link><Link className="button-like" to={`/map?library=${id}${currentFolderId != null ? `&folder=${currentFolderId}` : ""}`}>Map</Link><button className={view === "tile" ? "active" : ""} onClick={() => setView("tile")}>Tile</button><button className={view === "list" ? "active" : ""} onClick={() => setView("list")}>List</button></div></div>
    <BulkGPSBar items={mediaItems} selectedIds={selected} onSelectedIds={setSelected} onUpdated={applyBulkGPS}/>
    <VirtualEntries entries={entries} view={view} libraryId={libraryId} selectedIds={selected} onToggleSelected={toggleSelected(setSelected)} onOpenFolder={entry => navigate(`/library/${libraryId}/folder/${entry.id}`)}/></main>;
}

function LibraryTimeline() {
  const {id=""} = useParams();
  const libraryId = Number(id);
  const [items, setItems] = useState<Media[]>([]);
  const [kind, setKind] = useState<"all"|"image"|"video">("all");
  const [sort, setSort] = useState<"desc"|"asc">("desc");
  useEffect(() => { if (Number.isFinite(libraryId)) api.libraryMedia(libraryId).then(setItems); }, [libraryId]);
  const filtered = items.filter(item => kind === "all" || item.kind === kind);
  const sorted = sortMedia(filtered, sort);
  const [selected, setSelected] = useState<ID[]>([]);
  function applyBulkGPS(updated:Media[]) {
    setItems(current => current.map(item => updated.find(media => media.id === item.id) ?? item));
    setSelected([]);
  }
  return <main className="browser-page">
    <div className="browser-bar"><div className="crumbs"><Link to="/">Libraries</Link> / Timeline</div>
      <div className="view-toggle"><Link className="button-like" to={`/library/${id}`}>Folders</Link><Link className="button-like active" to={`/library/${id}/timeline`}>Timeline</Link>
        <button className={kind === "all" ? "active" : ""} onClick={() => setKind("all")}>All</button>
        <button className={kind === "image" ? "active" : ""} onClick={() => setKind("image")}>Images</button>
        <button className={kind === "video" ? "active" : ""} onClick={() => setKind("video")}>Videos</button>
        <button onClick={() => setSort(value => value === "desc" ? "asc" : "desc")}>{sort === "desc" ? "Newest first" : "Oldest first"}</button>
      </div></div>
    <BulkGPSBar items={filtered} selectedIds={selected} onSelectedIds={setSelected} onUpdated={applyBulkGPS}/>
    {sorted.length === 0 ? <div className="empty-state"><p>No dated items in this library yet.</p></div> :
      <div className="timeline-grid">{groupByDate(sorted).map(group =>
        <div className="timeline-group" key={group.label}>
          <span className="timeline-group-date">{group.label}</span>
          <span className="timeline-group-dot" aria-hidden="true"/>
          <div className="timeline-group-grid">{group.items.map(item =>
            <MediaCard key={item.id} item={item} view="tile" libraryId={libraryId} selected={selected.includes(item.id)} onToggleSelected={toggleSelected(setSelected)}/>
          )}</div>
        </div>
      )}</div>}
  </main>;
}

function Favorites() {
  const [views, setViews] = useState<FavoriteView[]>([]);
  const [name, setName] = useState("");
  const [error, setError] = useState("");
  function load() {
    api.favoriteViews().then(setViews).catch(cause => setError((cause as Error).message));
  }
  useEffect(load, []);
  async function create(event:FormEvent) {
    event.preventDefault();
    const trimmed = name.trim();
    if (!trimmed) return;
    try {
      const created = await api.createFavoriteView(trimmed);
      setViews(current => [...current, created].sort((left, right) => left.name.localeCompare(right.name)));
      setName("");
    } catch (cause) {
      setError((cause as Error).message);
    }
  }
  return <main className="browser-page">
    <h1>Favorite views</h1>
    <form className="inline-create" onSubmit={create}>
      <label>New view<input value={name} onChange={event => setName(event.target.value)} placeholder="Best photos"/></label>
      <button type="submit">Create</button>
    </form>
    {error && <p className="error">{error}</p>}
    {views.length === 0 ? <div className="empty-state"><p>No favorite views yet. Create one, then add media into it.</p></div> :
      <div className="library-table">{views.map(view => <FavoriteViewRow key={view.id} view={view} onChange={load}/>)}</div>}
  </main>;
}

function FavoriteViewRow({view,onChange}:{view:FavoriteView; onChange:()=>void}) {
  const [renaming, setRenaming] = useState(false);
  const [name, setName] = useState(view.name);
  const [busy, setBusy] = useState(false);
  useEffect(() => setName(view.name), [view.name]);
  async function saveRename(event:FormEvent) {
    event.preventDefault();
    setBusy(true);
    try {
      await api.updateFavoriteView(view.id, name);
      setRenaming(false);
      onChange();
    } finally {
      setBusy(false);
    }
  }
  async function remove() {
    if (!window.confirm(`Delete favorite view "${view.name}"?`)) return;
    setBusy(true);
    try {
      await api.deleteFavoriteView(view.id);
      onChange();
    } finally {
      setBusy(false);
    }
  }
  return <div className="library-row">
    {renaming ? <form className="library-main inline-edit" onSubmit={saveRename}><label>View name<input value={name} onChange={event => setName(event.target.value)} required/></label><button disabled={busy}>Save</button><button type="button" className="secondary" onClick={() => setRenaming(false)}>Cancel</button></form> :
      <Link className="library-main" to={`/favorites/${view.id}`}><span className="folder">★</span><span><strong>{view.name}</strong><small>{view.count} items</small></span></Link>}
    <details className="item-menu">
      <summary aria-label={`Favorite view menu ${view.name}`}>⋮</summary>
      <div className="item-submenu" role="menu" onClick={closeParentDetails}>
        <button type="button" role="menuitem" disabled={busy} onClick={() => setRenaming(true)}>Rename</button>
        <button type="button" role="menuitem" className="danger" disabled={busy} onClick={remove}>Delete</button>
      </div>
    </details>
  </div>;
}

function FavoriteViewPage() {
  const {viewId=""} = useParams();
  const favoriteViewId = Number(viewId);
  const [items, setItems] = useState<Media[]>([]);
  const [view, setView] = useState<"tile"|"list">("tile");
  const [views, setViews] = useState<FavoriteView[]>([]);
  const [error, setError] = useState("");
  const [selectedIds, setSelectedIds] = useState<ID[]>([]);
  function load() {
    if (Number.isFinite(favoriteViewId)) api.favoriteViewMedia(favoriteViewId).then(setItems).catch(cause => setError((cause as Error).message));
  }
  useEffect(load, [favoriteViewId]);
  useEffect(() => { api.favoriteViews().then(setViews).catch(() => undefined); }, []);
  const selected = views.find(view => view.id === Number(viewId));
  return <main className="browser-page">
    <h1>{selected?.name ?? "Favorite view"}</h1>
    <div className="browser-bar"><div className="crumbs"><Link to="/">Libraries</Link> / <Link to="/favorites">Favorite views</Link> / {selected?.name ?? "View"}</div>
      <div className="view-toggle"><button className={view === "tile" ? "active" : ""} onClick={() => setView("tile")}>Tile</button><button className={view === "list" ? "active" : ""} onClick={() => setView("list")}>List</button></div></div>
    <BulkGPSBar items={items} selectedIds={selectedIds} onSelectedIds={setSelectedIds} onUpdated={updated => { setItems(current => current.map(item => updated.find(media => media.id === item.id) ?? item)); setSelectedIds([]); }}/>
    {error && <p className="error">{error}</p>}
    {items.length === 0 ? <div className="empty-state"><p>No favorites yet.</p></div> :
      <div className={view === "tile" ? "grid" : "list-view"}>{items.map(item =>
        <MediaCard key={item.id} item={item} view={view} favoriteViewId={favoriteViewId} selected={selectedIds.includes(item.id)} onToggleSelected={toggleSelected(setSelectedIds)} onFavoriteChange={updated => setItems(current => updated.favorite ? current.map(media => media.id === updated.id ? updated : media) : current.filter(media => media.id !== updated.id))}/>
      )}</div>}
  </main>;
}

function FavoriteMediaViewerPage() {
  const {mediaId=""} = useParams();
  const [query] = useSearchParams();
  const navigate = useNavigate();
  const viewId = query.get("viewId") ?? "";
  const favoriteViewId = Number(viewId);
  const currentMediaId = Number(mediaId);
  const [items, setItems] = useState<Media[]>([]);
  const [fallbackItem, setFallbackItem] = useState<Media|null>(null);
  const [infoOpen, setInfoOpen] = useState(false);
  const index = items.findIndex(media => media.id === currentMediaId);
  const item = index >= 0 ? items[index] : fallbackItem;
  const previous = index > 0 ? items[index - 1] : null;
  const next = index >= 0 && index < items.length - 1 ? items[index + 1] : null;
  useEffect(() => { if (Number.isFinite(favoriteViewId)) api.favoriteViewMedia(favoriteViewId).then(setItems); }, [favoriteViewId]);
  useEffect(() => { if (Number.isFinite(currentMediaId)) api.media(currentMediaId).then(setFallbackItem).catch(() => setFallbackItem(null)); }, [currentMediaId]);
  function go(media:Media|null) {
    if (media) navigate(`/favorites/view/${media.id}?viewId=${encodeURIComponent(viewId)}`);
  }
  useEffect(() => {
    function onKeyDown(event:KeyboardEvent) {
      if (isEditableTarget(event.target)) return;
      if (event.key === "ArrowLeft" && previous) {
        event.preventDefault();
        go(previous);
      } else if (event.key === "ArrowRight" && next) {
        event.preventDefault();
        go(next);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [previous,next,navigate]);
  if (!item) return <main className="viewer-page"><p>Loading media…</p></main>;
  return <main className="viewer-page">
    <Viewer item={item} favoriteViewId={favoriteViewId} infoOpen={infoOpen} previous={previous} next={next} onGo={go} onToggleInfo={() => setInfoOpen(value => !value)}/>
  </main>;
}

function BulkGPSBar({items,selectedIds,onSelectedIds,onUpdated}:{items:Media[]; selectedIds:ID[]; onSelectedIds:(ids:ID[])=>void; onUpdated:(items:Media[])=>void}) {
  const [gps, setGPS] = useState("");
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState("");
  const selected = items.filter(item => selectedIds.includes(item.id));
  if (items.length === 0) return null;
  async function save(event:FormEvent) {
    event.preventDefault();
    const trimmed = gps.trim();
    if (selected.length === 0) {
      setError("Select at least one item");
      return;
    }
    if (!trimmed) {
      setError("GPS is required");
      return;
    }
    setBusy(true); setError("");
    try {
      const updated = await Promise.all(selected.map(item => api.updateMediaDetails(item.id, {
        name: item.name,
        gps: trimmed,
        takenAt: item.takenAt || null,
      })));
      onUpdated(updated);
      setGPS("");
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setBusy(false);
    }
  }
  return <form className="bulk-toolbar" aria-label="Bulk edit selected media" onSubmit={save}>
    <label className="check"><input type="checkbox" checked={selected.length > 0 && selected.length === items.length} onChange={event => onSelectedIds(event.target.checked ? items.map(item => item.id) : [])}/> Select all</label>
    <span>{selected.length} selected</span>
    <label>GPS for selected<input value={gps} onChange={event => setGPS(event.target.value)} placeholder="50.45,30.52"/></label>
    <button type="submit" disabled={busy || selected.length === 0}>{busy ? "Saving…" : "Apply GPS"}</button>
    {selected.length > 0 && <button type="button" className="secondary" disabled={busy} onClick={() => onSelectedIds([])}>Clear</button>}
    {error && <small className="error">{error}</small>}
  </form>;
}

function toggleSelected(setSelected:(update:(current:ID[])=>ID[])=>void) {
  return (id:ID) => setSelected(current => current.includes(id) ? current.filter(item => item !== id) : [...current, id]);
}

function sortMedia<T extends Media>(items:readonly T[], sort:"desc"|"asc") {
  return [...items].sort((left, right) => {
    const leftTime = mediaTime(left);
    const rightTime = mediaTime(right);
    if (leftTime !== rightTime) return sort === "desc" ? rightTime - leftTime : leftTime - rightTime;
    return left.name.localeCompare(right.name);
  });
}

function mediaTime(item:Media) {
  const value = item.takenAt ? Date.parse(item.takenAt) : Number.NaN;
  return Number.isFinite(value) ? value : 0;
}

function formatShortDate(value:string) {
  const date = value ? new Date(value) : null;
  if (!date || Number.isNaN(date.getTime())) return "Unknown date";
  return date.toLocaleDateString(undefined, {year:"numeric", month:"short", day:"numeric"});
}

function groupByDate<T extends {takenAt:string}>(items:readonly T[]): {label:string; items:T[]}[] {
  const groups: {label:string; items:T[]}[] = [];
  for (const item of items) {
    const label = formatShortDate(item.takenAt);
    const last = groups[groups.length - 1];
    if (last && last.label === label) last.items.push(item);
    else groups.push({label, items:[item]});
  }
  return groups;
}

function VirtualEntries({entries,view,libraryId,selectedIds,onToggleSelected,onOpenFolder}:{entries:Entry[]; view:"tile"|"list"; libraryId:ID; selectedIds:ID[]; onToggleSelected:(id:ID)=>void; onOpenFolder:(entry:Entry)=>void}) {
  const ref = useRef<HTMLDivElement|null>(null);
  const [viewport, setViewport] = useState({scrollY:window.scrollY, height:window.innerHeight, width:window.innerWidth, top:0});
  const gap = view === "tile" ? 18 : 6;
  const rowHeight = view === "tile" ? 292 : 78;
  const columns = view === "tile" ? Math.max(1, Math.floor((viewport.width + gap) / (190 + gap))) : 1;
  const totalRows = Math.ceil(entries.length / columns);
  const viewportTop = Math.max(0, viewport.scrollY - viewport.top);
  const firstVisibleRow = Math.floor(viewportTop / (rowHeight + gap));
  const visibleRows = Math.ceil(viewport.height / (rowHeight + gap));
  const overscan = 3;
  const firstRow = Math.max(0, firstVisibleRow - overscan);
  const lastRow = Math.min(totalRows, firstVisibleRow + visibleRows + overscan);
  const start = firstRow * columns;
  const end = Math.min(entries.length, lastRow * columns);
  const offsetY = firstRow * (rowHeight + gap);
  const totalHeight = totalRows === 0 ? 0 : totalRows * rowHeight + Math.max(0, totalRows - 1) * gap;
  useEffect(() => {
    function update() {
      const rect = ref.current?.getBoundingClientRect();
      setViewport({
        scrollY: window.scrollY,
        height: window.innerHeight,
        width: rect?.width || window.innerWidth,
        top: (rect?.top ?? 0) + window.scrollY,
      });
    }
    update();
    window.addEventListener("scroll", update, {passive:true});
    window.addEventListener("resize", update);
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(update);
    if (ref.current) observer?.observe(ref.current);
    return () => {
      window.removeEventListener("scroll", update);
      window.removeEventListener("resize", update);
      observer?.disconnect();
    };
  }, [entries.length, view]);
  if (entries.length === 0) return <div className="empty-state"><p>No items here.</p></div>;
  return <div ref={ref} className={`virtual-browser ${view === "tile" ? "virtual-tile" : "virtual-list"}`} style={{height:totalHeight}}>
    <div className={view === "tile" ? "grid virtual-window" : "list-view virtual-window"} style={{transform:`translateY(${offsetY}px)`}}>
      {entries.slice(start, end).map((entry, index) => {
        const row = Math.floor((start + index) / columns);
        const priority = row >= firstVisibleRow && row < firstVisibleRow + visibleRows;
        return entry.type === "folder" ?
          <FolderEntry key={`folder-${entry.id}`} entry={entry} view={view} libraryId={libraryId} priority={priority} onOpenFolder={onOpenFolder}/> :
          <MediaCard key={`media-${entry.media!.id}`} item={entry.media!} view={view} libraryId={libraryId} priority={priority} selected={selectedIds.includes(entry.media!.id)} onToggleSelected={onToggleSelected}/>;
      })}
    </div>
  </div>;
}

function FolderEntry({entry, view, libraryId, priority, onOpenFolder}:{entry:Entry; view:"tile"|"list"; libraryId:ID; priority?:boolean; onOpenFolder:(entry:Entry)=>void}) {
  const [stats, setStats] = useState<{folders:number; media:number; images:number; videos:number}|null>(null);
  const [statsError, setStatsError] = useState("");
  const [calculating, setCalculating] = useState(false);
  const [refreshing, setRefreshing] = useState(false);
  const [thumbnailOptionsOpen, setThumbnailOptionsOpen] = useState(false);
  const [thumbnailError, setThumbnailError] = useState("");
  async function openStats() {
    setStats(null); setStatsError(""); setCalculating(true);
    try {
      const children = await api.folderEntries(libraryId, entry.id);
      setStats({
        folders: children.filter(child => child.type === "folder").length,
        media: children.filter(child => child.type === "media").length,
        images: children.filter(child => child.media?.kind === "image").length,
        videos: children.filter(child => child.media?.kind === "video").length,
      });
    } catch (cause) {
      setStatsError((cause as Error).message);
    } finally {
      setCalculating(false);
    }
  }
  async function refreshFolder() {
    setRefreshing(true);
    try {
      await api.scanLibrary(libraryId);
    } finally {
      setRefreshing(false);
    }
  }
  async function refreshFolderThumbnails(recreateExisting:boolean) {
    setRefreshing(true); setThumbnailError("");
    try {
      await api.createThumbnails(libraryId, {recreateExisting});
      setThumbnailOptionsOpen(false);
    } catch (cause) {
      setThumbnailError((cause as Error).message);
    } finally {
      setRefreshing(false);
    }
  }
  return <div className={`card library folder-entry ${view === "list" ? "folder-entry-list" : ""}`}>
    <button type="button" className="folder-thumb-button" aria-label={`Open folder ${entry.name}`} onClick={() => onOpenFolder(entry)}>
      {view === "tile" && <FolderCover folderId={entry.folderThumbnail} priority={priority}/>}
      {view === "list" && <span className="folder">▰</span>}
    </button>
    <button type="button" className="folder-title-button" onClick={openStats}><h2>{entry.name}</h2></button>
    <details className="item-menu folder-menu">
      <summary aria-label={`Folder menu ${entry.name}`}>⋮</summary>
      <div className="item-submenu" role="menu" onClick={closeParentDetails}>
        <button type="button" role="menuitem" disabled={refreshing} onClick={refreshFolder}>{refreshing ? "Refreshing…" : "Refresh items"}</button>
        <button type="button" role="menuitem" disabled={refreshing} onClick={() => setThumbnailOptionsOpen(true)}>Refresh thumbnails…</button>
      </div>
    </details>
    {thumbnailOptionsOpen && <ThumbnailRefreshModal title={entry.name} busy={refreshing} error={thumbnailError} onClose={() => setThumbnailOptionsOpen(false)} onRefresh={refreshFolderThumbnails}/>}
    {(stats || statsError || calculating) && <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Folder statistics ${entry.name}`} onClick={event => closeOnBackdropClick(event, () => { setStats(null); setStatsError(""); })}>
      <div className="card settings modal folder-stats">
        <div className="panel-title"><h2>{entry.name}</h2><button type="button" className="secondary" onClick={() => { setStats(null); setStatsError(""); }}>Close</button></div>
        {calculating && <p className="muted">Calculating…</p>}
        {statsError && <p className="error">{statsError}</p>}
        {stats && <div className="stats-grid">
          <div><strong>{stats.folders}</strong><span>Folders</span></div>
          <div><strong>{stats.media}</strong><span>Files</span></div>
          <div><strong>{stats.images}</strong><span>Images</span></div>
          <div><strong>{stats.videos}</strong><span>Videos</span></div>
        </div>}
      </div>
    </div>}
  </div>;
}

const THUMB_RETRY_MS = 8000;

const DEFAULT_THUMB_PICTURES = [
  {id:"mountains", name:"Mountains", svg:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 150" preserveAspectRatio="xMidYMid slice"><rect width="200" height="150" fill="#55779b"/><circle cx="152" cy="40" r="17" fill="#f3d27a"/><path d="M0 112 L48 66 L92 98 L128 72 L164 106 L200 88 L200 150 L0 150 Z" fill="#3f5d43"/><path d="M0 132 L60 100 L110 126 L152 104 L200 122 L200 150 L0 150 Z" fill="#2f4634"/></svg>`},
  {id:"sunset", name:"Sunset", svg:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 150" preserveAspectRatio="xMidYMid slice"><rect width="200" height="150" fill="#d98c5f"/><rect y="18" width="200" height="46" fill="#e8b068"/><circle cx="100" cy="68" r="20" fill="#f7e08a"/><path d="M0 105 L30 88 L65 100 L100 84 L140 102 L180 90 L200 98 L200 150 L0 150 Z" fill="#5d3a45"/><path d="M0 128 L50 108 L95 126 L150 106 L200 120 L200 150 L0 150 Z" fill="#4a2d38"/></svg>`},
  {id:"forest", name:"Forest", svg:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 150" preserveAspectRatio="xMidYMid slice"><rect width="200" height="150" fill="#7d98a1"/><rect y="100" width="200" height="50" fill="#37523f"/><path d="M0 110 L200 110 L200 150 L0 150 Z" fill="#2c4434"/><path d="M30 112 L48 72 L66 112 Z" fill="#24402f"/><path d="M84 112 L104 64 L124 112 Z" fill="#24402f"/><path d="M140 112 L158 76 L176 112 Z" fill="#24402f"/></svg>`},
  {id:"ocean", name:"Ocean", svg:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 150" preserveAspectRatio="xMidYMid slice"><rect width="200" height="150" fill="#4f8dab"/><rect y="16" width="200" height="42" fill="#6ba3bd"/><circle cx="150" cy="58" r="16" fill="#f3d27a"/><path d="M0 96 Q25 88 50 96 T100 96 T150 96 T200 96 L200 150 L0 150 Z" fill="#3a7d99"/><path d="M0 118 Q25 110 50 118 T100 118 T150 118 T200 118 L200 150 L0 150 Z" fill="#2f6884"/></svg>`},
  {id:"city", name:"City", svg:`<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 200 150" preserveAspectRatio="xMidYMid slice"><rect width="200" height="150" fill="#b8c0cc"/><rect y="96" width="200" height="54" fill="#7d8794"/><path d="M12 70 L34 70 L34 96 L12 96 Z M42 48 L68 48 L68 96 L42 96 Z M76 64 L96 64 L96 96 L76 96 Z M104 40 L134 40 L134 96 L104 96 Z M142 58 L166 58 L166 96 L142 96 Z M174 76 L192 76 L192 96 L174 96 Z" fill="#626c79"/><path d="M48 54 L54 54 L54 60 L48 60 Z M60 54 L66 54 L66 60 L60 60 Z M48 66 L54 66 L54 72 L48 72 Z M110 46 L116 46 L116 52 L110 52 Z M122 46 L128 46 L128 52 L122 52 Z M110 58 L116 58 L116 64 L110 64 Z M122 58 L128 58 L128 64 L122 64 Z M148 64 L154 64 L154 70 L148 70 Z M160 64 L166 64 L166 70 L160 70 Z" fill="#a9b2bd"/></svg>`}
] as const;

function svgDataUri(svg:string) { return `data:image/svg+xml,${encodeURIComponent(svg)}`; }

const userDefaultThumbs = {image:"mountains", video:"mountains", folder:"mountains"};
function syncUserDefaultThumbs(settings:UserSettingsPayload) {
  userDefaultThumbs.image = settings.defaultThumbImage || "mountains";
  userDefaultThumbs.video = settings.defaultThumbVideo || "mountains";
  userDefaultThumbs.folder = settings.defaultThumbFolder || "mountains";
}
function defaultThumbPicture(kind?:string) {
  const id = kind === "folder" ? userDefaultThumbs.folder : kind === "video" ? userDefaultThumbs.video : userDefaultThumbs.image;
  return DEFAULT_THUMB_PICTURES.find(picture => picture.id === id) ?? DEFAULT_THUMB_PICTURES[0];
}

function ThumbImage({src, priority, kind}:{src:string; priority?:boolean; kind?:string}) {
  const [state, setState] = useState<"loading"|"missing"|"loaded">("loading");
  const [attempt, setAttempt] = useState(0);
  useEffect(() => {
    if (state !== "missing" || document.hidden) return;
    const interval = window.setInterval(() => setAttempt(value => value + 1), THUMB_RETRY_MS);
    return () => window.clearInterval(interval);
  }, [state]);
  const url = attempt === 0 ? src : `${src}${src.includes("?") ? "&" : "?"}ts=${attempt}`;
  return <div className="thumb-frame">
    {state !== "loaded" && <span className="thumb-placeholder" aria-hidden="true">
      <span className="thumb-default-picture" style={{backgroundImage:`url("${svgDataUri(defaultThumbPicture(kind).svg)}")`}}/>
      {kind !== "image" && <span className={`thumb-kind-badge ${kind === "folder" ? "thumb-kind-folder" : "thumb-kind-video"}`}>{kind === "video" ? "▶" : "▰"}</span>}
    </span>}
    <img loading="lazy" fetchPriority={priority ? "high" : "auto"} src={url} alt=""
      onLoad={() => setState("loaded")}
      onError={() => setState("missing")}/>
  </div>;
}

function FolderCover({folderId, priority}:{folderId?:ID; priority?:boolean}) {
  if (folderId == null) return <span className="folder">▰</span>;
  return <div className="folder-cover"><ThumbImage src={api.folderThumbnailUrl(folderId)} priority={priority} kind="folder"/></div>;
}

function MediaCard({item,view,libraryId,favoriteViewId,selected=false,priority,onToggleSelected,onFavoriteChange}:{item:Media; view:"tile"|"list"; libraryId?:ID; favoriteViewId?:ID; selected?:boolean; priority?:boolean; onToggleSelected?:(id:ID)=>void; onFavoriteChange?:(item:Media)=>void}) {
  const navigate = useNavigate();
  const open = () => favoriteViewId != null || libraryId == null ? navigate(`/favorites/view/${item.id}?viewId=${encodeURIComponent(String(favoriteViewId ?? ""))}`) : navigate(libraryItemURL(libraryId, item));
  return <article className="card media" onClick={open} role="button" tabIndex={0}
    onKeyDown={event => { if (event.key === "Enter" || event.key === " ") open(); }}>
    {onToggleSelected && <label className="select-media" aria-label={`Select ${item.name}`} onClick={event => event.stopPropagation()}>
      <input type="checkbox" checked={selected} onChange={() => onToggleSelected(item.id)}/>
    </label>}
    {view === "tile" && <div className="thumb-wrap">
      <ThumbImage src={api.thumbnailUrl(item.id)} priority={priority} kind={item.kind}/>
      {item.kind === "video" && <span className="play-badge" aria-hidden="true">▶</span>}
    </div>}
    <div><strong>{item.name}</strong><small>{item.kind} · {formatBytes(item.size)}</small></div>
    <FavoriteButton item={item} viewId={favoriteViewId} onChange={onFavoriteChange}/>
  </article>;
}

function FavoriteButton({item,viewId,onChange}:{item:Media; viewId?:ID; onChange?:(item:Media)=>void}) {
  const [favorite, setFavorite] = useState(Boolean(item.favorite));
  const [busy, setBusy] = useState(false);
  const [choosing, setChoosing] = useState(false);
  useEffect(() => setFavorite(Boolean(item.favorite)), [item.id,item.favorite]);
  async function toggle(event:MouseEvent<HTMLButtonElement>) {
    event.stopPropagation();
    if (!(favorite && viewId != null)) {
      setChoosing(true);
      return;
    }
    setBusy(true);
    try {
      const updated = await api.unfavoriteMedia(viewId, item.id);
      setFavorite(Boolean(updated.favorite));
      onChange?.(updated);
    } finally {
      setBusy(false);
    }
  }
  async function addToView(targetViewId:ID) {
    setBusy(true);
    try {
      const updated = await api.favoriteMedia(targetViewId, item.id);
      setFavorite(true);
      setChoosing(false);
      onChange?.(updated);
    } finally {
      setBusy(false);
    }
  }
  const label = favorite && viewId != null ? `Remove ${item.name} from this favorite view` : `Add ${item.name} to favorite view`;
  return <>
    <button type="button" className={`favorite-button ${favorite ? "active" : ""}`} aria-label={label} disabled={busy} onClick={toggle}>{favorite ? "★" : "☆"}</button>
    {choosing && <FavoriteViewChooser item={item} busy={busy} onAdd={addToView} onClose={() => setChoosing(false)}/>}
  </>;
}

function FavoriteViewChooser({item,busy,onAdd,onClose}:{item:Media; busy:boolean; onAdd:(viewId:ID)=>Promise<void>; onClose:()=>void}) {
  const [views, setViews] = useState<FavoriteView[]>([]);
  const [selected, setSelected] = useState<ID>(-1);
  const [newName, setNewName] = useState("");
  const [error, setError] = useState("");
  useEffect(() => {
    api.favoriteViews()
      .then(loaded => {
        setViews(loaded);
        setSelected(loaded[0]?.id ?? -1);
      })
      .catch(cause => setError((cause as Error).message));
  }, []);
  async function addExisting(event:FormEvent) {
    event.preventDefault();
    if (selected < 0) return;
    try {
      setError("");
      await onAdd(selected);
    } catch (cause) {
      setError((cause as Error).message);
    }
  }
  async function createAndAdd(event:FormEvent) {
    event.preventDefault();
    const name = newName.trim();
    if (!name) return;
    try {
      setError("");
      const created = await api.createFavoriteView(name);
      setViews(current => [...current, created].sort((left, right) => left.name.localeCompare(right.name)));
      setSelected(created.id);
      setNewName("");
      await onAdd(created.id);
    } catch (cause) {
      setError((cause as Error).message);
    }
  }
  return <div className="modal-backdrop" role="dialog" aria-modal="true" aria-label={`Add ${item.name} to favorite view`} onClick={event => closeOnBackdropClick(event, onClose)}>
    <div className="card settings modal favorite-picker">
      <div className="panel-title"><div><h2>Add to favorites</h2><p>{item.name}</p></div><button type="button" className="secondary" onClick={onClose}>Close</button></div>
      {error && <p className="error">{error}</p>}
      {views.length === 0 ? <p className="muted">No favorite views yet. Create one below.</p> :
        <form className="favorite-picker-form" onSubmit={addExisting}>
          <label>Favorite view<select value={selected} onChange={event => setSelected(Number(event.target.value))}>
            {views.map(view => <option key={view.id} value={view.id}>{view.name}</option>)}
          </select></label>
          <button type="submit" disabled={busy || selected < 0}>Add here</button>
        </form>}
      <form className="favorite-picker-form" onSubmit={createAndAdd}>
        <label>New favorite view<input value={newName} onChange={event => setNewName(event.target.value)} placeholder="Best photos"/></label>
        <button type="submit" disabled={busy || !newName.trim()}>Create and add</button>
      </form>
    </div>
  </div>;
}

function MediaViewerPage() {
  const {id="", folderId=""} = useParams();
  const libraryId = Number(id);
  const routeFolderId = Number(folderId);
  const [query] = useSearchParams();
  const navigate = useNavigate();
  const itemQuery = query.get("item");
  const hasItemQuery = itemQuery != null;
  const currentMediaId = Number(itemQuery ?? -1);
  const [items, setItems] = useState<Media[]>([]);
  const [fallbackItem, setFallbackItem] = useState<Media|null>(null);
  const [infoOpen, setInfoOpen] = useState(false);
  const folderMedia = items.filter(media => media.folderId === routeFolderId);
  const index = folderMedia.findIndex(media => media.id === currentMediaId);
  const item = index >= 0 ? folderMedia[index] : fallbackItem;
  const previous = index > 0 ? folderMedia[index - 1] : null;
  const next = index >= 0 && index < folderMedia.length - 1 ? folderMedia[index + 1] : null;
  useEffect(() => { if (Number.isFinite(libraryId)) api.libraryMedia(libraryId).then(setItems); }, [libraryId]);
  useEffect(() => { if (Number.isFinite(currentMediaId)) api.media(currentMediaId).then(setFallbackItem).catch(() => setFallbackItem(null)); }, [currentMediaId]);
  function go(media:Media|null) {
    if (media) navigate(libraryItemURL(libraryId, media));
  }
  useEffect(() => {
    function onKeyDown(event:KeyboardEvent) {
      if (isEditableTarget(event.target)) return;
      if (event.key === "ArrowLeft" && previous) {
        event.preventDefault();
        go(previous);
      } else if (event.key === "ArrowRight" && next) {
        event.preventDefault();
        go(next);
      }
    }
    window.addEventListener("keydown", onKeyDown);
    return () => window.removeEventListener("keydown", onKeyDown);
  }, [previous,next,libraryId,navigate]);
  if (!hasItemQuery) return <Navigate to={`/library/${libraryId}`} replace/>;
  if (!item) return <main className="viewer-page"><p>Loading media…</p></main>;
  return <main className="viewer-page">
    <Viewer item={item} infoOpen={infoOpen} previous={previous} next={next} onGo={go} onToggleInfo={() => setInfoOpen(value => !value)}/>
  </main>;
}

function libraryItemURL(libraryId:ID, item:Media) {
  const query = new URLSearchParams({item:String(item.id)});
  return `/library/${libraryId}/view/${item.folderId}?${query.toString()}`;
}

function isEditableTarget(target:EventTarget|null) {
  const element = target instanceof HTMLElement ? target : null;
  if (!element) return false;
  const tag = element.tagName.toLowerCase();
  return tag === "input" || tag === "textarea" || tag === "select" || element.isContentEditable;
}

function Viewer({item,favoriteViewId,infoOpen,previous,next,onGo,onToggleInfo}:{item:Media; favoriteViewId?:ID; infoOpen:boolean; previous:Media|null; next:Media|null; onGo:(media:Media|null)=>void; onToggleInfo:()=>void}) {
  const supported = supportedVideoCodecs();
  const [imageZoom, setImageZoom] = useState(1);
  const [imagePan, setImagePan] = useState({x:0, y:0});
  const [drag, setDrag] = useState<{pointerId:number; startX:number; startY:number; originX:number; originY:number}|null>(null);
  useEffect(() => { setImageZoom(1); setImagePan({x:0, y:0}); setDrag(null); }, [item.id]);
  useEffect(() => { if (imageZoom === 1) { setImagePan({x:0, y:0}); setDrag(null); } }, [imageZoom]);
  function adjustImageZoom(delta:number) {
    setImageZoom(value => clampZoom(Math.round((value + delta) * 100) / 100));
  }
  function onImageWheel(event:WheelEvent<HTMLDivElement>) {
    if (item.kind !== "image") return;
    event.preventDefault();
    adjustImageZoom(event.deltaY < 0 ? 0.25 : -0.25);
  }
  function startImagePan(event:ReactPointerEvent<HTMLImageElement>) {
    if (item.kind !== "image" || imageZoom <= 1) return;
    event.preventDefault();
    const point = eventPoint(event);
    event.currentTarget.setPointerCapture?.(event.pointerId);
    setDrag({pointerId:event.pointerId, startX:point.x, startY:point.y, originX:imagePan.x, originY:imagePan.y});
  }
  function moveImagePan(event:ReactPointerEvent<HTMLImageElement>) {
    if (!drag || drag.pointerId !== event.pointerId) return;
    const point = eventPoint(event);
    setImagePan({x:drag.originX + point.x - drag.startX, y:drag.originY + point.y - drag.startY});
  }
  function stopImagePan(event:ReactPointerEvent<HTMLImageElement>) {
    if (!drag || drag.pointerId !== event.pointerId) return;
    try {
      event.currentTarget.releasePointerCapture?.(event.pointerId);
    } catch {
      // Pointer capture may already be released by the browser.
    }
    setDrag(null);
  }
  return <div className="viewer-stage" aria-label={item.name}>
    <div className="viewer-media" onWheel={onImageWheel}>
      <FavoriteButton key={`favorite-${item.id}`} item={item} viewId={favoriteViewId}/>
      <button type="button" className="viewer-arrow viewer-arrow-left" aria-label="Previous media" disabled={!previous} onClick={() => onGo(previous)}>{"<"}</button>
      {item.kind === "video" ? <VideoPlayer key={`video-${item.id}`} item={item} supported={supported}/> :
        <img key={`image-${item.id}`} className={`${imageZoom > 1 ? "zoomed-image" : ""} ${drag ? "panning-image" : ""}`} style={{transform:`translate(${imagePan.x}px, ${imagePan.y}px) scale(${imageZoom})`}} src={api.contentUrl(item.id)} alt={item.name}
          onPointerDown={startImagePan} onPointerMove={moveImagePan} onPointerUp={stopImagePan} onPointerCancel={stopImagePan}/>}
      <button type="button" className="viewer-arrow viewer-arrow-right" aria-label="Next media" disabled={!next} onClick={() => onGo(next)}>{">"}</button>
      {item.kind === "image" && <div className="zoom-controls" aria-label="Image zoom controls">
        <button type="button" aria-label="Zoom out" disabled={imageZoom <= 0.5} onClick={() => adjustImageZoom(-0.25)}>−</button>
        <button type="button" aria-label="Reset zoom" disabled={imageZoom === 1} onClick={() => setImageZoom(1)}>{Math.round(imageZoom * 100)}%</button>
        <button type="button" aria-label="Zoom in" disabled={imageZoom >= 5} onClick={() => adjustImageZoom(0.25)}>+</button>
      </div>}
    </div>
    <button type="button" className="info-handle" aria-label={infoOpen ? "Hide info panel" : "Show info panel"} onClick={onToggleInfo}>{infoOpen ? ">>" : "<<"}</button>
    <aside className={`info-drawer ${infoOpen ? "open" : ""}`} aria-hidden={!infoOpen}>
      {infoOpen && <MediaInfo key={`info-${item.id}`} item={item}/>}
    </aside>
  </div>;
}

function clampZoom(value:number) {
  return Math.min(5, Math.max(0.5, value));
}

function eventPoint(event:ReactPointerEvent<HTMLImageElement>) {
  const native = event.nativeEvent;
  return {
    x: Number.isFinite(event.clientX) ? Number(event.clientX) : Number(native.pageX ?? 0),
    y: Number.isFinite(event.clientY) ? Number(event.clientY) : Number(native.pageY ?? 0),
  };
}

function MediaInfo({item}:{item:Media}) {
  const [name, setName] = useState(item.name);
  const [gpsValue, setGPSValue] = useState(item.gps ?? "");
  const [takenAt, setTakenAt] = useState(toDateTimeLocal(item.takenAt));
  const [current, setCurrent] = useState(item);
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);
  const [error, setError] = useState("");
  useEffect(() => {
    setName(item.name); setGPSValue(item.gps ?? ""); setTakenAt(toDateTimeLocal(item.takenAt)); setCurrent(item); setSaved(false); setError("");
  }, [item]);
  const trimmedName = name.trim();
  const trimmedGPS = gpsValue.trim();
  const dirty = trimmedName !== current.name || trimmedGPS !== (current.gps ?? "") || takenAt !== toDateTimeLocal(current.takenAt);
  async function saveDetails() {
    const trimmedName = name.trim();
    const trimmedGPS = gpsValue.trim();
    if (!trimmedName) {
      setError("name is required");
      return;
    }
    setSaving(true); setSaved(false); setError("");
    try {
      const updated = await api.updateMediaDetails(item.id, {name:trimmedName, gps:trimmedGPS, takenAt:takenAt});
      setCurrent(updated); setName(updated.name); setGPSValue(updated.gps ?? ""); setTakenAt(toDateTimeLocal(updated.takenAt)); setSaved(true);
    } catch (cause) {
      setError((cause as Error).message);
    } finally {
      setSaving(false);
    }
  }
  const gps = current.gps ? parseGPS(current.gps) : null;
  return <div className="info-panel"><h2>{current.name}</h2>
    <p>{current.kind} · {formatBytes(current.size)}</p>
    <div className="media-edit">
      <label>Name<input value={name} onChange={event => setName(event.target.value)} required/></label>
      <label>Date<input type="datetime-local" value={takenAt} onChange={event => setTakenAt(event.target.value)}/></label>
      <label>GPS<input value={gpsValue} onChange={event => setGPSValue(event.target.value)} placeholder="50.45,30.52"/></label>
      <div className="action-row">
        <button type="button" disabled={saving || !dirty} onClick={saveDetails}>{saving ? "Saving…" : "Save"}</button>
        {gps && <Link className="secondary-link" to={`/map?item=${current.id}`}>Open on map</Link>}
      </div>
      {error && <p className="error">{error}</p>}{saving && <p className="muted">Saving…</p>}{saved && !saving && <p className="success">Item saved.</p>}
    </div>
    {current.gps && gps && <Link className="gps-link" to={`/map?item=${current.id}`}>GPS: {current.gps}</Link>}
    {current.gps && !gps && <p>GPS: {current.gps}</p>}
    <MetadataSummary metadata={current.metadata}/>
  </div>;
}

function MetadataSummary({metadata}:{metadata:Record<string, unknown>}) {
  if (Object.keys(metadata ?? {}).length === 0) return <p className="muted">No camera metadata.</p>;
  return <div className="metadata-summary">
    <h3>Metadata</h3>
    {Object.entries(metadata).sort(([left], [right]) => left.localeCompare(right)).map(([key, value]) => <section className="metadata-json-section" key={key}>
      <h4>{key}</h4>
      <pre>{JSON.stringify(value, null, 2)}</pre>
    </section>)}
  </div>;
}

function toDateTimeLocal(value:string|undefined) {
  if (!value) return "";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "";
  const local = new Date(date.getTime() - date.getTimezoneOffset() * 60000);
  return local.toISOString().slice(0, 16);
}

function videoMetadataDuration(metadata:Record<string, unknown>): number {
  const ffprobe = metadata?.ffprobe as {format?:{duration?:unknown}}|undefined;
  const raw = ffprobe?.format?.duration;
  const value = typeof raw === "string" ? Number(raw) : raw;
  return typeof value === "number" && Number.isFinite(value) && value > 0 ? value : 0;
}

const TRANSCODE_SCHEMAS: Record<UserSettingsPayload["codec"], {videoLabel:string; audioLabel:string; containerLabel:string; supportLabel:string; compressionLabel:string; probe:string[]}> = {
  "h264-aac-mp4":  {videoLabel:"H.264", audioLabel:"AAC", containerLabel:"MP4", supportLabel:"Excellent", compressionLabel:"Good", probe:['video/mp4; codecs="avc1.42E01E,mp4a.40.2"']},
  "h264-opus-mp4": {videoLabel:"H.264", audioLabel:"Opus", containerLabel:"MP4", supportLabel:"Good, but less universal", compressionLabel:"Good", probe:['video/mp4; codecs="avc1.42E01E,opus"']},
  "vp9-opus-webm":  {videoLabel:"VP9", audioLabel:"Opus", containerLabel:"WebM", supportLabel:"Excellent", compressionLabel:"Very good", probe:['video/webm; codecs="vp9,opus"', 'video/webm; codecs="vp09.00.10.08,opus"']},
  "vp9-vorbis-webm":{videoLabel:"VP9", audioLabel:"Vorbis", containerLabel:"WebM", supportLabel:"Very good", compressionLabel:"Very good", probe:['video/webm; codecs="vp9,vorbis"']},
  "av1-opus-webm":  {videoLabel:"AV1", audioLabel:"Opus", containerLabel:"WebM", supportLabel:"Very good on modern devices", compressionLabel:"Excellent", probe:['video/webm; codecs="av01.0.04M.08,opus"']},
  "hevc-aac-mp4":   {videoLabel:"HEVC", audioLabel:"AAC", containerLabel:"MP4", supportLabel:"Platform-dependent", compressionLabel:"Excellent", probe:['video/mp4; codecs="hvc1,mp4a.40.2"', 'video/mp4; codecs="hev1,mp4a.40.2"']},
  "hevc-opus-mp4":  {videoLabel:"HEVC", audioLabel:"Opus", containerLabel:"MP4", supportLabel:"Poor / inconsistent", compressionLabel:"Excellent", probe:['video/mp4; codecs="hvc1,opus"', 'video/mp4; codecs="hev1,opus"']},
  "vp8-vorbis-webm":{videoLabel:"VP8", audioLabel:"Vorbis", containerLabel:"WebM", supportLabel:"Excellent", compressionLabel:"Fair", probe:['video/webm; codecs="vp8,vorbis"']},
  "vp8-opus-webm":  {videoLabel:"VP8", audioLabel:"Opus", containerLabel:"WebM", supportLabel:"Excellent", compressionLabel:"Fair", probe:['video/webm; codecs="vp8,opus"']}
};

function normalizeSchemaId(value:string): UserSettingsPayload["codec"] {
  if (value in TRANSCODE_SCHEMAS) return value as UserSettingsPayload["codec"];
  const legacy:Record<string, UserSettingsPayload["codec"]> = {h264:"h264-aac-mp4", h265:"hevc-aac-mp4", vp9:"vp9-opus-webm"};
  return legacy[value] ?? "h264-aac-mp4";
}

function codecLabel(codecName:string) {
  const names:Record<string,string> = {h264:"H.264", h265:"H.265 / HEVC", hevc:"H.265 / HEVC", vp9:"VP9", vp8:"VP8", av1:"AV1", mpeg1video:"MPEG-1", mpeg2video:"MPEG-2", msmpeg4v3:"MPEG-4 Part 2 (msmpeg4v3)", aac:"AAC", mp3:"MP3", mp2:"MP2", opus:"Opus", vorbis:"Vorbis", ac3:"AC-3", eac3:"E-AC-3", flac:"FLAC"};
  return names[codecName] ?? codecName;
}

const DIRECT_PLAY_AUDIO: Record<string,string[]> = {h264:["aac"], h265:["aac"], hevc:["aac"], vp9:["opus","vorbis"], vp8:["opus","vorbis"], av1:["opus"]};
const DIRECT_PLAY_CONTAINER: Record<string,string[]> = {h264:["video/mp4","video/x-m4v"], h265:["video/mp4","video/x-m4v"], hevc:["video/mp4","video/x-m4v"], vp9:["video/webm"], vp8:["video/webm"], av1:["video/webm"]};

function videoPlaybackReport(item:Media, supported:string[]): {mode:"original"|"transcoded"; reasons:string[]} {
  const streams = (item.metadata?.ffprobe as {streams?:Array<{codec_type?:string; codec_name?:string}>}|undefined)?.streams ?? [];
  const video = streams.find(stream => stream.codec_type === "video");
  const audio = streams.find(stream => stream.codec_type === "audio");
  const codecName = video?.codec_name ?? "";
  const sourceCodec = codecName === "h264" ? "h264" : codecName === "hevc" || codecName === "h265" ? "h265" : codecName === "vp9" ? "vp9" : codecName === "vp8" ? "vp8" : codecName === "av1" ? "av1" : "";
  const audioName = audio?.codec_name ?? "";
  const mime = (item.mimeType || "").split(";")[0].trim().toLowerCase();
  const reasons: string[] = [];
  if (!sourceCodec) {
    reasons.push(`Video codec "${codecLabel(codecName) || "unknown"}" is not supported by your browser.`);
  } else if (!supported.includes(sourceCodec)) {
    reasons.push(`Video codec "${codecLabel(sourceCodec)}" is not supported by your browser.`);
  }
  if (sourceCodec) {
    const expectedAudio = DIRECT_PLAY_AUDIO[sourceCodec] ?? [];
    if (audioName !== "" && !expectedAudio.includes(audioName)) {
      reasons.push(`Audio track is ${codecLabel(audioName)}, but ${codecLabel(sourceCodec)} video needs ${expectedAudio.map(codecLabel).join(" or ")} audio to play without transcoding.`);
    }
    const containerOk = (DIRECT_PLAY_CONTAINER[sourceCodec] ?? []).includes(mime);
    if (!containerOk) {
      reasons.push(`File container is ${mime || "unknown"}, but ${codecLabel(sourceCodec)} video needs a ${sourceCodec === "vp9" || sourceCodec === "vp8" || sourceCodec === "av1" ? "WebM" : "MP4"} container.`);
    }
  }
  return {mode: reasons.length > 0 ? "transcoded" : "original", reasons};
}

function VideoPlayer({item,supported}:{item:Media; supported:string[]}) {
  const videoRef = useRef<HTMLVideoElement|null>(null);
  const metadataDuration = videoMetadataDuration(item.metadata);
  const [thumbs, setThumbs] = useState<VideoThumbnail[]>([]);
  const [hover, setHover] = useState<VideoThumbnail|null>(null);
  const [hoverLeft, setHoverLeft] = useState(0);
  const [playing, setPlaying] = useState(true);
  const [current, setCurrent] = useState(0);
  const [duration, setDuration] = useState(metadataDuration);
  const [offsets, setOffsets] = useState<[number,number]>([0,0]);
  const [active, setActive] = useState(0);
  const [pending, setPending] = useState<number|null>(null);
  const videoRefs = [useRef<HTMLVideoElement|null>(null), useRef<HTMLVideoElement|null>(null)];
  const activeRef = useRef(0);
  const seekTimer = useRef<ReturnType<typeof setTimeout>|null>(null);
  const seekPending = useRef(false);
  const playback = videoPlaybackReport(item, supported);
  const transcoded = playback.mode === "transcoded";
  const [reportOpen, setReportOpen] = useState(false);
  const targetSlot = active === 0 ? 1 : 0;
  useEffect(() => { api.videoThumbnails(item.id).then(setThumbs).catch(() => setThumbs([])); }, [item.id]);
  useEffect(() => () => { if (seekTimer.current !== null) clearTimeout(seekTimer.current); }, []);
  useEffect(() => { activeRef.current = active; }, [active]);
  function activeVideo() {
    return videoRefs[activeRef.current].current;
  }
  function toggle() {
    const video = activeVideo();
    if (!video) return;
    if (video.paused) void video.play();
    else video.pause();
  }
  function seek(value:number) {
    if (!Number.isFinite(duration)) return;
    const clamped = Math.min(Math.max(value, 0), duration || value);
    setCurrent(clamped);
    if (!transcoded) {
      const video = activeVideo();
      if (video) video.currentTime = clamped;
      return;
    }
    if (seekTimer.current !== null) clearTimeout(seekTimer.current);
    seekPending.current = true;
    seekTimer.current = setTimeout(() => {
      const slot = activeRef.current === 0 ? 1 : 0;
      setPending(clamped);
      setOffsets(prev => { const next = [...prev] as [number,number]; next[slot] = clamped; return next; });
    }, 400);
  }
  function jump(delta:number) {
    seek(current + delta);
  }
  function stop() {
    const video = activeVideo();
    if (video) video.pause();
    if (seekTimer.current !== null) clearTimeout(seekTimer.current);
    seekPending.current = false;
    setPending(null);
    setCurrent(0);
    setPlaying(false);
  }
  function replay() {
    if (seekTimer.current !== null) clearTimeout(seekTimer.current);
    seekPending.current = false;
    setCurrent(0);
    setPlaying(true);
    if (!transcoded) {
      const video = activeVideo();
      if (video) { video.currentTime = 0; void video.play(); }
      return;
    }
    const slot = activeRef.current === 0 ? 1 : 0;
    setPending(0);
    setOffsets(prev => { const next = [...prev] as [number,number]; next[slot] = 0; return next; });
  }
  function hoverTimeline(event:MouseEvent<HTMLDivElement>) {
    if (thumbs.length === 0) return;
    const rect = event.currentTarget.getBoundingClientRect();
    const ratio = Math.min(0.999, Math.max(0, (event.clientX - rect.left) / rect.width));
    const second = duration > 0 ? ratio * duration : thumbs[Math.min(thumbs.length - 1, Math.floor(ratio * thumbs.length))].timeSeconds;
    const closest = thumbs.reduce((best, candidate) =>
      Math.abs(candidate.timeSeconds - second) < Math.abs(best.timeSeconds - second) ? candidate : best, thumbs[0]);
    setHover(closest);
    setHoverLeft(ratio * 100);
  }
  function renderVideo(slot:number) {
    const isActive = active === slot;
    const offset = offsets[slot];
    return <video key={`slot-${slot}-${offset}`} ref={videoRefs[slot]} src={api.playbackUrl(item.id, supported, offset)}
      style={isActive ? undefined : {visibility:"hidden", position:"absolute", top:0, left:0, width:"100%"}}
      preload="auto" muted={!isActive} autoPlay={isActive && playing} playsInline
      onCanPlay={() => {
        if (isActive) return;
        if (pending === null || offsets[slot] !== pending) return;
        seekPending.current = false;
        setPending(null);
        setActive(slot);
        const el = videoRefs[slot].current;
        if (!el) return;
        el.muted = false;
        if (playing) {
          try { void el.play(); } catch { /* ignore */ }
        } else el.pause();
      }}
      onLoadedMetadata={event => { const reported = event.currentTarget.duration; if (!(metadataDuration > 0) && Number.isFinite(reported) && reported > 0) setDuration(reported); }}
      onTimeUpdate={event => { if (!isActive || seekPending.current) return; const position = offset + event.currentTarget.currentTime; setCurrent(duration > 0 ? Math.min(position, duration) : position); }}
      onPlay={isActive ? () => setPlaying(true) : undefined}
      onPause={isActive ? () => setPlaying(false) : undefined}>
      Your browser cannot play this video.
    </video>;
  }
  return <div className="video-box">
    <div className="video-stack">
      {renderVideo(active)}
      {transcoded && pending !== null && renderVideo(targetSlot)}
    </div>
    {transcoded && <div className="video-badge-wrap">
      <button type="button" className="video-badge" aria-expanded={reportOpen} aria-controls="transcode-reasons"
        onClick={event => { event.stopPropagation(); setReportOpen(v => !v); }}>Transcoded ▾</button>
      {reportOpen && <div className="video-badge-popover" id="transcode-reasons" role="tooltip">
        <strong>Why transcoded?</strong>
        <ul>{playback.reasons.map(reason => <li key={reason}>{reason}</li>)}</ul>
      </div>}
    </div>}
    <div className="video-controls">
      <button type="button" onClick={toggle}>{playing ? "Pause" : "Play"}</button>
      <button type="button" onClick={stop}>Stop</button>
      <button type="button" onClick={replay}>Replay</button>
      <button type="button" onClick={() => jump(-10)}>−10s</button>
      <span>{formatTime(current)}</span>
      <div className="timeline" onMouseMove={hoverTimeline} onMouseLeave={() => setHover(null)}>
        <input aria-label="Seek video" type="range" min="0" max={duration || 0} step="0.1" value={Math.min(current, duration || current)}
          onChange={event => seek(Number(event.target.value))}/>
        {hover && <div className="timeline-preview" style={{left:`${hoverLeft}%`}}>
          <img src={api.thumbnailUrl(item.id, hover.index)} alt=""/><small>{formatTime(hover.timeSeconds)}</small>
        </div>}
      </div>
      <span>{duration ? formatTime(duration) : "0:00"}</span>
      <button type="button" onClick={() => jump(10)}>+10s</button>
    </div>
  </div>;
}

const VIDEO_FORMAT_PROBES: Record<string,{label:string; probe:string[]}> = {
  h264: {label:"MP4 — H.264 + AAC", probe:['video/mp4; codecs="avc1.42E01E,mp4a.40.2"', 'video/mp4; codecs="avc1.42E01E,opus"']},
  h265: {label:"MP4 — H.265 / HEVC + AAC", probe:['video/mp4; codecs="hvc1,mp4a.40.2"', 'video/mp4; codecs="hev1,mp4a.40.2"', 'video/mp4; codecs="hvc1,opus"', 'video/mp4; codecs="hev1,opus"']},
  vp9: {label:"WebM — VP9 + Opus", probe:['video/webm; codecs="vp9,opus"', 'video/webm; codecs="vp09.00.10.08,opus"', 'video/webm; codecs="vp9,vorbis"']},
  vp8: {label:"WebM — VP8 + Vorbis", probe:['video/webm; codecs="vp8,vorbis"', 'video/webm; codecs="vp8,opus"']},
  av1: {label:"WebM — AV1 + Opus", probe:['video/webm; codecs="av01.0.04M.08,opus"']}
};

function supportedVideoFormats() {
  const video = document.createElement("video");
  return Object.values(VIDEO_FORMAT_PROBES)
    .filter(entry => entry.probe.some(type => video.canPlayType(type) !== ""))
    .map(entry => entry.label);
}

function supportedVideoCodecs() {
  const video = document.createElement("video");
  return Object.entries(VIDEO_FORMAT_PROBES)
    .filter(([,entry]) => entry.probe.some(type => video.canPlayType(type) !== ""))
    .map(([codec]) => codec);
}

function loginErrorMessage(cause:unknown) {
  const message = cause instanceof Error ? cause.message : String(cause);
  if (message.includes("Expected JSON") || message.includes("Unexpected token") || message.includes("<html")) {
    return "API is not reachable from the login page. Check that the backend/gateway is running and /api is proxied to this project.";
  }
  if (message.includes("502") || message.includes("503") || message.includes("504")) {
    return "Backend is not reachable. Check that the API container is running.";
  }
  return message;
}

function useProgressiveReveal<T>(items:readonly T[], batch = 200): readonly T[] {
  const [count, setCount] = useState(() => Math.min(batch, items.length));
  const currentRef = useRef(count);
  useEffect(() => {
    let alive = true;
    let frame = 0;
    currentRef.current = Math.min(batch, items.length);
    setCount(currentRef.current);
    const step = () => {
      if (!alive) return;
      currentRef.current = Math.min(currentRef.current + batch, items.length);
      setCount(currentRef.current);
      if (currentRef.current < items.length) frame = requestAnimationFrame(step);
    };
    if (currentRef.current < items.length) frame = requestAnimationFrame(step);
    return () => { alive = false; cancelAnimationFrame(frame); };
  }, [items, batch]);
  return items.slice(0, count);
}

function GeoMap({theme}:{theme:"light"|"dark"}) {
  const [items, setItems] = useState<MapMedia[]>([]);
  const [pickedGPS, setPickedGPS] = useState("");
  const [copyStatus, setCopyStatus] = useState("");
  const [selectMode, setSelectMode] = useState(false);
  const [area, setArea] = useState<{bounds:L.LatLngBounds; items:MapMedia[]}|null>(null);
  const [rendering, setRendering] = useState(false);
  const [query] = useSearchParams();
  const libraryParam = query.get("library");
  const folderParam = query.get("folder");
  useEffect(() => {
    api.map(libraryParam ? Number(libraryParam) : undefined, folderParam ? Number(folderParam) : undefined).then(setItems);
  }, [libraryParam, folderParam]);
  const focused = items.find(item => item.id === Number(query.get("item")));
  const focusedGPS = focused ? parseGPS(focused.gps) : null;
  function selectArea(start:L.LatLng, end:L.LatLng) {
    const bounds = L.latLngBounds(start, end);
    const southWest = bounds.getSouthWest();
    const northEast = bounds.getNorthEast();
    api.map(
      libraryParam ? Number(libraryParam) : undefined,
      folderParam ? Number(folderParam) : undefined,
      {west:southWest.lng, south:southWest.lat, east:northEast.lng, north:northEast.lat}
    ).then(selected => setArea({bounds, items: sortMedia(selected, "desc")}));
  }
  function selectCluster(cluster:MapMedia[]) {
    const points = cluster.map(item => parseGPS(item.gps)).filter((point): point is [number,number] => point !== null);
    setArea({bounds: points.length > 0 ? L.latLngBounds(points) : L.latLngBounds([0,0],[0,0]), items: sortMedia(cluster, "desc")});
  }
  return <main className={`map-page ${area ? "panel-open" : ""}`} aria-label="Media map">
    <div className="map-stage">
      <MapContainer center={focusedGPS ?? [20,0]} zoom={focusedGPS ? 15 : 2} className="map">
        <TileLayer key={theme} attribution='&copy; OpenStreetMap contributors' url={theme === "dark" ? "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}.png" : "https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png"}/>
        <ScaleControl position="bottomleft" imperial={false}/>
        <MapItems items={items} focused={focused} pickedGPS={pickedGPS} copyStatus={copyStatus} selectMode={selectMode} onCopyStatus={setCopyStatus} onPick={value => { setPickedGPS(value); setCopyStatus(""); }} onSelectCluster={selectCluster} onRenderProgress={setRendering}/>
        <AreaSelector enabled={selectMode} onArea={selectArea}/>
        {area && <SelectionRectangle bounds={area.bounds}/>}
      </MapContainer>
      <div className="map-toolbar">
        <button className={`select-area-button ${selectMode ? "active" : ""}`} onClick={() => setSelectMode(value => !value)}>{selectMode ? "Cancel area selection" : "Select area"}</button>
        {area && <button className="secondary" onClick={() => setArea(null)}>Clear selection</button>}
        {rendering && <span className="map-render-status" role="status">Rendering markers…</span>}
      </div>
    </div>
    {area && <MapAreaPanel items={area.items} onClear={() => setArea(null)}/>}
  </main>;
}

function AreaSelector({enabled,onArea}:{enabled:boolean; onArea:(start:L.LatLng,end:L.LatLng)=>void}) {
  const map = useMap();
  const rectangleRef = useRef<L.Rectangle|null>(null);
  const dragRef = useRef<{start:L.LatLng; current:L.LatLng}|null>(null);
  function clearRectangle() {
    if (rectangleRef.current) {
      map.removeLayer(rectangleRef.current);
      rectangleRef.current = null;
    }
  }
  function finishDrag() {
    const drag = dragRef.current;
    if (!drag) return;
    onArea(drag.start, drag.current);
    dragRef.current = null;
    clearRectangle();
  }
  useEffect(() => {
    if (enabled) {
      map.dragging.disable();
      map.boxZoom.disable();
    } else {
      map.dragging.enable();
      map.boxZoom.enable();
      dragRef.current = null;
      clearRectangle();
    }
  }, [enabled, map]);
  useEffect(() => {
    window.addEventListener("mouseup", finishDrag);
    return () => window.removeEventListener("mouseup", finishDrag);
  });
  useMapEvents({
    mousedown: event => {
      if (!enabled) return;
      dragRef.current = {start:event.latlng, current:event.latlng};
      if (!rectangleRef.current) {
        rectangleRef.current = L.rectangle(L.latLngBounds(event.latlng, event.latlng), {color:"#1769e0", weight:2, dashArray:"6 6", fillOpacity:0.12}).addTo(map);
      }
    },
    mousemove: event => {
      if (!enabled || !dragRef.current) return;
      dragRef.current = {...dragRef.current, current:event.latlng};
      rectangleRef.current?.setBounds(L.latLngBounds(dragRef.current.start, event.latlng));
    },
    mouseup: finishDrag
  });
  return null;
}

function SelectionRectangle({bounds}:{bounds:L.LatLngBounds}) {
  const map = useMap();
  useEffect(() => {
    const layer = L.rectangle(bounds, {color:"#1769e0", weight:2, dashArray:"6 6", fillOpacity:0.1}).addTo(map);
    return () => { map.removeLayer(layer); };
  }, [bounds, map]);
  return null;
}

function MapAreaPanel({items,onClear}:{items:MapMedia[]; onClear:()=>void}) {
  const [sort, setSort] = useState<"desc"|"asc">("desc");
  const sorted = useMemo(() => sortMedia(items, sort), [items, sort]);
  const visible = useProgressiveReveal(sorted, 100);
  const groups = groupByDate(visible);
  return <aside className="map-timeline-panel" aria-label="Selected area">
    <div className="map-timeline-head">
      <strong>{sorted.length} {sorted.length === 1 ? "item" : "items"}</strong>
      <button className="secondary" onClick={() => setSort(value => value === "desc" ? "asc" : "desc")}>{sort === "desc" ? "Newest first" : "Oldest first"}</button>
      <button className="secondary" onClick={onClear}>Clear</button>
    </div>
    {visible.length < sorted.length && <p className="map-render-status inline">Loading {visible.length} of {sorted.length}…</p>}
    {sorted.length === 0 ? <div className="empty-state"><p>No media with GPS inside the selected area.</p></div> :
      <div className="timeline-grid map-area-grid">{groups.map(group =>
        <div className="timeline-group" key={group.label}>
          <span className="timeline-group-date">{group.label}</span>
          <span className="timeline-group-dot" aria-hidden="true"/>
          <div className="timeline-group-grid">{group.items.map(item =>
            <MapAreaItem key={item.id} item={item}/>
          )}</div>
        </div>
      )}</div>}
  </aside>;
}

function MapAreaItem({item}:{item:MapMedia}) {
  return <Link className="map-area-item" to={libraryItemURL(item.libraryId, item)} aria-label={`Open ${item.name} in folder`}>
    <span className="thumb-wrap"><ThumbImage src={api.thumbnailUrl(item.id)} kind={item.kind}/>{item.kind === "video" && <span className="play-badge" aria-hidden="true">▶</span>}</span>
    <small>{item.name}</small>
  </Link>;
}

function MapItems({items,focused,pickedGPS,copyStatus,selectMode,onCopyStatus,onPick,onSelectCluster,onRenderProgress}:{items:MapMedia[]; focused?:MapMedia; pickedGPS:string; copyStatus:string; selectMode:boolean; onCopyStatus:(status:string)=>void; onPick:(gps:string)=>void; onSelectCluster:(items:MapMedia[])=>void; onRenderProgress:(rendering:boolean)=>void}) {
  const map = useMap();
  const [zoom, setZoom] = useState(map.getZoom());
  useMapEvents({
    click: event => { if (!selectMode) onPick(formatGPS(event.latlng.lat, event.latlng.lng)); },
    zoomend: event => setZoom(event.target.getZoom())
  });
  useEffect(() => {
    const gps = focused ? parseGPS(focused.gps) : null;
    if (gps) {
      map.setView(gps, Math.max(map.getZoom(), 15));
      return;
    }
    const points = items.map(item => parseGPS(item.gps)).filter((point): point is [number,number] => point !== null);
    if (points.length === 1) {
      map.setView(points[0], Math.max(map.getZoom(), 13));
    } else if (points.length > 1) {
      map.fitBounds(points, {padding:[36,36], maxZoom:13});
    }
  }, [focused,items,map]);
  const clusters = useMemo(() => clusterMedia(items, zoom), [items, zoom]);
  const visibleClusters = useProgressiveReveal(clusters, 200);
  useEffect(() => {
    onRenderProgress(visibleClusters.length < clusters.length);
  }, [visibleClusters.length, clusters.length, onRenderProgress]);
  return <>{visibleClusters.map(cluster =>
    <Marker key={cluster.id} position={[cluster.lat, cluster.lng]} icon={cluster.items.length === 1 ? mediaPointIcon() : clusterIcon(cluster.items.length)} eventHandlers={{click: () => { if (!selectMode) onSelectCluster(cluster.items); }}}/>)}
    {pickedGPS && parseGPS(pickedGPS) && <Marker position={parseGPS(pickedGPS)!} icon={pickedPointIcon()}>
      <Popup><PickedPointPopup gps={pickedGPS} copyStatus={copyStatus} onCopyStatus={onCopyStatus}/></Popup>
    </Marker>}
  </>;
}

function PickedPointPopup({gps,copyStatus,onCopyStatus}:{gps:string; copyStatus:string; onCopyStatus:(status:string)=>void}) {
  async function copyPickedGPS() {
    try {
      await copyText(gps);
      onCopyStatus("Copied.");
    } catch {
      onCopyStatus("Could not copy automatically. Select and copy the field manually.");
    }
  }
  return <div className="map-item picked-point-popup">
    <strong>Picked point</strong>
    <small>Use this GPS for image/video location.</small>
    <input aria-label="Picked GPS coordinates" value={gps} readOnly onFocus={event => event.currentTarget.select()}/>
    <button type="button" className="secondary" onClick={copyPickedGPS}>Copy coordinates</button>
    {copyStatus && <small className={copyStatus === "Copied." ? "success" : "error"}>{copyStatus}</small>}
  </div>;
}

function clusterMedia(items:MapMedia[], zoom:number) {
  const cell = clusterCellSize(zoom);
  const grouped = new Map<string,{items:MapMedia[];lat:number;lng:number}>();
  for (const item of items) {
    const gps = parseGPS(item.gps);
    if (!gps) continue;
    const key = `${Math.round(gps[0]/cell)},${Math.round(gps[1]/cell)}`;
    const existing = grouped.get(key) ?? {items:[], lat:0, lng:0};
    existing.items.push(item); existing.lat += gps[0]; existing.lng += gps[1];
    grouped.set(key, existing);
  }
  return [...grouped.entries()].map(([id, group]) => ({
    id, items:group.items, lat:group.lat/group.items.length, lng:group.lng/group.items.length
  }));
}

function clusterCellSize(zoom:number) {
  if (zoom >= 16) return 0.0001;
  if (zoom >= 13) return 0.002;
  if (zoom >= 10) return 0.01;
  if (zoom >= 7) return 0.05;
  if (zoom >= 4) return 0.5;
  return 5;
}

function clusterIcon(count:number) {
  return L.divIcon({className:"cluster-marker", html:`<span>${count}</span>`, iconSize:[38,38], iconAnchor:[19,19]});
}

function pickedPointIcon() {
  return L.divIcon({className:"picked-point-marker", html:"<span></span>", iconSize:[28,28], iconAnchor:[14,14]});
}

function mediaPointIcon() {
  return L.divIcon({className:"media-point-marker", html:"<span></span>", iconSize:[30,30], iconAnchor:[15,15]});
}

function formatBytes(value:number) {
  if (value < 1024) return `${value} B`;
  if (value < 1048576) return `${(value/1024).toFixed(1)} KB`;
  return `${(value/1048576).toFixed(1)} MB`;
}

function formatTime(seconds:number) {
  const minutes = Math.floor(seconds / 60);
  const rest = Math.floor(seconds % 60).toString().padStart(2, "0");
  return `${minutes}:${rest}`;
}

function parseGPS(gps:string):[number,number]|null {
  const parts = gps.split(",").map(Number);
  return parts.length === 2 && parts.every(Number.isFinite) ? [parts[0],parts[1]] : null;
}

function formatGPS(lat:number, lng:number) {
  return `${Number(lat.toFixed(6))},${Number(lng.toFixed(6))}`;
}

async function copyText(value:string) {
  if (navigator.clipboard?.writeText) {
    await navigator.clipboard.writeText(value);
    return;
  }
  const input = document.createElement("textarea");
  input.value = value;
  input.style.position = "fixed";
  input.style.opacity = "0";
  document.body.appendChild(input);
  input.select();
  const copied = document.execCommand("copy");
  input.remove();
  if (!copied) throw new Error("copy failed");
}
