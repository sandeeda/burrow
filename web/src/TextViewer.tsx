import { useEffect, useState } from 'react'
import { downloadURL, type Entry } from './api'
import { useEscape } from './ui'
import { CloseIcon, DownloadIcon } from './icons'
import { triggerDownload } from './util'

const LIMIT = 256 * 1024

/** Shows the start of a text file as plain text (never rendered as HTML). */
export function TextViewer({ entry, onClose }: { entry: Entry; onClose: () => void }) {
  const [text, setText] = useState<string | null>(null)
  const [error, setError] = useState('')
  useEscape(onClose)

  useEffect(() => {
    const ctl = new AbortController()
    fetch(downloadURL(entry.path, true), { headers: { Range: `bytes=0-${LIMIT - 1}` }, signal: ctl.signal })
      .then((r) => (r.ok ? r.text() : Promise.reject(new Error(`HTTP ${r.status}`))))
      .then(setText)
      .catch((e) => !ctl.signal.aborted && setError(String(e.message ?? e)))
    return () => ctl.abort()
  }, [entry.path])

  return (
    <div className="textview">
      <div className="viewer-bar light">
        <button className="icon-btn" aria-label="Close" onClick={onClose}>
          <CloseIcon />
        </button>
        <div className="viewer-title">
          <span className="viewer-name">{entry.name}</span>
        </div>
        <button className="icon-btn" aria-label="Download" onClick={() => triggerDownload(downloadURL(entry.path))}>
          <DownloadIcon />
        </button>
      </div>
      {error && <p className="empty">Couldn't load: {error}</p>}
      {text === null && !error && <p className="empty">Loading…</p>}
      {text !== null && (
        <pre className="textview-body">
          {text}
          {entry.size > LIMIT && '\n\n… (showing the first 256 KB — download for the rest)'}
        </pre>
      )}
    </div>
  )
}
