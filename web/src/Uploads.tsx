import { useCallback, useRef, useState } from 'react'
import { upload, type Entry, type UploadHandle } from './api'
import { CloseIcon } from './icons'
import { formatSize } from './util'

export type UploadItem = {
  id: number
  name: string
  dir: string
  loaded: number
  total: number
  status: 'queued' | 'uploading' | 'done' | 'error'
  error?: string
}

/**
 * Upload queue: files go one at a time (gentler on a phone's connection),
 * each with its own progress. onUploaded fires after each success.
 */
export function useUploads(onUploaded: (dir: string, files: Entry[]) => void) {
  const [items, setItems] = useState<UploadItem[]>([])
  const queue = useRef<{ item: UploadItem; file: File }[]>([])
  const running = useRef(false)
  const current = useRef<UploadHandle | null>(null)
  const nextId = useRef(1)
  const cb = useRef(onUploaded)
  cb.current = onUploaded

  const patch = (id: number, p: Partial<UploadItem>) => setItems((xs) => xs.map((x) => (x.id === id ? { ...x, ...p } : x)))

  const pump = useCallback(async () => {
    if (running.current) return
    running.current = true
    while (queue.current.length) {
      const { item, file } = queue.current.shift()!
      patch(item.id, { status: 'uploading' })
      const h = upload(item.dir, file, (loaded, total) => patch(item.id, { loaded, total }))
      current.current = h
      try {
        const saved = await h.promise
        patch(item.id, { status: 'done', loaded: item.total })
        cb.current(item.dir, saved)
      } catch (e) {
        patch(item.id, { status: 'error', error: e instanceof Error ? e.message : String(e) })
      }
      current.current = null
    }
    running.current = false
  }, [])

  const add = useCallback(
    (dir: string, files: FileList | File[]) => {
      const added = Array.from(files).map((file) => {
        const item: UploadItem = { id: nextId.current++, name: file.name, dir, loaded: 0, total: file.size, status: 'queued' }
        queue.current.push({ item, file })
        return item
      })
      setItems((xs) => [...xs, ...added])
      pump()
    },
    [pump],
  )

  const cancelAll = useCallback(() => {
    for (const q of queue.current) patch(q.item.id, { status: 'error', error: 'Cancelled' })
    queue.current = []
    current.current?.abort()
  }, [])

  const clearFinished = useCallback(() => setItems((xs) => xs.filter((x) => x.status === 'queued' || x.status === 'uploading')), [])

  return { items, add, cancelAll, clearFinished }
}

export function UploadPanel({ items, onCancel, onClear }: { items: UploadItem[]; onCancel: () => void; onClear: () => void }) {
  if (!items.length) return null
  const active = items.filter((i) => i.status === 'queued' || i.status === 'uploading')
  const failed = items.filter((i) => i.status === 'error')
  const total = items.reduce((s, i) => s + i.total, 0)
  const loaded = items.reduce((s, i) => s + (i.status === 'done' ? i.total : i.loaded), 0)
  const pct = total ? Math.round((loaded / total) * 100) : 100

  return (
    <div className="uploads glass" role="status" aria-live="polite">
      <div className="uploads-head">
        <strong>
          {active.length
            ? `Uploading ${items.length - active.length + 1} of ${items.length} · ${pct}%`
            : failed.length
              ? `${failed.length} upload${failed.length > 1 ? 's' : ''} failed`
              : `Uploaded ${items.length} file${items.length > 1 ? 's' : ''}`}
        </strong>
        {active.length ? (
          <button className="pill small" onClick={onCancel}>
            Cancel
          </button>
        ) : (
          <button className="glass-btn small" aria-label="Dismiss" onClick={onClear}>
            <CloseIcon size={20} />
          </button>
        )}
      </div>
      <div className="progress">
        <div style={{ width: `${pct}%` }} />
      </div>
      <ul className="uploads-list">
        {items.map((i) => (
          <li key={i.id} className={i.status}>
            <span className="u-name">{i.name}</span>
            <span className="u-state">
              {i.status === 'uploading' && `${formatSize(i.loaded)} / ${formatSize(i.total)}`}
              {i.status === 'queued' && 'Waiting'}
              {i.status === 'done' && 'Done'}
              {i.status === 'error' && i.error}
            </span>
          </li>
        ))}
      </ul>
    </div>
  )
}
