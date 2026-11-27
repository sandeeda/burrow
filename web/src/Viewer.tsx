import { useEffect, useRef, useState } from 'react'
import { downloadURL, thumbURL, type Entry } from './api'
import { useEscape } from './ui'
import { ChevronLeft, ChevronRight, CloseIcon, DownloadIcon, ExternalIcon, MoreIcon } from './icons'
import { hasThumb, isHeic, kindOf, triggerDownload } from './util'

type Props = {
  items: Entry[]
  index: number
  onIndex: (i: number) => void
  onClose: () => void
  onMenu: (e: Entry) => void
}

/** Full-screen viewer for photos, videos and audio. Swipe or use arrows. */
export function Viewer({ items, index, onIndex, onClose, onMenu }: Props) {
  const entry = items[index]
  const [chrome, setChrome] = useState(true)
  const touch = useRef<{ x: number; y: number; t: number } | null>(null)
  const hasPrev = index > 0
  const hasNext = index < items.length - 1
  const prev = () => hasPrev && onIndex(index - 1)
  const next = () => hasNext && onIndex(index + 1)

  useEscape(onClose)
  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') prev()
      if (e.key === 'ArrowRight') next()
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  // Preload neighbouring photos so swiping feels instant.
  useEffect(() => {
    for (const n of [items[index - 1], items[index + 1]]) {
      if (n && hasThumb(n)) new Image().src = thumbURL(n, 1280)
    }
  }, [items, index])

  if (!entry) return null
  const kind = kindOf(entry)

  function onTouchStart(e: React.TouchEvent) {
    if (e.touches.length !== 1 || (e.target as HTMLElement).closest('video,audio')) {
      touch.current = null
      return
    }
    touch.current = { x: e.touches[0].clientX, y: e.touches[0].clientY, t: Date.now() }
  }
  function onTouchEnd(e: React.TouchEvent) {
    const s = touch.current
    touch.current = null
    if (!s || (window.visualViewport && window.visualViewport.scale > 1.05)) return // zoomed in: let the user pan
    const dx = e.changedTouches[0].clientX - s.x
    const dy = e.changedTouches[0].clientY - s.y
    if (Math.abs(dx) > 50 && Math.abs(dx) > Math.abs(dy) * 1.5 && Date.now() - s.t < 800) {
      if (dx < 0) next()
      else prev()
    } else if (Math.abs(dy) > 90 && dy > 0 && Math.abs(dy) > Math.abs(dx) * 1.5) {
      onClose() // swipe down to close
    }
  }

  return (
    <div className={'viewer' + (chrome ? '' : ' no-chrome')} onTouchStart={onTouchStart} onTouchEnd={onTouchEnd}>
      <div className="viewer-bar">
        <button className="icon-btn" aria-label="Close" onClick={onClose}>
          <CloseIcon />
        </button>
        <div className="viewer-title">
          <span className="viewer-name">{entry.name}</span>
          <span className="viewer-count">
            {index + 1} / {items.length}
          </span>
        </div>
        {kind === 'image' && (
          <a className="icon-btn" href={downloadURL(entry.path, true)} target="_blank" rel="noopener" aria-label="Open original" title="Open original">
            <ExternalIcon />
          </a>
        )}
        <button className="icon-btn" aria-label="Download" title="Download" onClick={() => triggerDownload(downloadURL(entry.path))}>
          <DownloadIcon />
        </button>
        <button className="icon-btn" aria-label="More actions" onClick={() => onMenu(entry)}>
          <MoreIcon />
        </button>
      </div>

      <div className="viewer-stage" onClick={(e) => e.target === e.currentTarget && setChrome((c) => !c)}>
        <Media key={entry.path} entry={entry} onTap={() => setChrome((c) => !c)} />
      </div>

      {hasPrev && (
        <button className="viewer-nav left" aria-label="Previous" onClick={prev}>
          <ChevronLeft size={32} />
        </button>
      )}
      {hasNext && (
        <button className="viewer-nav right" aria-label="Next" onClick={next}>
          <ChevronRight size={32} />
        </button>
      )}
    </div>
  )
}

function Media({ entry, onTap }: { entry: Entry; onTap: () => void }) {
  const [failed, setFailed] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const kind = kindOf(entry)

  if (failed) {
    return (
      <div className="viewer-fallback">
        <p>
          {isHeic(entry)
            ? "This browser can't show HEIC photos (iPhone format). Safari on iPhone, iPad and Mac can."
            : "This file can't be previewed here."}
        </p>
        <button className="btn primary" onClick={() => triggerDownload(downloadURL(entry.path))}>
          <DownloadIcon size={18} /> Download
        </button>
      </div>
    )
  }

  if (kind === 'video') {
    return <video className="viewer-media" src={downloadURL(entry.path, true)} controls autoPlay playsInline preload="metadata" onError={() => setFailed(true)} />
  }
  if (kind === 'audio') {
    return <audio className="viewer-audio" src={downloadURL(entry.path, true)} controls autoPlay onError={() => setFailed(true)} />
  }
  // Photos: the 1280px preview saves mobile data; others load the original.
  const src = hasThumb(entry) ? thumbURL(entry, 1280) : downloadURL(entry.path, true)
  return (
    <>
      {!loaded && hasThumb(entry) && <img className="viewer-media placeholder" src={thumbURL(entry)} alt="" />}
      <img
        className={'viewer-media' + (loaded ? '' : ' loading')}
        src={src}
        alt={entry.name}
        draggable={false}
        onLoad={() => setLoaded(true)}
        onError={() => setFailed(true)}
        onClick={onTap}
      />
    </>
  )
}
