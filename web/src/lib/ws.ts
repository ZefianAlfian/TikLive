import type { WSMessage } from '../types'

export interface LiveConn {
  close: () => void
  send: (m: unknown) => void
}

/**
 * connectWS opens a WebSocket to /ws with the given role and keeps it alive
 * with exponential backoff (500ms -> 5s cap). Every message is dispatched to
 * onMessage; connection state changes fire onStatus.
 */
export function connectWS(
  role: 'overlay' | 'dashboard',
  onMessage: (m: WSMessage) => void,
  onStatus: (connected: boolean) => void,
): LiveConn {
  let ws: WebSocket | null = null
  let retry = 500
  let closed = false

  const url = `${location.protocol === 'https:' ? 'wss' : 'ws'}://${location.host}/ws?role=${role}`

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
      onStatus(true)
    }
    ws.onmessage = (e) => {
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
      try { ws?.close() } catch { /* noop */ }
    },
    send(m) {
      if (ws && ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(m))
    },
  }
}
