import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, downloadURL, thumbURL, type Entry, type Info } from './api'
import { Browser } from './Browser'
import { Viewer } from './Viewer'
import { TextViewer } from './TextViewer'
import { UploadPanel, useUploads } from './Uploads'
import { ActionSheet, Alert, KindIcon, Menu, type MenuSection, type SheetAction } from './ui'
import {
  CameraIcon,
  ChevronLeft,
  ChevronRight,
  CloseIcon,
  DownloadIcon,
  EditIcon,
  GridIcon,
  ListIcon,
  MoreIcon,
  NewFolderIcon,
  PlusIcon,
  SearchIcon,
  TrashIcon,
  UploadIcon,
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

type DialogState = { kind: 'rename'; entry: Entry } | { kind: 'delete'; entry: Entry } | { kind: 'mkdir' } | null

export function App() {
  const [info, setInfo] = useState<Info | null>(null)
  const [path, setPath] = useState(() => pathFromHash(location.hash))
  const [direction, setDirection] = useState<'forward' | 'back' | 'none'>('none')
  const [entries, setEntries] = useState<Entry[] | null>(null)
  const [error, setError] = useState('')
  const [sort, setSort] = useState<SortKey>(() => load('burrow.sort', SORTS, 'name'))
  const [view, setView] = useState<'grid' | 'list'>(() => load('burrow.view', VIEWS, 'grid'))
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
  const [titleHidden, setTitleHidden] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const cameraInput = useRef<HTMLInputElement>(null)
  const searchInput = useRef<HTMLInputElement>(null)
  const largeTitle = useRef<HTMLHeadingElement>(null)
  const readOnly = info?.readOnly ?? true
  const rootName = info?.name ?? 'Burrow'

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
    api.info().then(setInfo, (e) => setError(e.message))
  }, [])

  useEffect(() => {
    refresh(path)
  }, [path, refresh])

  useEffect(() => {
    const onHash = () => {
      const next = pathFromHash(location.hash)
      const prev = pathRef.current
      setDirection(next.length > prev.length && next.startsWith(prev) ? 'forward' : 'back')
      setPath(next)
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

  const pathRef = useRef(path)
  pathRef.current = path
  const uploads = useUploads((dir) => {
    if (dir === pathRef.current) refresh(dir, true)
  })

  // ---- iOS navigation bar: glass once content scrolls under it, small
  // title once the large title has scrolled away ----

  useEffect(() => {
    const onScroll = () => setScrolled(window.scrollY > 2)
    onScroll()
    window.addEventListener('scroll', onScroll, { passive: true })
    return () => window.removeEventListener('scroll', onScroll)
  }, [])

  useEffect(() => {
    const el = largeTitle.current
    if (!el) return
    const io = new IntersectionObserver(([e]) => setTitleHidden(!e.isIntersecting), { rootMargin: '-60px 0px 0px 0px' })
    io.observe(el)
    return () => io.disconnect()
  }, [searching, path])

  // Keep the bottom bar above the on-screen keyboard.
  useEffect(() => {
    const vv = window.visualViewport
    if (!vv) return
    const update = () => {
      const inset = Math.max(0, window.innerHeight - vv.height - vv.offsetTop)
      document.documentElement.style.setProperty('--kb', `${inset}px`)
    }
    vv.addEventListener('resize', update)
    vv.addEventListener('scroll', update)
    return () => {
      vv.removeEventListener('resize', update)
      vv.removeEventListener('scroll', update)
    }
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
  const list = showResults ? (results ?? []) : sorted

  const viewSections: MenuSection[] = [
    {
      items: [
        { label: 'Grid', checked: view === 'grid', icon: <GridIcon size={18} />, onSelect: () => (setView('grid'), save('burrow.view', 'grid')) },
        { label: 'List', checked: view === 'list', icon: <ListIcon size={18} />, onSelect: () => (setView('list'), save('burrow.view', 'list')) },
      ],
    },
    {
      title: 'Sort by',
      items: SORTS.map((s) => ({
        label: { name: 'Name', newest: 'Date', largest: 'Size' }[s],
        checked: sort === s,
        onSelect: () => (setSort(s), save('burrow.sort', s)),
      })),
    },
  ]

  return (
    <div className={'app' + (searching ? ' is-searching' : '')}>
      <div className="ambient" aria-hidden="true" />

      <header className={'navbar' + (scrolled || searching ? ' scrolled' : '')}>
        <div className="nav-side">
          {path && !searching && (
            <button className="glass-btn glass" onClick={() => go(parentOf(path))} aria-label={`Back to ${parentName}`}>
              <ChevronLeft size={22} />
            </button>
          )}
          {!path && !searching && <img className="nav-logo" src="/icons/favicon-32.png" alt="" width={28} height={28} />}
        </div>
        <div className={'nav-title' + (titleHidden || searching ? ' shown' : '')}>{searching ? 'Search' : title}</div>
        <div className="nav-side right">
          {!searching && (
            <button
              className="glass-btn glass"
              aria-label="View and sort options"
              onClick={(e) => setViewMenu(e.currentTarget.getBoundingClientRect())}
            >
              <MoreIcon size={20} />
            </button>
          )}
        </div>
      </header>

      <main className={'content dir-' + direction} key={searching ? 'search' : path}>
        {!searching && (
          <div className="large-title">
            {path && (
              <button className="back-link" onClick={() => go(parentOf(path))}>
                <ChevronLeft size={16} />
                {parentName}
              </button>
            )}
            <h1 ref={largeTitle}>{title}</h1>
          </div>
        )}

        {error && (
          <div className="empty">
            <p>{error}</p>
            <button className="pill" onClick={() => refresh(path)}>
              Try again
            </button>
          </div>
        )}
        {!error && showResults && results !== null && !results.length && <p className="empty">No files named like “{query.trim()}”.</p>}
        {!error && searching && !query.trim() && <p className="empty hint">Search every file name in {rootName}.</p>}
        {!error && !searching && entries === null && <Skeleton />}
        {!error && !searching && entries !== null && !entries.length && (
          <div className="empty">
            <p>This folder is empty.</p>
            {!readOnly && (
              <button className="pill accent" onClick={() => fileInput.current?.click()}>
                <UploadIcon size={18} /> Upload files
              </button>
            )}
          </div>
        )}
        {!error && list.length > 0 && (!searching || showResults) && (
          <Browser
            entries={list}
            view={showResults ? 'list' : view}
            showFolder={showResults}
            resetKey={showResults ? 'search:' + query : path + ':' + sort + ':' + view}
            onOpen={(e) => open(e, list)}
            onMenu={setMenu}
          />
        )}
      </main>

      <div className="bottombar">
        <label className={'searchbar glass' + (searching ? ' active' : '')} onClick={() => !searching && startSearch()}>
          <SearchIcon size={18} />
          <input
            ref={searchInput}
            type="search"
            placeholder="Search"
            value={query}
            onFocus={() => setSearching(true)}
            onChange={(e) => setQuery(e.target.value)}
            enterKeyHint="search"
            aria-label="Search file names"
          />
          {query && (
            <button className="clear" aria-label="Clear search" onClick={() => setQuery('')}>
              <CloseIcon size={14} />
            </button>
          )}
        </label>
        {searching ? (
          <button className="glass-btn glass big" aria-label="Close search" onClick={endSearch}>
            <CloseIcon size={22} />
          </button>
        ) : (
          !readOnly && (
            <button className="glass-btn glass big accent" aria-label="Add files or folder" onClick={() => setAddMenu(true)}>
              <PlusIcon size={26} />
            </button>
          )
        )}
      </div>

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
        <div className="toast glass" key={toast} role="status">
          {toast}
        </div>
      )}
      {dragging && <div className="dropzone glass">Drop to upload to “{title}”</div>}

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

      {menu && <ActionSheet header={<SheetHeader entry={menu} />} title={menu.name} actions={actionsFor(menu)} onClose={() => setMenu(null)} />}

      {addMenu && (
        <ActionSheet
          title={`Add to ${title}`}
          actions={[
            { label: 'Take Photo or Video', icon: <CameraIcon size={20} />, onSelect: () => cameraInput.current?.click() },
            { label: 'Upload Files', icon: <UploadIcon size={20} />, onSelect: () => fileInput.current?.click() },
            { label: 'New Folder', icon: <NewFolderIcon size={20} />, onSelect: () => setDialog({ kind: 'mkdir' }) },
          ]}
          onClose={() => setAddMenu(false)}
        />
      )}

      {dialog?.kind === 'mkdir' && (
        <Alert
          title="New Folder"
          message={`Create a folder in “${title}”.`}
          confirmLabel="Create"
          input={{ initial: '', placeholder: 'Name' }}
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
          input={{ initial: dialog.entry.name, selectTo: dialog.entry.isDir ? undefined : splitExt(dialog.entry.name)[0].length }}
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
          title={`Delete “${dialog.entry.name}”?`}
          message={`It moves to Burrow's trash on your PC${dialog.entry.isDir ? ', with everything inside it' : ''}, so you can still recover it.`}
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

/** Thumbnail + name + details at the top of an item's action sheet. */
function SheetHeader({ entry }: { entry: Entry }) {
  return (
    <div className="sheet-header">
      <span className="sheet-thumb">{hasThumb(entry) ? <img src={thumbURL(entry)} alt="" /> : <KindIcon entry={entry} size={26} />}</span>
      <span className="sheet-header-text">
        <strong>{entry.name}</strong>
        <span>{[entry.isDir ? 'Folder' : formatSize(entry.size), formatDate(entry.modTime)].join(' · ')}</span>
      </span>
    </div>
  )
}

/** Shimmering placeholder tiles while a folder loads. */
function Skeleton() {
  return (
    <ul className="grid skeleton" aria-label="Loading">
      {Array.from({ length: 18 }, (_, i) => (
        <li key={i}>
          <span className="tile" />
        </li>
      ))}
    </ul>
  )
}
