import { useEffect, useLayoutEffect, useRef, useState } from 'react'
import { downloadURL, thumbURL, type Entry } from './api'
import { useEscape } from './ui'
import { animateSpring, BOUNCY, rubberBand, SNAPPY, VelocityTracker, reducedMotion } from './spring'
import { ChevronLeft, ChevronRight, CloseIcon, DownloadIcon, ExternalIcon, MoreIcon } from './icons'
import { hasThumb, isHeic, kindOf, triggerDownload } from './util'

type Props = {
  items: Entry[]
  index: number
  onIndex: (i: number) => void
  onClose: () => void
  onMenu: (e: Entry) => void
}

type Gesture =
  | { kind: 'idle' }
  | { kind: 'pending'; x0: number; y0: number }
  | { kind: 'swipe'; x0: number }
  | { kind: 'dismiss'; x0: number; y0: number }
  | { kind: 'pan'; x0: number; y0: number; px: number; py: number; canTap: boolean }
  | { kind: 'pinch'; d0: number; s0: number; cx: number; cy: number; px: number; py: number }

const MAX_ZOOM = 5

/**
 * Full-screen viewer with iOS Photos gestures: swipe between items (the
 * photo follows your finger), drag down to close, pinch or double-tap to
 * zoom, tap to hide the bars.
 */
export function Viewer({ items, index, onIndex, onClose, onMenu }: Props) {
  const entry = items[index]
  const [chrome, setChrome] = useState(true)
  const root = useRef<HTMLDivElement>(null)
  const track = useRef<HTMLDivElement>(null)
  const current = useRef<HTMLDivElement>(null) // wrapper of the current slide (dismiss transform)
  const media = useRef<HTMLElement | null>(null) // current img (zoom transform)

  // Animated values, written straight to the DOM (no re-render per frame).
  const v = useRef({ x: 0, dy: 0, dx: 0, scale: 1, px: 0, py: 0, bg: 1 })
  const stops = useRef<(() => void)[]>([])
  const pointers = useRef(new Map<number, { x: number; y: number }>())
  const gesture = useRef<Gesture>({ kind: 'idle' })
  const tracker = useRef(new VelocityTracker())
  const lastTap = useRef<{ t: number; x: number; y: number } | null>(null)
  const tapTimer = useRef<number | undefined>(undefined)
  const busy = useRef(false) // a slide change is animating

  const width = () => root.current?.clientWidth || window.innerWidth
  const height = () => root.current?.clientHeight || window.innerHeight

  const render = () => {
    const s = v.current
    if (track.current) track.current.style.transform = `translate3d(${s.x}px,0,0)`
    if (current.current) {
      const shrink = 1 - Math.min(Math.max(s.dy, 0) / height(), 1) * 0.35
      current.current.style.transform = `translate3d(${s.dx}px,${s.dy}px,0) scale(${shrink})`
    }
    if (media.current) media.current.style.transform = `translate3d(${s.px}px,${s.py}px,0) scale(${s.scale})`
    if (root.current) root.current.style.setProperty('--bg-opacity', String(s.bg))
  }
  const stopAll = () => {
    stops.current.forEach((f) => f())
    stops.current = []
  }
  const spring = (key: keyof typeof v.current, to: number, velocity = 0, cfg = SNAPPY, done?: () => void) => {
    stops.current.push(
      animateSpring(v.current[key], to, velocity, cfg, (val) => {
        v.current[key] = val
        render()
      }, done),
    )
  }

  // New slide: reset zoom/position.
  useLayoutEffect(() => {
    v.current = { ...v.current, x: 0, dx: 0, dy: 0, scale: 1, px: 0, py: 0 }
    media.current = current.current?.querySelector('img.viewer-media, video.viewer-media') ?? null
    render()
  }, [index])

  // Opening animation.
  useLayoutEffect(() => {
    if (reducedMotion()) return
    v.current.bg = 0
    v.current.dy = 40
    render()
    spring('bg', 1, 0, SNAPPY)
    spring('dy', 0, 0, BOUNCY)
    return stopAll
  }, [])

  const close = () => {
    stopAll()
    setChrome(false)
    spring('bg', 0, 0, SNAPPY)
    spring('dy', height() * 0.6, 1200, { stiffness: 300, damping: 35 }, onClose)
  }
  useEscape(close)

  const go = (dir: -1 | 1, velocity = 0) => {
    const target = index + dir
    if (target < 0 || target >= items.length || busy.current) {
      spring('x', 0, velocity, BOUNCY)
      return
    }
    busy.current = true
    spring('x', -dir * width(), velocity, SNAPPY, () => {
      busy.current = false
      onIndex(target)
    })
  }

  useEffect(() => {
    const h = (e: KeyboardEvent) => {
      if (e.key === 'ArrowLeft') go(-1)
      if (e.key === 'ArrowRight') go(1)
    }
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  })

  // ---- zoom helpers ----

  const baseSize = () => {
    const m = media.current
    return { w: m?.offsetWidth || width(), h: m?.offsetHeight || height() }
  }
  const clampPan = (px: number, py: number, scale: number) => {
    const { w, h } = baseSize()
    const mx = Math.max(0, (w * scale - width()) / 2)
    const my = Math.max(0, (h * scale - height()) / 2)
    return { px: Math.max(-mx, Math.min(mx, px)), py: Math.max(-my, Math.min(my, py)), mx, my }
  }
  const zoomTo = (scale: number, cx: number, cy: number) => {
    // Keep the point under (cx, cy) fixed while zooming.
    const r = root.current!.getBoundingClientRect()
    const ox = cx - r.left - r.width / 2
    const oy = cy - r.top - r.height / 2
    const s = v.current
    const k = scale / s.scale
    const t = clampPan(ox - (ox - s.px) * k, oy - (oy - s.py) * k, scale)
    spring('scale', scale, 0, SNAPPY)
    spring('px', t.px, 0, SNAPPY)
    spring('py', t.py, 0, SNAPPY)
  }
  const isZoomed = () => v.current.scale > 1.01

  // ---- pointer handling ----

  const onPointerDown = (e: React.PointerEvent) => {
    const target = e.target as HTMLElement
    if (target.closest('button, a')) return
    const vid = target.closest('video')
    if (vid && e.clientY > vid.getBoundingClientRect().bottom - 72) return // leave the scrubber alone
    // Capturing on a video would swallow the taps that show its controls.
    if (!vid) {
      try {
        root.current?.setPointerCapture(e.pointerId)
      } catch {
        // capture is a nicety; gestures still work without it
      }
    }
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    stopAll()
    busy.current = false
    tracker.current.reset()
    tracker.current.add(e.clientX, e.clientY)

    if (pointers.current.size === 2) {
      const [a, b] = [...pointers.current.values()]
      const s = v.current
      gesture.current = {
        kind: 'pinch',
        d0: Math.hypot(a.x - b.x, a.y - b.y),
        s0: s.scale,
        cx: (a.x + b.x) / 2,
        cy: (a.y + b.y) / 2,
        px: s.px,
        py: s.py,
      }
    } else if (isZoomed()) {
      // Zoomed in: a drag pans, but a touch that barely moves is still a tap,
      // so a second double-tap can zoom back out.
      gesture.current = { kind: 'pan', x0: e.clientX, y0: e.clientY, px: v.current.px, py: v.current.py, canTap: true }
    } else {
      gesture.current = { kind: 'pending', x0: e.clientX - v.current.x, y0: e.clientY }
    }
  }

  const onPointerMove = (e: React.PointerEvent) => {
    if (!pointers.current.has(e.pointerId)) return
    pointers.current.set(e.pointerId, { x: e.clientX, y: e.clientY })
    tracker.current.add(e.clientX, e.clientY)
    const g = gesture.current
    const s = v.current

    if (g.kind === 'pending') {
      const dx = e.clientX - g.x0 - s.x
      const dy = e.clientY - g.y0
      if (Math.hypot(dx, dy) < 8) return
      gesture.current = Math.abs(dx) > Math.abs(dy) || dy < 0 ? { kind: 'swipe', x0: g.x0 } : { kind: 'dismiss', x0: e.clientX, y0: g.y0 }
      if (gesture.current.kind === 'dismiss') setChrome(false)
      return
    }
    if (g.kind === 'swipe') {
      let x = e.clientX - g.x0
      const atStart = index === 0 && x > 0
      const atEnd = index === items.length - 1 && x < 0
      if (atStart || atEnd) x = rubberBand(x, width())
      s.x = x
    } else if (g.kind === 'dismiss') {
      s.dy = Math.max(e.clientY - g.y0, rubberBand(e.clientY - g.y0, 200))
      s.dx = (e.clientX - g.x0) * 0.6
      s.bg = 1 - Math.min(Math.max(s.dy, 0) / (height() * 0.5), 1)
    } else if (g.kind === 'pan') {
      const t = clampPan(g.px + e.clientX - g.x0, g.py + e.clientY - g.y0, s.scale)
      const rx = g.px + e.clientX - g.x0
      const ry = g.py + e.clientY - g.y0
      s.px = t.px + rubberBand(rx - t.px, width())
      s.py = t.py + rubberBand(ry - t.py, height())
    } else if (g.kind === 'pinch' && pointers.current.size >= 2) {
      const [a, b] = [...pointers.current.values()]
      const d = Math.hypot(a.x - b.x, a.y - b.y)
      let scale = (g.s0 * d) / (g.d0 || 1)
      if (scale > MAX_ZOOM) scale = MAX_ZOOM + rubberBand(scale - MAX_ZOOM, 2)
      if (scale < 1) scale = 1 - rubberBand(1 - scale, 1, 0.8)
      const r = root.current!.getBoundingClientRect()
      const ox = g.cx - r.left - r.width / 2
      const oy = g.cy - r.top - r.height / 2
      const k = scale / g.s0
      s.scale = scale
      s.px = ox - (ox - g.px) * k + ((a.x + b.x) / 2 - g.cx)
      s.py = oy - (oy - g.py) * k + ((a.y + b.y) / 2 - g.cy)
    }
    render()
  }

  const onPointerUp = (e: React.PointerEvent) => {
    if (!pointers.current.has(e.pointerId)) return
    pointers.current.delete(e.pointerId)
    const g = gesture.current
    const vel = tracker.current.velocity()
    const s = v.current

    if (g.kind === 'pinch') {
      if (pointers.current.size === 1) {
        // One finger still down: continue as a pan.
        const [p] = [...pointers.current.values()]
        gesture.current = { kind: 'pan', x0: p.x, y0: p.y, px: s.px, py: s.py, canTap: false }
        return
      }
      const scale = Math.min(Math.max(s.scale, 1), MAX_ZOOM)
      const t = clampPan(s.px, s.py, scale)
      spring('scale', scale, 0, SNAPPY)
      spring('px', scale <= 1 ? 0 : t.px, 0, SNAPPY)
      spring('py', scale <= 1 ? 0 : t.py, 0, SNAPPY)
    } else if (g.kind === 'pan' && g.canTap && Math.hypot(e.clientX - g.x0, e.clientY - g.y0) < 10) {
      // Barely moved: snap back from any tiny nudge and count it as a tap.
      const t = clampPan(s.px, s.py, s.scale)
      spring('px', t.px, 0, SNAPPY)
      spring('py', t.py, 0, SNAPPY)
      handleTap(e.clientX, e.clientY)
    } else if (g.kind === 'pan') {
      // Glide with momentum, then settle inside the edges.
      const t = clampPan(s.px + vel.x * 0.25, s.py + vel.y * 0.25, s.scale)
      spring('px', t.px, vel.x, SNAPPY)
      spring('py', t.py, vel.y, SNAPPY)
    } else if (g.kind === 'swipe') {
      const w = width()
      if (s.x < -w * 0.25 || vel.x < -500) go(1, vel.x)
      else if (s.x > w * 0.25 || vel.x > 500) go(-1, vel.x)
      else spring('x', 0, vel.x, BOUNCY)
    } else if (g.kind === 'dismiss') {
      if (s.dy > 110 || vel.y > 700) {
        spring('bg', 0, 0, SNAPPY)
        spring('dy', height(), vel.y, SNAPPY, onClose)
      } else {
        setChrome(true)
        spring('dy', 0, vel.y, BOUNCY)
        spring('dx', 0, vel.x, BOUNCY)
        spring('bg', 1, 0, SNAPPY)
      }
    } else if (g.kind === 'pending') {
      handleTap(e.clientX, e.clientY)
    }
    gesture.current = { kind: 'idle' }
  }

  const handleTap = (x: number, y: number) => {
    const now = performance.now()
    const prev = lastTap.current
    if (prev && now - prev.t < 300 && Math.hypot(x - prev.x, y - prev.y) < 30) {
      // Double tap: zoom in on that point, or back out.
      window.clearTimeout(tapTimer.current)
      lastTap.current = null
      if (kindOf(entry) !== 'image') return
      if (isZoomed()) zoomTo(1, x, y)
      else zoomTo(2.5, x, y)
      return
    }
    lastTap.current = { t: now, x, y }
    tapTimer.current = window.setTimeout(() => setChrome((c) => !c), 260)
  }

  if (!entry) return null
  const slides = [index - 1, index, index + 1]

  return (
    <div
      ref={root}
      className={'viewer' + (chrome ? '' : ' no-chrome')}
      onPointerDown={onPointerDown}
      onPointerMove={onPointerMove}
      onPointerUp={onPointerUp}
      onPointerCancel={onPointerUp}
    >
      <div className="viewer-bg" />
      <div ref={track} className="viewer-track">
        {slides.map((i, pos) =>
          items[i] ? (
            <div key={items[i].path} className="viewer-slide" style={{ left: `${(pos - 1) * 100}%` }}>
              <div ref={pos === 1 ? current : undefined} className="viewer-slide-inner">
                <Media entry={items[i]} active={pos === 1} />
              </div>
            </div>
          ) : null,
        )}
      </div>

      <div className="viewer-top">
        <button className="glass-btn glass" aria-label="Close" onClick={close}>
          <CloseIcon size={20} />
        </button>
        <div className="viewer-title glass">
          <span className="viewer-name">{entry.name}</span>
          <span className="viewer-count">
            {index + 1} of {items.length}
          </span>
        </div>
        <button className="glass-btn glass" aria-label="More actions" onClick={() => onMenu(entry)}>
          <MoreIcon size={20} />
        </button>
      </div>

      <div className="viewer-bottom glass">
        <button aria-label="Download" onClick={() => triggerDownload(downloadURL(entry.path))}>
          <DownloadIcon size={22} />
        </button>
        {kindOf(entry) === 'image' && (
          <a href={downloadURL(entry.path, true)} target="_blank" rel="noopener" aria-label="Open original">
            <ExternalIcon size={22} />
          </a>
        )}
      </div>

      {index > 0 && (
        <button className="viewer-nav left glass" aria-label="Previous" onClick={() => go(-1)}>
          <ChevronLeft size={26} />
        </button>
      )}
      {index < items.length - 1 && (
        <button className="viewer-nav right glass" aria-label="Next" onClick={() => go(1)}>
          <ChevronRight size={26} />
        </button>
      )}
    </div>
  )
}

function Media({ entry, active }: { entry: Entry; active: boolean }) {
  const [failed, setFailed] = useState(false)
  const [loaded, setLoaded] = useState(false)
  const kind = kindOf(entry)

  if (failed) {
    return (
      <div className="viewer-fallback glass">
        <p>
          {isHeic(entry)
            ? "This browser can't show HEIC photos (iPhone format). Safari on iPhone, iPad and Mac can."
            : "This file can't be previewed here."}
        </p>
        <button className="pill accent" onClick={() => triggerDownload(downloadURL(entry.path))}>
          <DownloadIcon size={18} /> Download
        </button>
      </div>
    )
  }

  if (kind === 'video') {
    if (!active) return <div className="viewer-placeholder" />
    return (
      <video
        className="viewer-media"
        src={downloadURL(entry.path, true)}
        controls
        autoPlay
        playsInline
        preload="metadata"
        onError={() => setFailed(true)}
      />
    )
  }
  if (kind === 'audio') {
    if (!active) return <div className="viewer-placeholder" />
    return <audio className="viewer-audio" src={downloadURL(entry.path, true)} controls autoPlay onError={() => setFailed(true)} />
  }
  // Photos: the 1280px preview saves mobile data; others load the original.
  const src = hasThumb(entry) ? thumbURL(entry, 1280) : downloadURL(entry.path, true)
  return (
    <>
      {!loaded && hasThumb(entry) && <img className="viewer-blur" src={thumbURL(entry)} alt="" draggable={false} />}
      <img
        className={'viewer-media' + (loaded ? ' loaded' : '')}
        src={src}
        alt={entry.name}
        draggable={false}
        onLoad={() => setLoaded(true)}
        onError={() => setFailed(true)}
      />
    </>
  )
}
