import { useEffect, useRef, useState } from 'react'
import { thumbURL, type Entry } from './api'
import { KindIcon, useLongPress } from './ui'
import { ChevronRight, EllipsisCircle, VideoGlyph } from './icons'
import { formatDate, formatSize, hasThumb, kindOf, parentOf } from './util'

const PAGE = 150

type Props = {
  entries: Entry[]
  view: 'grid' | 'list'
  showFolder?: boolean // search results: show where each file lives
  onOpen: (e: Entry) => void
  onMenu: (e: Entry) => void
  resetKey: string
}

/** Renders entries in pages so folders with thousands of files stay smooth. */
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
        <ul className="grid" key={resetKey}>
          {shown.map((e, i) => (
            <Tile key={e.path} entry={e} delay={i < 40 ? i : 0} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      ) : (
        <ul className="list" key={resetKey}>
          {shown.map((e) => (
            <Row key={e.path} entry={e} showFolder={showFolder} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      )}
      {count < entries.length && <div ref={sentinel} className="sentinel" aria-hidden="true" />}
      {count >= entries.length && <Summary entries={entries} />}
    </>
  )
}

/** "38 Photos, 3 Videos" footer, like the Photos app. */
function Summary({ entries }: { entries: Entry[] }) {
  const counts = { folder: 0, image: 0, video: 0, other: 0 }
  for (const e of entries) {
    const k = kindOf(e)
    if (k === 'folder' || k === 'image' || k === 'video') counts[k]++
    else counts.other++
  }
  const part = (n: number, one: string, many: string) => (n ? `${n.toLocaleString()} ${n === 1 ? one : many}` : null)
  const text = [
    part(counts.folder, 'Folder', 'Folders'),
    part(counts.image, 'Photo', 'Photos'),
    part(counts.video, 'Video', 'Videos'),
    part(counts.other, 'File', 'Files'),
  ]
    .filter(Boolean)
    .join(', ')
  return <p className="summary">{text}</p>
}

type ItemProps = { entry: Entry; onOpen: (e: Entry) => void; onMenu: (e: Entry) => void }

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
      <button
        className={'tile ' + style}
        onClick={() => !lp.consumed() && onOpen(entry)}
        aria-label={entry.name}
        title={entry.name}
        {...lp.handlers}
      >
        {style === 'photo' && <Thumb entry={entry} size={32} />}
        {style === 'video' && (
          <>
            <PlayCircle />
            <VideoGlyph className="badge" />
          </>
        )}
        {style === 'doc' && <KindIcon entry={entry} size={kind === 'folder' ? 68 : 52} />}
        {style !== 'photo' && <span className="tile-name">{entry.name}</span>}
      </button>
    </li>
  )
}

const PlayCircle = () => (
  <svg className="play" width="40" height="40" viewBox="0 0 40 40" aria-hidden="true">
    <circle cx="20" cy="20" r="19" fill="rgba(255,255,255,.22)" />
    <path d="m16 12.5 12 7.5-12 7.5z" fill="#fff" />
  </svg>
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
          <EllipsisCircle size={22} />
        </button>
      )}
    </li>
  )
}
