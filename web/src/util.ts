import type { Entry } from './api'

export type Kind = 'folder' | 'image' | 'video' | 'audio' | 'pdf' | 'text' | 'other'

const ext = (name: string) => {
  const i = name.lastIndexOf('.')
  return i > 0 ? name.slice(i + 1).toLowerCase() : ''
}

const imageExt = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp', 'avif', 'bmp', 'heic', 'heif'])
const videoExt = new Set(['mp4', 'm4v', 'mov', 'webm', 'mkv', 'ogv'])
const audioExt = new Set(['mp3', 'm4a', 'aac', 'wav', 'ogg', 'oga', 'opus', 'flac'])
const textExt = new Set(['txt', 'md', 'log', 'csv', 'json', 'yaml', 'yml', 'ini'])
const thumbExt = new Set(['jpg', 'jpeg', 'png', 'gif', 'webp', 'heic', 'heif'])

// Video previews need ffmpeg on the PC; the server says whether it has it.
let videoThumbs = false
export const setVideoThumbs = (on: boolean) => {
  videoThumbs = on
}

export function kindOf(e: Entry): Kind {
  if (e.isDir) return 'folder'
  const x = ext(e.name)
  if (imageExt.has(x)) return 'image'
  if (videoExt.has(x)) return 'video'
  if (audioExt.has(x)) return 'audio'
  if (x === 'pdf') return 'pdf'
  if (textExt.has(x)) return 'text'
  return 'other'
}

/** Whether the server can make a thumbnail for this file. */
export const hasThumb = (e: Entry) => !e.isDir && (thumbExt.has(ext(e.name)) || (videoThumbs && videoExt.has(ext(e.name))))

/** HEIC and friends only display natively on Apple devices. */
export const isHeic = (e: Entry) => ['heic', 'heif'].includes(ext(e.name))

export const isMedia = (e: Entry) => ['image', 'video', 'audio'].includes(kindOf(e))

export function formatSize(n: number): string {
  if (n < 1024) return `${n} B`
  const units = ['KB', 'MB', 'GB', 'TB']
  let v = n / 1024
  let u = 0
  while (v >= 1024 && u < units.length - 1) {
    v /= 1024
    u++
  }
  return `${v < 10 ? v.toFixed(1) : Math.round(v)} ${units[u]}`
}

const dateFmt = new Intl.DateTimeFormat(undefined, { day: 'numeric', month: 'short', year: 'numeric' })
export const formatDate = (iso: string) => dateFmt.format(new Date(iso))

export const parentOf = (p: string) => (p.includes('/') ? p.slice(0, p.lastIndexOf('/')) : '')
export const joinPath = (dir: string, name: string) => (dir ? `${dir}/${name}` : name)

// Routes live in the URL hash: #/Photos/2024 → "Photos/2024".
export function pathFromHash(hash: string): string {
  const raw = hash.replace(/^#\/?/, '')
  if (!raw) return ''
  try {
    return raw
      .split('/')
      .filter(Boolean)
      .map((s) => decodeURIComponent(s))
      .join('/')
  } catch {
    return ''
  }
}

export const hashFromPath = (p: string) => '#/' + p.split('/').filter(Boolean).map(encodeURIComponent).join('/')

export type SortKey = 'name' | 'newest' | 'largest'

const collator = new Intl.Collator(undefined, { numeric: true, sensitivity: 'base' })

export function sortEntries(entries: Entry[], key: SortKey): Entry[] {
  const out = [...entries]
  out.sort((a, b) => {
    if (a.isDir !== b.isDir) return a.isDir ? -1 : 1
    if (key === 'newest') return b.modTime.localeCompare(a.modTime) || collator.compare(a.name, b.name)
    if (key === 'largest') return b.size - a.size || collator.compare(a.name, b.name)
    return collator.compare(a.name, b.name)
  })
  return out
}

// localStorage can be unavailable (private mode); never let that break the app.
export function load<T extends string>(key: string, allowed: readonly T[], fallback: T): T {
  try {
    const v = localStorage.getItem(key)
    return v && (allowed as readonly string[]).includes(v) ? (v as T) : fallback
  } catch {
    return fallback
  }
}

export function save(key: string, value: string) {
  try {
    localStorage.setItem(key, value)
  } catch {
    // ignore
  }
}

/** Splits "photo.jpg" into ["photo", ".jpg"] for renaming. */
export function splitExt(name: string): [string, string] {
  const i = name.lastIndexOf('.')
  return i > 0 ? [name.slice(0, i), name.slice(i)] : [name, '']
}

export function triggerDownload(url: string) {
  const a = document.createElement('a')
  a.href = url
  a.download = ''
  document.body.appendChild(a)
  a.click()
  a.remove()
}
