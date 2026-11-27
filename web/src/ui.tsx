// Shared UI pieces: action sheet, dialogs, long-press, file icons.
import { useEffect, useRef, useState, type ReactNode } from 'react'
import type { Entry } from './api'
import { kindOf } from './util'
import { AudioIcon, FileIcon, FolderIcon, ImageIcon, PdfIcon, TextIcon, VideoIcon } from './icons'

export function KindIcon({ entry, size = 24 }: { entry: Entry; size?: number }) {
  switch (kindOf(entry)) {
    case 'folder':
      return <FolderIcon size={size} className="ic-folder" />
    case 'image':
      return <ImageIcon size={size} className="ic-image" />
    case 'video':
      return <VideoIcon size={size} className="ic-video" />
    case 'audio':
      return <AudioIcon size={size} className="ic-audio" />
    case 'pdf':
      return <PdfIcon size={size} className="ic-pdf" />
    case 'text':
      return <TextIcon size={size} className="ic-text" />
    default:
      return <FileIcon size={size} className="ic-file" />
  }
}

export type SheetAction = { label: string; icon?: ReactNode; danger?: boolean; onSelect: () => void }

/** Bottom sheet on phones, centred menu on desktop. */
export function ActionSheet({ title, actions, onClose }: { title?: string; actions: SheetAction[]; onClose: () => void }) {
  useEscape(onClose)
  return (
    <div className="backdrop" onClick={onClose}>
      <div className="sheet" role="menu" aria-label={title} onClick={(e) => e.stopPropagation()}>
        {title && <div className="sheet-title">{title}</div>}
        {actions.map((a) => (
          <button
            key={a.label}
            role="menuitem"
            className={'sheet-item' + (a.danger ? ' danger' : '')}
            onClick={() => {
              onClose()
              a.onSelect()
            }}
          >
            {a.icon}
            <span>{a.label}</span>
          </button>
        ))}
        <button className="sheet-item sheet-cancel" onClick={onClose}>
          Cancel
        </button>
      </div>
    </div>
  )
}

type DialogProps = {
  title: string
  message?: ReactNode
  confirmLabel: string
  danger?: boolean
  input?: { initial: string; selectTo?: number; placeholder?: string }
  onConfirm: (value: string) => Promise<void>
  onClose: () => void
}

/** Modal with an optional single text field. Shows server errors inline. */
export function Dialog({ title, message, confirmLabel, danger, input, onConfirm, onClose }: DialogProps) {
  const [value, setValue] = useState(input?.initial ?? '')
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const ref = useRef<HTMLInputElement>(null)
  useEscape(onClose)

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
      onClose()
    } catch (e) {
      setError(e instanceof Error ? e.message : String(e))
      setBusy(false)
    }
  }

  return (
    <div className="backdrop center" onClick={onClose}>
      <form className="dialog" role="dialog" aria-modal="true" aria-label={title} onClick={(e) => e.stopPropagation()} onSubmit={submit}>
        <h2>{title}</h2>
        {message && <p className="dialog-msg">{message}</p>}
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
        {error && <p className="dialog-error">{error}</p>}
        <div className="dialog-buttons">
          <button type="button" className="btn" onClick={onClose}>
            Cancel
          </button>
          <button type="submit" className={'btn primary' + (danger ? ' danger' : '')} disabled={busy}>
            {busy ? 'Working…' : confirmLabel}
          </button>
        </div>
      </form>
    </div>
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
        }, 500)
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
