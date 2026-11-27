import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, downloadURL, thumbURL, type Entry, type Info } from './api'
import { Browser, countsText, FolderCards } from './Browser'
import { Viewer } from './Viewer'
import { TextViewer } from './TextViewer'
import { UploadPanel, useUploads } from './Uploads'
import { ActionSheet, Alert, KindIcon, Menu, type MenuSection, type SheetAction } from './ui'
import {
  ArrowLeft,
  CameraIcon,
  ChevronRight,
  CloseIcon,
  DownloadIcon,
  EditIcon,
  FileIcon,
  FolderIcon,
  GridIcon,
  HomeIcon,
  ImageIcon,
  ListIcon,
  LogoutIcon,
  MenuIcon,
  NewFolderIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  UploadIcon,
  VideoIcon,
} from './icons'
import {
  formatDate,
  formatSize,
  hasThumb,
  hashFromPath,
  isMedia,
  joinPath,
  kindOf,
  load,
  parentOf,
  pathFromHash,
  save,
  sortEntries,
  splitExt,
  triggerDownload,
  type SortKey,
} from './util'

const SORTS = ['name', 'newest', 'largest'] as const
const VIEWS = ['grid', 'list'] as const

type Category = 'all' | 'photos' | 'videos' | 'docs' | 'folders'

const CATEGORIES: { key: Category; label: string; icon: React.ReactNode }[] = [
  { key: 'all', label: 'All', icon: <GridIcon size={22} /> },
  { key: 'photos', label: 'Photos', icon: <ImageIcon size={22} /> },
  { key: 'videos', label: 'Videos', icon: <VideoIcon size={22} /> },
  { key: 'docs', label: 'Docs', icon: <FileIcon size={22} /> },
  { key: 'folders', label: 'Folders', icon: <FolderIcon size={22} /> },
]

function inCategory(e: Entry, c: Category): boolean {
  const k = kindOf(e)
  switch (c) {
    case 'all':
      return true
    case 'photos':
      return k === 'image'
    case 'videos':
      return k === 'video'
    case 'folders':
      return k === 'folder'
    case 'docs':
      return k !== 'image' && k !== 'video' && k !== 'folder'
  }
}

type DialogState = { kind: 'rename'; entry: Entry } | { kind: 'delete'; entry: Entry } | { kind: 'mkdir' } | null

export function App({ info, onLogout }: { info: Info; onLogout: () => void }) {
  const [path, setPath] = useState(() => pathFromHash(location.hash))
  const [direction, setDirection] = useState<'forward' | 'back' | 'none'>('none')
  const [entries, setEntries] = useState<Entry[] | null>(null)
  const [error, setError] = useState('')
  const [sort, setSort] = useState<SortKey>(() => load('burrow.sort', SORTS, 'name'))
  const [view, setView] = useState<'grid' | 'list'>(() => load('burrow.view', VIEWS, 'grid'))
  const [category, setCategory] = useState<Category>('all')
  const [query, setQuery] = useState('')
  const [searching, setSearching] = useState(false)
  const [results, setResults] = useState<Entry[] | null>(null)
  const [viewer, setViewer] = useState<{ items: Entry[]; index: number } | null>(null)
  const [textEntry, setTextEntry] = useState<Entry | null>(null)
  const [menu, setMenu] = useState<Entry | null>(null)
  const [addMenu, setAddMenu] = useState(false)
  const [viewMenu, setViewMenu] = useState<DOMRect | null>(null)
  const [dialog, setDialog] = useState<DialogState>(null)
  const [toast, setToast] = useState('')
  const [dragging, setDragging] = useState(false)
  const [scrolled, setScrolled] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const cameraInput = useRef<HTMLInputElement>(null)
  const searchInput = useRef<HTMLInputElement>(null)
  const readOnly = info.readOnly
  const rootName = info.name

  const flash = useCallback((msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast((t) => (t === msg ? '' : t)), 3000)
  }, [])

  // ---- data ----

  const refresh = useCallback(async (p: string, quiet = false) => {
    if (!quiet) setEntries(null)
    setError('')
    try {
      const res = await api.list(p)
      setEntries(res.entries)
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
    }
  }, [])

  useEffect(() => {
    refresh(path)
  }, [path, refresh])

  const pathRef = useRef(path)
  pathRef.current = path

  useEffect(() => {
    const onHash = () => {
      const next = pathFromHash(location.hash)
      const prev = pathRef.current
      setDirection(next.length > prev.length && next.startsWith(prev) ? 'forward' : 'back')
      setPath(next)
      setCategory('all')
      setQuery('')
      setSearching(false)
      window.scrollTo(0, 0)
    }
    window.addEventListener('hashchange', onHash)
    return () => window.removeEventListener('hashchange', onHash)
  }, [])

  // Debounced search.
  useEffect(() => {
    const q = query.trim()
    if (!q) {
      setResults(null)
      return
    }
    const ctl = new AbortController()
    const t = window.setTimeout(() => {
      api.search(q, ctl.signal).then(
        (r) => setResults(r.results),
        (e) => !ctl.signal.aborted && flash(e.message),
      )
    }, 200)
    return () => {
      window.clearTimeout(t)
      ctl.abort()
    }
  }, [query, flash])

  const sorted = useMemo(() => (entries ? sortEntries(entries, sort) : []), [entries, sort])
  const available = useMemo(() => CATEGORIES.filter((c) => c.key === 'all' || sorted.some((e) => inCategory(e, c.key))), [sorted])
  const filtered = useMemo(() => sorted.filter((e) => inCategory(e, category)), [sorted, category])
  const folders = filtered.filter((e) => e.isDir)
  const files = filtered.filter((e) => !e.isDir)

  const uploads = useUploads((dir) => {
    if (dir === pathRef.current) refresh(dir, true)
  })

  // Header gets a shadow once content scrolls under it.
  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 4)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  // ---- navigation ----

  const go = (p: string) => {
    location.hash = hashFromPath(p)
  }

  // The phone's back button should close the viewer, not leave the folder.
  const overlayOpen = !!viewer || !!textEntry
  useEffect(() => {
    if (!overlayOpen) return
    history.pushState({ overlay: true }, '')
    const onPop = () => {
      setViewer(null)
      setTextEntry(null)
    }
    window.addEventListener('popstate', onPop)
    return () => window.removeEventListener('popstate', onPop)
  }, [overlayOpen])
  const closeOverlay = () => (history.state?.overlay ? history.back() : (setViewer(null), setTextEntry(null)))

  function open(e: Entry, siblings: Entry[]) {
    if (e.isDir) return go(e.path)
    const kind = kindOf(e)
    if (isMedia(e)) {
      const items = siblings.filter(isMedia)
      return setViewer({ items, index: Math.max(0, items.findIndex((x) => x.path === e.path)) })
    }
    if (kind === 'pdf') return void window.open(downloadURL(e.path, true), '_blank', 'noopener')
    if (kind === 'text') return setTextEntry(e)
    triggerDownload(downloadURL(e.path))
  }

  const startSearch = () => {
    setSearching(true)
    requestAnimationFrame(() => searchInput.current?.focus())
  }
  const endSearch = () => {
    setQuery('')
    setSearching(false)
    searchInput.current?.blur()
  }

  // ---- actions ----

  function actionsFor(e: Entry): SheetAction[] {
    const list: SheetAction[] = []
    if (e.isDir) list.push({ label: 'Open', icon: <ChevronRight size={20} />, onSelect: () => go(e.path) })
    if (!e.isDir) list.push({ label: 'Download', icon: <DownloadIcon size={20} />, onSelect: () => triggerDownload(downloadURL(e.path)) })
    if (!readOnly) {
      list.push({ label: 'Rename', icon: <EditIcon size={20} />, onSelect: () => setDialog({ kind: 'rename', entry: e }) })
      list.push({ label: 'Delete', icon: <TrashIcon size={20} />, danger: true, onSelect: () => setDialog({ kind: 'delete', entry: e }) })
    }
    return list
  }

  // After a rename/delete, keep the open viewer and search results in sync.
  function afterChange(oldPath: string, updated: Entry | null) {
    setViewer((v) => {
      if (!v) return v
      const items = updated ? v.items.map((x) => (x.path === oldPath ? updated : x)) : v.items.filter((x) => x.path !== oldPath)
      if (!items.length) return null
      return { items, index: Math.min(v.index, items.length - 1) }
    })
    setResults((r) => (r ? (updated ? r.map((x) => (x.path === oldPath ? updated : x)) : r.filter((x) => x.path !== oldPath)) : r))
    refresh(path, true)
  }

  function startUpload(files: FileList | File[] | null) {
    if (!files || !files.length || readOnly) return
    uploads.add(path, files)
  }

  // ---- drag & drop (desktop) ----

  useEffect(() => {
    if (readOnly) return
    let depth = 0
    const hasFiles = (e: DragEvent) => !!e.dataTransfer && Array.from(e.dataTransfer.types).includes('Files')
    const enter = (e: DragEvent) => {
      if (!hasFiles(e)) return
      depth++
      setDragging(true)
    }
    const leave = (e: DragEvent) => {
      if (!hasFiles(e)) return
      if (--depth <= 0) {
        depth = 0
        setDragging(false)
      }
    }
    const over = (e: DragEvent) => hasFiles(e) && e.preventDefault()
    const drop = (e: DragEvent) => {
      if (!hasFiles(e)) return
      e.preventDefault()
      depth = 0
      setDragging(false)
      startUpload(e.dataTransfer!.files)
    }
    window.addEventListener('dragenter', enter)
    window.addEventListener('dragleave', leave)
    window.addEventListener('dragover', over)
    window.addEventListener('drop', drop)
    return () => {
      window.removeEventListener('dragenter', enter)
      window.removeEventListener('dragleave', leave)
      window.removeEventListener('dragover', over)
      window.removeEventListener('drop', drop)
    }
  })

  // ---- render ----

  const crumbs = path ? path.split('/') : []
  const title = crumbs.length ? crumbs[crumbs.length - 1] : rootName
  const parentName = crumbs.length > 1 ? crumbs[crumbs.length - 2] : rootName
  const showResults = searching && query.trim() !== ''
  const mediaSiblings = showResults ? (results ?? []) : files

  const viewSections: MenuSection[] = [
    {
      title: 'View',
      items: [
        { label: 'Grid', checked: view === 'grid', icon: <GridIcon size={18} />, onSelect: () => (setView('grid'), save('burrow.view', 'grid')) },
        { label: 'List', checked: view === 'list', icon: <ListIcon size={18} />, onSelect: () => (setView('list'), save('burrow.view', 'list')) },
      ],
    },
    {
      title: 'Sort by',
      items: SORTS.map((s) => ({
        label: { name: 'Name', newest: 'Newest first', largest: 'Largest first' }[s],
        checked: sort === s,
        onSelect: () => (setSort(s), save('burrow.sort', s)),
      })),
    },
    {
      items: [{ label: 'Log out', icon: <LogoutIcon size={18} />, onSelect: onLogout }],
    },
  ]

  return (
    <div className={'app' + (searching ? ' is-searching' : '')}>
      <header className={'topbar' + (scrolled || searching ? ' scrolled' : '')}>
        <div className="topbar-row">
          {searching ? (
            <button className="icon-circle" aria-label="Close search" onClick={endSearch}>
              <ArrowLeft size={20} />
            </button>
          ) : path ? (
            <button className="icon-circle" aria-label={`Back to ${parentName}`} onClick={() => go(parentOf(path))}>
              <ArrowLeft size={20} />
            </button>
          ) : (
            <img className="brand-mark" src="/icons/favicon-32.png" alt="Burrow" width={36} height={36} />
          )}

          <label className={'searchpill' + (searching ? ' active' : '')} onClick={() => !searching && startSearch()}>
            <SearchIcon size={20} />
            {searching ? (
              <input
                ref={searchInput}
                type="search"
                placeholder="Search file names"
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                enterKeyHint="search"
                aria-label="Search file names"
              />
            ) : (
              <span className="searchpill-text">
                <strong>Search your files</strong>
                <span>in {title}</span>
              </span>
            )}
            {searching && query && (
              <button className="clear" aria-label="Clear search" onClick={() => setQuery('')}>
                <CloseIcon size={14} />
              </button>
            )}
          </label>
        </div>

        {!searching && available.length > 2 && (
          <nav className="chips" aria-label="Show">
            {available.map((c) => (
              <button key={c.key} className={'chip' + (category === c.key ? ' active' : '')} aria-pressed={category === c.key} onClick={() => setCategory(c.key)}>
                {c.icon}
                <span>{c.label}</span>
              </button>
            ))}
          </nav>
        )}
      </header>

      <main className={'content dir-' + direction} key={searching ? 'search' : path}>
        {!searching && (
          <div className="page-title">
            <h1>{title}</h1>
            {entries && entries.length > 0 && <p>{countsText(entries)}</p>}
          </div>
        )}

        {error && (
          <div className="empty">
            <p>{error}</p>
            <button className="btn secondary" onClick={() => refresh(path)}>
              Try again
            </button>
          </div>
        )}
        {!error && showResults && results !== null && !results.length && <p className="empty">No files named like “{query.trim()}”.</p>}
        {!error && searching && !query.trim() && <p className="empty hint">Search every file name in {rootName}.</p>}
        {!error && !searching && entries === null && <Skeleton />}
        {!error && !searching && entries !== null && !entries.length && (
          <div className="empty">
            <div className="empty-art">
              <FolderIcon size={40} />
            </div>
            <p>
              <strong>Nothing here yet</strong>
              <br />
              Upload photos and files from your phone.
            </p>
            {!readOnly && (
              <button className="btn primary" onClick={() => fileInput.current?.click()}>
                <UploadIcon size={18} /> Upload
              </button>
            )}
          </div>
        )}

        {!error && showResults && results && results.length > 0 && (
          <Browser entries={results} view="list" showFolder resetKey={'search:' + query} onOpen={(e) => open(e, mediaSiblings)} onMenu={setMenu} />
        )}

        {!error && !searching && folders.length > 0 && (
          <section>
            {files.length > 0 && <h2 className="section-title">Folders</h2>}
            <FolderCards folders={folders} onOpen={(e) => open(e, files)} onMenu={setMenu} />
          </section>
        )}
        {!error && !searching && files.length > 0 && (
          <section>
            {folders.length > 0 && <h2 className="section-title">{files.every(isMedia) ? 'Photos & videos' : 'Files'}</h2>}
            <Browser
              entries={files}
              view={view}
              resetKey={path + ':' + sort + ':' + view + ':' + category}
              onOpen={(e) => open(e, files)}
              onMenu={setMenu}
            />
          </section>
        )}
      </main>

      {!searching && (
        <nav className="tabbar" aria-label="Main">
          <button className={'tab' + (!path ? ' active' : '')} onClick={() => go('')}>
            <HomeIcon size={24} />
            <span>Home</span>
          </button>
          <button className="tab" onClick={startSearch}>
            <SearchIcon size={24} />
            <span>Search</span>
          </button>
          {!readOnly && (
            <button className="tab tab-add" aria-label="Upload or create" onClick={() => setAddMenu(true)}>
              <span className="tab-add-circle">
                <PlusIcon size={26} />
              </span>
            </button>
          )}
          <button className="tab" onClick={(e) => setViewMenu(e.currentTarget.getBoundingClientRect())}>
            <MenuIcon size={24} />
            <span>Menu</span>
          </button>
        </nav>
      )}

      <input ref={fileInput} type="file" multiple hidden onChange={(e) => (startUpload(e.target.files), (e.target.value = ''))} />
      <input
        ref={cameraInput}
        type="file"
        accept="image/*,video/*"
        capture="environment"
        hidden
        onChange={(e) => (startUpload(e.target.files), (e.target.value = ''))}
      />

      <UploadPanel items={uploads.items} onCancel={uploads.cancelAll} onClear={uploads.clearFinished} />
      {toast && (
        <div className="toast" key={toast} role="status">
          {toast}
        </div>
      )}
      {dragging && <div className="dropzone">Drop to upload to “{title}”</div>}

      {viewer && (
        <Viewer
          items={viewer.items}
          index={viewer.index}
          onIndex={(index) => setViewer((v) => (v ? { ...v, index } : v))}
          onClose={closeOverlay}
          onMenu={setMenu}
        />
      )}
      {textEntry && <TextViewer entry={textEntry} onClose={closeOverlay} />}

      {viewMenu && <Menu anchor={viewMenu} sections={viewSections} onClose={() => setViewMenu(null)} />}

      {menu && (
        <ActionSheet
          title={menu.name}
          message={[menu.isDir ? 'Folder' : formatSize(menu.size), formatDate(menu.modTime)].join(' · ')}
          leading={<SheetThumb entry={menu} />}
          actions={actionsFor(menu)}
          onClose={() => setMenu(null)}
        />
      )}

      {addMenu && (
        <ActionSheet
          title={`Add to ${title}`}
          message="Photos, videos, files or a new folder"
          actions={[
            { label: 'Take photo or video', icon: <CameraIcon size={20} />, onSelect: () => cameraInput.current?.click() },
            { label: 'Upload from this device', icon: <UploadIcon size={20} />, onSelect: () => fileInput.current?.click() },
            { label: 'New folder', icon: <NewFolderIcon size={20} />, onSelect: () => setDialog({ kind: 'mkdir' }) },
          ]}
          onClose={() => setAddMenu(false)}
        />
      )}

      {dialog?.kind === 'mkdir' && (
        <Alert
          title="New folder"
          message={`Create a folder in “${title}”.`}
          confirmLabel="Create"
          input={{ initial: '', placeholder: 'Folder name' }}
          onConfirm={async (name) => {
            await api.mkdir(joinPath(path, name))
            refresh(path, true)
          }}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === 'rename' && (
        <Alert
          title="Rename"
          confirmLabel="Save"
          input={{ initial: dialog.entry.name, selectTo: dialog.entry.isDir ? undefined : splitExt(dialog.entry.name)[0].length, placeholder: 'Name' }}
          onConfirm={async (name) => {
            if (name === dialog.entry.name) return
            const updated = await api.rename(dialog.entry.path, name)
            afterChange(dialog.entry.path, updated)
            flash(`Renamed to ${updated.name}`)
          }}
          onClose={() => setDialog(null)}
        />
      )}
      {dialog?.kind === 'delete' && (
        <Alert
          title="Delete?"
          message={`“${dialog.entry.name}” moves to Burrow's trash on your PC${dialog.entry.isDir ? ', with everything inside it' : ''}, so you can still recover it.`}
          confirmLabel="Delete"
          danger
          onConfirm={async () => {
            await api.remove(dialog.entry.path)
            afterChange(dialog.entry.path, null)
            if (dialog.entry.path === path) go(parentOf(path))
            flash(`Deleted ${dialog.entry.name}`)
          }}
          onClose={() => setDialog(null)}
        />
      )}
    </div>
  )
}

function SheetThumb({ entry }: { entry: Entry }) {
  return <span className="sheet-thumb">{hasThumb(entry) ? <img src={thumbURL(entry)} alt="" /> : <KindIcon entry={entry} size={30} />}</span>
}

/** Shimmering placeholders while a folder loads. */
function Skeleton() {
  return (
    <ul className="tiles skeleton" aria-label="Loading">
      {Array.from({ length: 12 }, (_, i) => (
        <li key={i}>
          <span className="tile" />
        </li>
      ))}
    </ul>
  )
}
