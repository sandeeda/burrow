import { useCallback, useEffect, useMemo, useRef, useState } from 'react'
import { api, downloadURL, type Entry, type Info } from './api'
import { Browser } from './Browser'
import { Viewer } from './Viewer'
import { TextViewer } from './TextViewer'
import { UploadPanel, useUploads } from './Uploads'
import { ActionSheet, Dialog, type SheetAction } from './ui'
import {
  CameraIcon,
  CloseIcon,
  DownloadIcon,
  EditIcon,
  GridIcon,
  ListIcon,
  NewFolderIcon,
  PlusIcon,
  SearchIcon,
  SortIcon,
  TrashIcon,
  UploadIcon,
} from './icons'
import {
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
const SORT_LABEL: Record<SortKey, string> = { name: 'Name', newest: 'Newest', largest: 'Largest' }

type DialogState =
  | { kind: 'rename'; entry: Entry }
  | { kind: 'delete'; entry: Entry }
  | { kind: 'mkdir' }
  | null

export function App() {
  const [info, setInfo] = useState<Info | null>(null)
  const [path, setPath] = useState(() => pathFromHash(location.hash))
  const [entries, setEntries] = useState<Entry[] | null>(null)
  const [error, setError] = useState('')
  const [sort, setSort] = useState<SortKey>(() => load('burrow.sort', SORTS, 'name'))
  const [view, setView] = useState<'grid' | 'list'>(() => load('burrow.view', VIEWS, 'grid'))
  const [query, setQuery] = useState('')
  const [searchOpen, setSearchOpen] = useState(false)
  const [results, setResults] = useState<Entry[] | null>(null)
  const [viewer, setViewer] = useState<{ items: Entry[]; index: number } | null>(null)
  const [textEntry, setTextEntry] = useState<Entry | null>(null)
  const [menu, setMenu] = useState<Entry | null>(null)
  const [addMenu, setAddMenu] = useState(false)
  const [dialog, setDialog] = useState<DialogState>(null)
  const [toast, setToast] = useState('')
  const [dragging, setDragging] = useState(false)
  const fileInput = useRef<HTMLInputElement>(null)
  const cameraInput = useRef<HTMLInputElement>(null)
  const readOnly = info?.readOnly ?? true

  const flash = useCallback((msg: string) => {
    setToast(msg)
    window.setTimeout(() => setToast((t) => (t === msg ? '' : t)), 3500)
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
      setPath(pathFromHash(location.hash))
      setQuery('')
      setSearchOpen(false)
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
    }, 250)
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

  // ---- navigation ----

  const go = (p: string) => {
    setQuery('')
    setSearchOpen(false)
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

  // ---- actions ----

  function actionsFor(e: Entry): SheetAction[] {
    const list: SheetAction[] = []
    if (!e.isDir) list.push({ label: 'Download', icon: <DownloadIcon size={20} />, onSelect: () => triggerDownload(downloadURL(e.path)) })
    if (!readOnly) {
      list.push({ label: 'Rename', icon: <EditIcon size={20} />, onSelect: () => setDialog({ kind: 'rename', entry: e }) })
      list.push({ label: 'Delete', icon: <TrashIcon size={20} />, danger: true, onSelect: () => setDialog({ kind: 'delete', entry: e }) })
    }
    return list
  }

  // After a rename/delete, keep the open viewer in sync.
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
  const searching = query.trim() !== ''
  const list = searching ? (results ?? []) : sorted
  const nextSort = () => {
    const s = SORTS[(SORTS.indexOf(sort) + 1) % SORTS.length]
    setSort(s)
    save('burrow.sort', s)
  }
  const toggleView = () => {
    const v = view === 'grid' ? 'list' : 'grid'
    setView(v)
    save('burrow.view', v)
  }

  return (
    <div className="app">
      <header className="top">
        {searchOpen ? (
          <div className="search">
            <SearchIcon size={20} />
            <input
              autoFocus
              type="search"
              placeholder="Search file names"
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              enterKeyHint="search"
              aria-label="Search file names"
            />
            <button
              className="icon-btn"
              aria-label="Close search"
              onClick={() => {
                setQuery('')
                setSearchOpen(false)
              }}
            >
              <CloseIcon size={20} />
            </button>
          </div>
        ) : (
          <>
            <button className="brand" onClick={() => go('')} aria-label="Go to top folder">
              <img src="/icons/favicon-32.png" alt="" width={26} height={26} />
              <span>{info?.name ?? 'Burrow'}</span>
            </button>
            <div className="top-actions">
              <button className="icon-btn" aria-label="Search" onClick={() => setSearchOpen(true)}>
                <SearchIcon />
              </button>
            </div>
          </>
        )}
      </header>

      {!searching && (
        <nav className="toolbar" aria-label="Folder">
          <ol className="crumbs">
            <li>
              <button onClick={() => go('')} aria-current={path === '' ? 'page' : undefined}>
                {info?.name ?? 'Home'}
              </button>
            </li>
            {crumbs.map((c, i) => {
              const p = crumbs.slice(0, i + 1).join('/')
              return (
                <li key={p}>
                  <button onClick={() => go(p)} aria-current={p === path ? 'page' : undefined}>
                    {c}
                  </button>
                </li>
              )
            })}
          </ol>
          <button className="chip" onClick={nextSort} aria-label={`Sort: ${SORT_LABEL[sort]}`}>
            <SortIcon size={16} />
            {SORT_LABEL[sort]}
          </button>
          <button className="icon-btn" onClick={toggleView} aria-label={view === 'grid' ? 'Show as list' : 'Show as grid'}>
            {view === 'grid' ? <ListIcon size={20} /> : <GridIcon size={20} />}
          </button>
        </nav>
      )}

      <main className="content">
        {error && (
          <div className="empty">
            <p>{error}</p>
            <button className="btn" onClick={() => refresh(path)}>
              Try again
            </button>
          </div>
        )}
        {!error && searching && results !== null && !results.length && <p className="empty">No files named like “{query.trim()}”.</p>}
        {!error && !searching && entries === null && <p className="empty">Loading…</p>}
        {!error && !searching && entries !== null && !entries.length && (
          <div className="empty">
            <p>This folder is empty.</p>
            {!readOnly && (
              <button className="btn primary" onClick={() => fileInput.current?.click()}>
                <UploadIcon size={18} /> Upload files
              </button>
            )}
          </div>
        )}
        {!error && list.length > 0 && (
          <Browser
            entries={list}
            view={searching ? 'list' : view}
            showFolder={searching}
            resetKey={searching ? 'search:' + query : path + ':' + sort}
            onOpen={(e) => open(e, list)}
            onMenu={setMenu}
          />
        )}
      </main>

      {!readOnly && !searching && (
        <button className="fab" aria-label="Add files or folder" onClick={() => setAddMenu(true)}>
          <PlusIcon size={28} />
        </button>
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
      {toast && <div className="toast">{toast}</div>}
      {dragging && <div className="dropzone">Drop to upload to “{path ? crumbs[crumbs.length - 1] : (info?.name ?? 'Burrow')}”</div>}

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

      {menu && <ActionSheet title={menu.name} actions={[...(menu.isDir ? [{ label: 'Open', onSelect: () => go(menu.path) }] : []), ...actionsFor(menu)]} onClose={() => setMenu(null)} />}

      {addMenu && (
        <ActionSheet
          actions={[
            { label: 'Upload files', icon: <UploadIcon size={20} />, onSelect: () => fileInput.current?.click() },
            { label: 'Take photo or video', icon: <CameraIcon size={20} />, onSelect: () => cameraInput.current?.click() },
            { label: 'New folder', icon: <NewFolderIcon size={20} />, onSelect: () => setDialog({ kind: 'mkdir' }) },
          ]}
          onClose={() => setAddMenu(false)}
        />
      )}

      {dialog?.kind === 'mkdir' && (
        <Dialog
          title="New folder"
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
        <Dialog
          title={`Rename ${dialog.entry.isDir ? 'folder' : 'file'}`}
          confirmLabel="Rename"
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
        <Dialog
          title={`Delete “${dialog.entry.name}”?`}
          message={
            <>
              It moves to Burrow's trash on your PC{dialog.entry.isDir ? ', with everything inside it' : ''}. You can still recover it from there.
            </>
          }
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
