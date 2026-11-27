// Shared UI in the style of iOS system controls: action sheets, menus and
// alerts (with spring motion), long-press, and Files-app icons.
import { useEffect, useLayoutEffect, useRef, useState, type ReactNode } from 'react'
import { createPortal } from 'react-dom'
import type { Entry } from './api'
import { kindOf } from './util'
import { animateSpring, rubberBand, SNAPPY, VelocityTracker } from './spring'
import { CheckIcon, DocFill, FolderFill } from './icons'

const extLabel = (name: string) => {
  const i = name.lastIndexOf('.')
  return i > 0 ? name.slice(i + 1, i + 5).toUpperCase() : ''
}

/** Files-app style icon: blue folder, or a document page labelled with its type. */
export function KindIcon({ entry, size = 24 }: { entry: Entry; size?: number }) {
  const kind = kindOf(entry)
  if (kind === 'folder') return <FolderFill size={size} className="ic-folder" />
  return <DocFill size={size} className={'ic-doc ic-' + kind} label={extLabel(entry.name)} />
}
export type SheetAction = { label: string; danger?: boolean; onSelect: () => void }

const EXIT_MS = 220

/**
 * iOS action sheet: centred options in a rounded group, a separate Cancel
 * button. Springs up, and can be flicked down to dismiss.
 */
export function ActionSheet({
  title,
  message,
  actions,
  onClose,
}: {
  title?: string
  message?: string
  actions: SheetAction[]
  onClose: () => void
}) {
  const sheet = useRef<HTMLDivElement>(null)
  const backdrop = useRef<HTMLDivElement>(null)
  const y = useRef(0)
  const stop = useRef<() => void>(() => {})
  const drag = useRef<{ startY: number; tracker: VelocityTracker } | null>(null)
  const closing = useRef(false)

  const apply = (v: number) => {
    y.current = v
    const el = sheet.current
    if (!el) return
    el.style.transform = `translate3d(0, ${v}px, 0)`
    const h = el.offsetHeight || 1
    if (backdrop.current) backdrop.current.style.opacity = String(Math.max(0, 1 - v / h))
  }

  useLayoutEffect(() => {
    const h = sheet.current?.offsetHeight ?? 300
    apply(h)
    stop.current = animateSpring(h, 0, 0, SNAPPY, apply)
    return () => stop.current()
  }, [])

  const close = (velocity = 0, then?: () => void) => {
    if (closing.current) return
    closing.current = true
    stop.current()
    const h = sheet.current?.offsetHeight ?? 300
    stop.current = animateSpring(y.current, h + 20, Math.max(velocity, 600), { stiffness: 420, damping: 40 }, apply, () => {
      onClose()
      then?.()
    })
  }
  useEscape(() => close())

  const onPointerDown = (e: React.PointerEvent) => {
    if ((e.target as HTMLElement).closest('button') && e.pointerType === 'mouse') return
    stop.current()
    drag.current = { startY: e.clientY - Math.max(0, y.current), tracker: new VelocityTracker() }
    drag.current.tracker.add(0, e.clientY)
  }
  const onPointerMove = (e: React.PointerEvent) => {
    const d = drag.current
    if (!d) return
    d.tracker.add(0, e.clientY)
    const dy = e.clientY - d.startY
    if (Math.abs(dy) > 6) {
      try {
        sheet.current?.setPointerCapture(e.pointerId)
      } catch {
        // not fatal
      }
    }
    apply(dy < 0 ? rubberBand(dy, 200) : dy)
  }
  const onPointerUp = () => {
    const d = drag.current
    drag.current = null
    if (!d) return
    const vy = d.tracker.velocity().y
    const h = sheet.current?.offsetHeight ?? 300
    if (y.current > h * 0.35 || vy > 700) close(vy)
    else stop.current = animateSpring(y.current, 0, vy, SNAPPY, apply)
  }

  return createPortal(
    <div className="overlay">
      <div ref={backdrop} className="dim" onClick={() => close()} />
      <div
        ref={sheet}
        className="sheet"
        role="menu"
        aria-label={title}
        onPointerDown={onPointerDown}
        onPointerMove={onPointerMove}
        onPointerUp={onPointerUp}
        onPointerCancel={onPointerUp}
      >
        <div className="sheet-group material">
          {(title || message) && (
            <div className="sheet-title">
              {title && <strong>{title}</strong>}
              {message && <span>{message}</span>}
            </div>
          )}
          {actions.map((a) => (
            <button
              key={a.label}
              role="menuitem"
              className={'sheet-item' + (a.danger ? ' danger' : '')}
              onClick={() => {
                if (Math.abs(y.current) > 4) return
                close(0, a.onSelect)
              }}
            >
              {a.label}
            </button>
          ))}
        </div>
        <button className="sheet-cancel material" onClick={() => close()}>
          Cancel
        </button>
      </div>
    </div>,
    document.body,
  )
}

export type MenuItem = { label: string; checked?: boolean; icon?: ReactNode; onSelect: () => void }
export type MenuSection = { title?: string; items: MenuItem[] }

/** iOS pull-down menu that springs out of the button that opened it. */
export function Menu({ anchor, sections, onClose }: { anchor: DOMRect; sections: MenuSection[]; onClose: () => void }) {
  const [leaving, setLeaving] = useState(false)
  const close = (then?: () => void) => {
    if (leaving) return
    setLeaving(true)
    window.setTimeout(() => {
      onClose()
      then?.()
    }, EXIT_MS)
  }
  useEscape(() => close())

  // Open upwards from bottom toolbars, downwards from top ones.
  const fromBottom = anchor.top > window.innerHeight / 2
  const left = Math.min(Math.max(8, anchor.left), window.innerWidth - 258)
  const style: React.CSSProperties = fromBottom
    ? { left, bottom: window.innerHeight - anchor.top + 10, transformOrigin: `${anchor.left - left + anchor.width / 2}px bottom` }
    : { left, top: anchor.bottom + 10, transformOrigin: `${anchor.left - left + anchor.width / 2}px top` }

  return createPortal(
    <div className="overlay">
      <div className="dim clear" onClick={() => close()} />
      <div className={'menu material' + (leaving ? ' leaving' : '')} style={style} role="menu">
        {sections.map((s, i) => (
          <div key={i} className="menu-section">
            {s.title && <div className="menu-title">{s.title}</div>}
            {s.items.map((it) => (
              <button key={it.label} role="menuitemradio" aria-checked={it.checked} className="menu-item" onClick={() => close(it.onSelect)}>
                <span className="menu-check">{it.checked && <CheckIcon size={16} />}</span>
                <span className="menu-label">{it.label}</span>
                {it.icon}
              </button>
            ))}
          </div>
        ))}
      </div>
    </div>,
    document.body,
  )
}

type AlertProps = {
  title: string
  message?: ReactNode
  confirmLabel: string
  danger?: boolean
  input?: { initial: string; selectTo?: number; placeholder?: string }
  onConfirm: (value: string) => Promise<void>
  onClose: () => void
}

/** iOS-style alert: zooms in, optional text field, side-by-side buttons. */
export function Alert({ title, message, confirmLabel, danger, input, onConfirm, onClose }: AlertProps) {
  const [value, setValue] = useState(input?.initial ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [leaving, setLeaving] = useState(false)
  const ref = useRef<HTMLInputElement>(null)

  const close = () => {
    if (leaving) return
    setLeaving(true)
    window.setTimeout(onClose, EXIT_MS)
  }
  useEscape(close)

  useEffect(() => {
    const el = ref.current
    if (!el) return
    el.focus()
    el.setSelectionRange(0, input?.selectTo ?? el.value.length)
  }, [input?.selectTo])

  async function submit(ev: React.FormEvent) {
    ev.preventDefault()
    if (input && !value.trim()) return
    setBusy(true)
    setError('')
    try {
      await onConfirm(value.trim())
      close()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return createPortal(
    <div className="overlay center">
      <div className={'dim' + (leaving ? ' leaving' : '')} />
      <form className={'alert material' + (leaving ? ' leaving' : '')} role="alertdialog" aria-modal="true" aria-label={title} onSubmit={submit}>
        <div className="alert-body">
          <h2>{title}</h2>
          {message && <p>{message}</p>}
          {input && (
            <input
              ref={ref}
              value={value}
              placeholder={input.placeholder}
              onChange={(e) => setValue(e.target.value)}
              autoCapitalize="off"
              autoCorrect="off"
              spellCheck={false}
              enterKeyHint="done"
            />
          )}
          {error && <p className="alert-error">{error}</p>}
        </div>
        <div className="alert-buttons">
          <button type="button" onClick={close}>
            Cancel
          </button>
          <button type="submit" className={'strong' + (danger ? ' danger' : '')} disabled={busy}>
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </form>
    </div>,
    document.body,
  )
}

export function useEscape(fn: () => void) {
  const ref = useRef(fn)
  ref.current = fn
  useEffect(() => {
    const h = (e: KeyboardEvent) => e.key === 'Escape' && ref.current()
    window.addEventListener('keydown', h)
    return () => window.removeEventListener('keydown', h)
  }, [])
}

/**
 * Long-press (touch) and right-click (mouse) both open the item menu.
 * Returns handlers to spread on the element, plus a guard for onClick so a
 * long-press doesn't also count as a tap.
 */
export function useLongPress(onLong: () => void) {
  const timer = useRef<number | undefined>(undefined)
  const start = useRef<{ x: number; y: number } | null>(null)
  const fired = useRef(false)

  const clear = () => {
    window.clearTimeout(timer.current)
    start.current = null
  }

  return {
    handlers: {
      onPointerDown: (e: React.PointerEvent) => {
        if (e.pointerType === 'mouse') return
        fired.current = false
        start.current = { x: e.clientX, y: e.clientY }
        timer.current = window.setTimeout(() => {
          fired.current = true
          navigator.vibrate?.(10)
          onLong()
        }, 450)
      },
      onPointerMove: (e: React.PointerEvent) => {
        const s = start.current
        if (s && Math.hypot(e.clientX - s.x, e.clientY - s.y) > 10) clear()
      },
      onPointerUp: clear,
      onPointerCancel: clear,
      onContextMenu: (e: React.MouseEvent) => {
        e.preventDefault()
        if (!fired.current) onLong()
      },
    },
    /** Call at the start of onClick; returns true if the click should be ignored. */
    consumed: () => {
      const f = fired.current
      fired.current = false
      return f
    },
  }
}
