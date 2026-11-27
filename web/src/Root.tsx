import { useCallback, useEffect, useState } from 'react'
import { api, setCsrf, UNAUTHORIZED_EVENT, type Info, type Session } from './api'
import { App } from './App'
import { Login } from './Login'

type State = { kind: 'checking' } | { kind: 'offline'; message: string } | { kind: 'out' } | { kind: 'in'; info: Info }

/** Decides between the login screen and the app. */
export function Root() {
  const [state, setState] = useState<State>({ kind: 'checking' })

  const accept = useCallback((s: Session) => {
    if (s.authenticated && s.csrf) {
      setCsrf(s.csrf)
      setState({ kind: 'in', info: { name: s.name ?? 'Burrow', readOnly: s.readOnly } })
    } else {
      setCsrf('')
      setState({ kind: 'out' })
    }
  }, [])

  const check = useCallback(() => {
    setState({ kind: 'checking' })
    api.session().then(accept, (e) => setState({ kind: 'offline', message: e.message }))
  }, [accept])

  useEffect(check, [check])

  // Any request that comes back 401 (session expired, password changed on
  // the PC…) returns to the login screen.
  useEffect(() => {
    const h = () => {
      setCsrf('')
      setState({ kind: 'out' })
    }
    window.addEventListener(UNAUTHORIZED_EVENT, h)
    return () => window.removeEventListener(UNAUTHORIZED_EVENT, h)
  }, [])

  const logout = useCallback(async () => {
    try {
      await api.logout()
    } catch {
      // Even if the PC is unreachable, forget the session locally.
    }
    setCsrf('')
    setState({ kind: 'out' })
  }, [])

  switch (state.kind) {
    case 'checking':
      return (
        <div className="login">
          <img className="splash-icon" src="/icons/icon-192.png" alt="Burrow" width={84} height={84} />
        </div>
      )
    case 'offline':
      return (
        <div className="login">
          <div className="login-card">
            <img className="login-icon" src="/icons/icon-192.png" alt="" width={84} height={84} />
            <h1>Can't reach your PC</h1>
            <p className="login-sub">{state.message}</p>
            <button className="pill accent login-btn" onClick={check}>
              Try Again
            </button>
          </div>
        </div>
      )
    case 'out':
      return <Login onLoggedIn={accept} />
    case 'in':
      return <App info={state.info} onLogout={logout} />
  }
}
