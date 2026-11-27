import { useEffect, useRef, useState } from 'react'
import { thumbURL, type Entry } from './api'
import { KindIcon, useLongPress } from './ui'
import { MoreIcon, PlayBadge } from './icons'
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
        <ul className="grid">
          {shown.map((e) => (
            <Tile key={e.path} entry={e} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      ) : (
        <ul className="list">
          {shown.map((e) => (
            <Row key={e.path} entry={e} showFolder={showFolder} onOpen={onOpen} onMenu={onMenu} />
          ))}
        </ul>
      )}
      {count < entries.length && <div ref={sentinel} className="sentinel" aria-hidden="true" />}
    </>
  )
}

type ItemProps = { entry: Entry; onOpen: (e: Entry) => void; onMenu: (e: Entry) => void }

function Thumb({ entry, size }: { entry: Entry; size: number }) {
  const [failed, setFailed] = useState(false)
  if (hasThumb(entry) && !failed) {
    return <img src={thumbURL(entry)} alt="" loading="lazy" decoding="async" draggable={false} onError={() => setFailed(true)} />
  }
  return <KindIcon entry={entry} size={size} />
}

function Tile({ entry, onOpen, onMenu }: ItemProps) {
  const lp = useLongPress(() => onMenu(entry))
  const kind = kindOf(entry)
  return (
    <li>
      <button
        className={'tile' + (hasThumb(entry) ? ' has-thumb' : '')}
        onClick={() => !lp.consumed() && onOpen(entry)}
        title={entry.name}
        {...lp.handlers}
      >
        <span className="tile-media">
          <Thumb entry={entry} size={40} />
          {kind === 'video' && <PlayBadge className="badge" />}
        </span>
        <span className="tile-name">{entry.name}</span>
      </button>
    </li>
  )
}

function Row({ entry, showFolder, onOpen, onMenu }: ItemProps & { showFolder?: boolean }) {
  const lp = useLongPress(() => onMenu(entry))
  const meta = [entry.isDir ? null : formatSize(entry.size), formatDate(entry.modTime)].filter(Boolean).join(' · ')
  const folder = parentOf(entry.path)
  return (
    <li className="row">
      <button className="row-main" onClick={() => !lp.consumed() && onOpen(entry)} {...lp.handlers}>
        <span className="row-icon">
          <Thumb entry={entry} size={26} />
        </span>
        <span className="row-text">
          <span className="row-name">{entry.name}</span>
          <span className="row-meta">
            {showFolder && <span className="row-folder">{folder ? `in ${folder}` : 'in top folder'} · </span>}
            {meta}
          </span>
        </span>
      </button>
      <button className="icon-btn" aria-label={`More actions for ${entry.name}`} onClick={() => onMenu(entry)}>
        <MoreIcon />
      </button>
    </li>
  )
}
