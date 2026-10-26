import type { WSMessage } from '../types'

export interface LiveConn {
  close: () => void
  send: (m: unknown) => void
}

// The server broadcasts a full state snapshot every 5s. If we hear nothing
// for WATCHDOG_MS the link is dead (OBS suspend/resume, half-open TCP) and
// we force a reconnect instead of sitting on a zombie socket.
const WATCHDOG_MS = 8000

/**
 * connectWS opens a WebSocket to /ws with the given role and keeps it alive
 * with exponential backoff (500ms -> 5s cap). Every message is dispatched to
 * onMessage; connection state changes fire onStatus. The server pushes a
 * state snapshot immediately on connect, so every reconnect self-heals.
 *
 * OBS-specific recovery: when a Browser Source is backgrounded OBS freezes
 * its timers, so a setInterval watchdog never fires while hidden. We also
 * hook visibilitychange/pageshow (fired the moment the source becomes
 * visible again) and force a reconnect there if the link looks stale.
 */
export function connectWS(
  role: 'overlay' | 'dashboard',
  onMessage: (m: WSMessage) => void,
  onStatus: (connected: boolean) => void,
): LiveConn {
  let ws: WebSocket | null = null
  let retry = 500
  let closed = false
  let lastMsg = Date.now()

  const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?role=${role}`

  function stale() {
    return Date.now() - lastMsg > WATCHDOG_MS
  }

  function killIfStale() {
    if (closed || !ws) return
    if (stale()) {
      try { ws.close() } catch { /* noop */ } // triggers onclose -> schedule()
    }
  }

  // Primary watchdog (works while the page is visible/rendered).
  const watchdog = setInterval(killIfStale, 3000)

  // OBS wake-up hooks: these fire reliably when the Browser Source becomes
  // active again, even after long suspends, and catch the case where the
  // interval watchdog was frozen the whole time.
  const wake = () => {
    if (closed) return
    // Give the socket a beat to deliver buffered frames; then check.
    setTimeout(killIfStale, 1500)
  }
  document.addEventListener('visibilitychange', () => {
    if (document.visibilityState === 'visible') wake()
  })
  window.addEventListener('pageshow', wake)
  window.addEventListener('focus', wake)

  function open() {
    if (closed) return
    try {
      ws = new WebSocket(url)
    } catch {
      schedule()
      return
    }
    ws.onopen = () => {
      retry = 500
      lastMsg = Date.now()
      onStatus(true)
    }
    ws.onmessage = (e) => {
      lastMsg = Date.now()
      try {
        onMessage(JSON.parse(e.data) as WSMessage)
      } catch {
        /* ignore malformed frame */
      }
    }
    ws.onclose = () => {
      onStatus(false)
      schedule()
    }
    ws.onerror = () => {
      try { ws?.close() } catch { /* noop */ }
    }
  }

  function schedule() {
    if (closed) return
    setTimeout(open, retry)
    retry = Math.min(retry * 2, 5000)
  }

  open()

  return {
    close() {
      closed = true
      clearInterval(watchdog)
      try { ws?.close() } catch { /* noop */ }
    },
    send(m) {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(m))
    },
  }
}
