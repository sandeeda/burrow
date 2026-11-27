import { useEffect, useRef, useState } from 'react'
import { api, ApiError, type Session } from './api'
import { Field } from './ui'

/** iOS-style login screen. Password managers can save the password (username "burrow"). */
export function Login({ onLoggedIn }: { onLoggedIn: (s: Session) => void }) {
  const [password, setPassword] = useState('')
  const [remember, setRemember] = useState(true)
  const [busy, setBusy] = useState(false)
  const [error, setError] = useState('')
  const [shaking, setShaking] = useState(false)
  const [lockedFor, setLockedFor] = useState(0)
  const input = useRef<HTMLInputElement>(null)

  useEffect(() => input.current?.focus(), [])

  // Lockout countdown.
  useEffect(() => {
    if (lockedFor <= 0) return
    const t = window.setTimeout(() => setLockedFor((s) => s - 1), 1000)
    return () => window.clearTimeout(t)
  }, [lockedFor])

  async function submit(e: React.FormEvent) {
    e.preventDefault()
    if (!password || busy || lockedFor > 0) return
    setBusy(true)
    setError('')
    try {
      onLoggedIn(await api.login(password, remember))
    } catch (err) {
      const ae = err instanceof ApiError ? err : null
      if (ae?.status === 429) {
        setLockedFor(ae.retryAfter || 60)
        setError('Too many wrong passwords.')
      } else {
        setError(ae?.message || String(err))
      }
      setShaking(true)
      setPassword('')
    } finally {
      setBusy(false)
    }
  }

  // Re-focus after a failed attempt (the field is disabled while checking).
  useEffect(() => {
    if (!busy && lockedFor <= 0) input.current?.focus()
  }, [busy, lockedFor])

  const mm = Math.floor(lockedFor / 60)
  const ss = String(lockedFor % 60).padStart(2, '0')

  return (
    <div className="login">
      <form className={'login-card' + (shaking ? ' shaking' : '')} onAnimationEnd={() => setShaking(false)} onSubmit={submit}>
        <img className="login-icon" src="/icons/icon-192.png" alt="" width={64} height={64} />
        <h1>Welcome back</h1>
        <p className="login-sub">Log in to open your files.</p>

        {/* Lets iOS Keychain / password managers save and fill the password. */}
        <input type="text" name="username" autoComplete="username" value="burrow" readOnly hidden />
        <Field
          inputRef={input}
          label="Password"
          type="password"
          name="password"
          autoComplete="current-password"
          value={password}
          onChange={setPassword}
          disabled={lockedFor > 0 || busy}
          invalid={!!error}
          enterKeyHint="go"
        />

        <label className="switch-row">
          <span>Remember this device</span>
          <input type="checkbox" role="switch" checked={remember} onChange={(e) => setRemember(e.target.checked)} />
          <span className="switch" aria-hidden="true" />
        </label>

        <p className="login-error" role="alert">
          {error}
          {lockedFor > 0 && ` Try again in ${mm}:${ss}.`}
        </p>

        <button className="btn primary block" type="submit" disabled={!password || busy || lockedFor > 0}>
          {busy ? 'Checking…' : 'Continue'}
        </button>
      </form>
      <p className="login-foot">Forgot it? Run <code>burrow --reset-password</code> on your PC.</p>
    </div>
  )
}
