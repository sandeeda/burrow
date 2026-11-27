import { useEffect, useRef, useState } from 'react'
import { thumbURL, type Entry } from './api'
import { KindIcon, useLongPress } from './ui'
import { ChevronRight, MoreIcon } from './icons'
import { formatDate, formatSize, hasThumb, kindOf, parentOf } from './util'

const PAGE = 150

/** "12 photos · 3 videos · 2 folders" */
export function countsText(entries: Entry[]): string {
  const n = { folder: 0, image: 0, video: 0, other: 0 }
  for (const e of entries) {
    const k = kindOf(e)
    if (k === 'folder' || k === 'image' || k === 'video') n[k]++
    else n.other++
  }
  const part = (c: number, one: string, many: string) => (c ? `${c.toLocaleString()} ${c === 1 ? one : many}` : null)
  return (
    [part(n.image, 'photo', 'photos'), part(n.video, 'video', 'videos'), part(n.other, 'file', 'files'), part(n.folder, 'folder', 'folders')]
      .filter(Boolean)
      .join(' · ') || 'Empty'
  )
}

type ItemProps = { entry: Entry; onOpen: (e: Entry) => void; onMenu: (e: Entry) => void }

/** Folders as big rounded cards. */
export function FolderCards({ folders, onOpen, onMenu }: { folders: Entry[]; onOpen: (e: Entry) => void; onMenu: (e: Entry) => void }) {
  return (
    <ul className="cards">
      {folders.map((f, i) => (
        <FolderCard key={f.path} entry={f} delay={Math.min(i, 12)} onOpen={onOpen} onMenu={onMenu} />
      ))}
    </ul>
  )
}

function FolderCard({ entry, delay, onOpen, onMenu }: ItemProps & { delay: number }) {
  const lp = useLongPress(() => onMenu(entry))
  return (
    <li style={{ '--i': delay } as React.CSSProperties}>
      <button className="card" onClick={() => !lp.consumed() && onOpen(entry)} {...lp.handlers}>
        <span className="card-cover">
          <KindIcon entry={entry} size={56} />
        </span>
        <span className="card-title">{entry.name}</span>
        <span className="card-sub">{formatDate(entry.modTime)}</span>
      </button>
    </li>
  )
}

type Props = {
  entries: Entry[]
  view: 'grid' | 'list'
  showFolder?: boolean // search results: show where each file lives
  onOpen: (e: Entry) => void
  onMenu: (e: Entry) => void
  resetKey: string
}

/** Files as a tile grid or a list, rendered in pages so huge folders stay smooth. */
export function Browser({ entries, view, showFolder, onOpen, onMenu, resetKey }: Props) {
  const [count, setCount] = useState(PAGE)
  const sentinel = useRef<HTMLDivElement>(null)

  useEffect(() => setCount(PAGE), [resetKey])

  useEffect(() => {
    const el = sentinel.current
    if (!el) return
    const io = new IntersectionObserver(
      (items) => {
        if (items.some((i) => i.isIntersecting)) setCount((c) => c + PAGE)
      },
      { rootMargin: '1200px' },
    )
    io.observe(el)
    return () => io.disconnect()
  }, [count, entries.length])

  const shown = entries.slice(0, count)
  return (
    <>
      {view === 'grid' ? (
        <ul className="tiles" key={resetKey}>
          {shown.map((e, i) => (
            <Tile key={e.path} entry={e} delay={i < 24 ? i : 0} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      ) : (
        <ul className="rows" key={resetKey}>
          {shown.map((e) => (
            <Row key={e.path} entry={e} showFolder={showFolder} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      )}
      {count < entries.length && <div ref={sentinel} className="sentinel" aria-hidden="true" />}
    </>
  )
}

function Thumb({ entry, size }: { entry: Entry; size: number }) {
  const [state, setState] = useState<'loading' | 'ok' | 'failed'>('loading')
  if (hasThumb(entry) && state !== 'failed') {
    return (
      <img
        className={state === 'ok' ? 'in' : ''}
        src={thumbURL(entry)}
        alt=""
        loading="lazy"
        decoding="async"
        draggable={false}
        onLoad={() => setState('ok')}
        onError={() => setState('failed')}
      />
    )
  }
  return <KindIcon entry={entry} size={size} />
}

function Tile({ entry, delay, onOpen, onMenu }: ItemProps & { delay: number }) {
  const lp = useLongPress(() => onMenu(entry))
  const kind = kindOf(entry)
  const photo = hasThumb(entry)
  const style = photo ? 'photo' : kind === 'video' ? 'video' : 'doc'
  return (
    <li style={{ '--i': delay } as React.CSSProperties}>
      <button className={'tile ' + style} onClick={() => !lp.consumed() && onOpen(entry)} aria-label={entry.name} title={entry.name} {...lp.handlers}>
        {style === 'photo' && <Thumb entry={entry} size={32} />}
        {style === 'video' && <PlayButton />}
        {style === 'doc' && <KindIcon entry={entry} size={44} />}
        {style !== 'photo' && <span className="tile-name">{entry.name}</span>}
      </button>
    </li>
  )
}

const PlayButton = () => (
  <span className="play" aria-hidden="true">
    <svg width="16" height="16" viewBox="0 0 16 16">
      <path d="M5 3.2v9.6a.8.8 0 0 0 1.2.7l7.4-4.8a.8.8 0 0 0 0-1.4L6.2 2.5A.8.8 0 0 0 5 3.2z" fill="currentColor" />
    </svg>
  </span>
)

function Row({ entry, showFolder, onOpen, onMenu }: ItemProps & { showFolder?: boolean }) {
  const lp = useLongPress(() => onMenu(entry))
  const meta = [entry.isDir ? null : formatSize(entry.size), formatDate(entry.modTime)].filter(Boolean).join(' · ')
  const folder = parentOf(entry.path)
  return (
    <li className="row">
      <button className="row-main" onClick={() => !lp.consumed() && onOpen(entry)} {...lp.handlers}>
        <span className="row-icon">
          <Thumb entry={entry} size={30} />
        </span>
        <span className="row-text">
          <span className="row-name">{entry.name}</span>
          <span className="row-meta">
            {showFolder && <span>{folder ? `in ${folder}` : 'in top folder'} · </span>}
            {meta}
          </span>
        </span>
        {entry.isDir && <ChevronRight size={18} className="row-chevron" />}
      </button>
      {!entry.isDir && (
        <button className="row-more" aria-label={`More actions for ${entry.name}`} onClick={() => onMenu(entry)}>
          <MoreIcon size={20} />
        </button>
      )}
    </li>
  )
}
