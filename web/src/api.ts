// Thin client for Burrow's JSON API (see PROGRESS.md for the endpoint list).

export type Entry = {
  name: string
  path: string
  isDir: boolean
  size: number
  modTime: string
  type?: string
}

export type Info = { name: string; readOnly: boolean }

export class ApiError extends Error {
  status: number
  constructor(status: number, message: string) {
    super(message)
    this.status = status
  }
}

const enc = encodeURIComponent

async function toError(res: Response): Promise<ApiError> {
  let msg = res.statusText || `HTTP ${res.status}`
  try {
    const body = await res.json()
    if (body && typeof body.error === 'string') msg = body.error
  } catch {
    // not JSON
  }
  return new ApiError(res.status, msg)
}

async function request<T>(url: string, init?: RequestInit): Promise<T> {
  let res: Response
  try {
    res = await fetch(url, { credentials: 'same-origin', ...init })
  } catch {
    throw new ApiError(0, "Can't reach your PC. Is Burrow running and Tailscale connected?")
  }
  if (!res.ok) throw await toError(res)
  if (res.status === 204) return undefined as T
  return res.json() as Promise<T>
}

function postJSON<T>(url: string, body: unknown): Promise<T> {
  return request<T>(url, {
    method: 'POST',
    headers: { 'Content-Type': 'application/json', 'X-Burrow': '1' },
    body: JSON.stringify(body),
  })
}

export const api = {
  info: () => request<Info>('/api/info'),
  list: (path: string) => request<{ path: string; entries: Entry[] }>(`/api/list?path=${enc(path)}`),
  search: (q: string, signal?: AbortSignal) =>
    request<{ query: string; results: Entry[] }>(`/api/search?limit=200&q=${enc(q)}`, { signal }),
  mkdir: (path: string) => postJSON<Entry>('/api/mkdir', { path }),
  rename: (path: string, name: string) => postJSON<Entry>('/api/rename', { path, name }),
  remove: (path: string) => postJSON<void>('/api/delete', { path }),
}

export function downloadURL(path: string, inline = false): string {
  return `/api/download?${inline ? 'inline=1&' : ''}path=${enc(path)}`
}

export function thumbURL(e: Entry, size: 256 | 1280 = 256): string {
  return `/api/thumb?size=${size}&path=${enc(e.path)}&v=${enc(e.modTime)}`
}

export type UploadHandle = { promise: Promise<Entry[]>; abort: () => void }

// upload sends one file with progress reporting (fetch can't report upload
// progress, so this uses XMLHttpRequest).
export function upload(dir: string, file: File, onProgress: (loaded: number, total: number) => void): UploadHandle {
  const xhr = new XMLHttpRequest()
  const promise = new Promise<Entry[]>((resolve, reject) => {
    xhr.open('POST', `/api/upload?path=${enc(dir)}`)
    xhr.setRequestHeader('X-Burrow', '1')
    xhr.upload.onprogress = (ev) => onProgress(ev.loaded, ev.total || file.size)
    xhr.onload = () => {
      let body: { files?: Entry[]; error?: string } = {}
      try {
        body = JSON.parse(xhr.responseText)
      } catch {
        // ignore
      }
      if (xhr.status >= 200 && xhr.status < 300) resolve(body.files ?? [])
      else reject(new ApiError(xhr.status, body.error ?? `Upload failed (${xhr.status})`))
    }
    xhr.onerror = () => reject(new ApiError(0, 'Upload failed: connection lost'))
    xhr.onabort = () => reject(new ApiError(0, 'Upload cancelled'))
    const form = new FormData()
    form.append('file', file, file.name)
    xhr.send(form)
  })
  return { promise, abort: () => xhr.abort() }
}
