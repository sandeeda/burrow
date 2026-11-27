import { useEffect, useRef, useState } from 'react'
import { api, ApiError, type Session } from './api'

const MIN = 8

/**
 * First-time setup: the owner chooses Burrow's password from their own
 * device. The server only offers this to this PC itself or to devices signed
 * in to the PC's Tailscale account.
 */
export function Setup({ onDone }: { onDone: (s: Session) => void }) {
  const [password, setPassword] = useState('')
  const [confirm, setConfirm] = useState('')
  const [remember, setRemember] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => input.current?.focus(), [])

  const tooShort = [...password].length < MIN
  const mismatch = confirm !== '' && confirm !== password
  const hint = password && tooShort ? `Use at least ${MIN} characters.` : mismatch ? "Passwords don't match." : ''

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (tooShort || password !== confirm || busy) return
    setBusy(true)
    setError('')
    try {
      onDone(await api.setup(password, remember))
    } catch (err) {
      setError(err instanceof ApiError ? err.message : String(err))
      setBusy(false)
    }
  }

  return (
    <div className="login">
      <form className="login-card" onSubmit={submit}>
        <img className="login-icon" src="/icons/icon-192.png" alt="" width={84} height={84} />
        <h1>Set Up Burrow</h1>
        <p className="login-sub">Choose a password. You'll use it to open your files on any of your devices.</p>

        {/* Lets iOS Keychain / password managers save the new password. */}
        <input type="text" name="username" autoComplete="username" value="burrow" readOnly hidden />
        <input
          ref={input}
          className="login-input"
          type="password"
          name="new-password"
          autoComplete="new-password"
          placeholder="New password"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          disabled={busy}
          aria-label="New password"
        />
        <input
          className="login-input mid"
          type="password"
          name="confirm-password"
          autoComplete="new-password"
          placeholder="Confirm password"
          value={confirm}
          onChange={(e) => setConfirm(e.target.value)}
          disabled={busy}
          aria-label="Confirm password"
          aria-invalid={mismatch}
          enterKeyHint="done"
        />
        <label className="switch-row">
          <span>Remember this device</span>
          <input type="checkbox" role="switch" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
          <span className="switch" aria-hidden="true" />
        </label>

        <p className="login-error" role="alert">
          {error || hint}
        </p>

        <button className="pill accent login-btn" type="submit" disabled={tooShort || password !== confirm || busy}>
          {busy ? 'Setting Up…' : 'Set Password'}
        </button>
      </form>
      <p className="login-foot">You can change it later with <code>burrow --reset-password</code> on your PC.</p>
    </div>
  )
}
