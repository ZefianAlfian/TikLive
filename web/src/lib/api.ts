// API helpers for the dashboard. The admin token is sent as X-Admin-Token
// header (never in the URL). The overlay never calls these.
const TOKEN_KEY = 'tiklive_admin_token'

export function getToken(): string {
  return localStorage.getItem(TOKEN_KEY) || ''
}
export function setToken(t: string) {
  localStorage.setItem(TOKEN_KEY, t)
}

async function req(method: string, path: string, body?: unknown) {
  const headers: Record<string, string> = { 'Content-Type': 'application/json' }
  const token = getToken()
  if (token) headers['X-Admin-Token'] = token
  const res = await fetch(path, {
    method,
    headers,
    body: body ? JSON.stringify(body) : undefined,
  })
  let json: any = null
  try {
    json = await res.json()
  } catch {
    /* not JSON */
  }
  if (!res.ok) {
    const msg = (json && json.error) || res.statusText
    throw new Error(`${msg} (${res.status})`)
  }
  return json
}

export const api = {
  start: () => req('POST', '/api/control', { action: 'start' }),
  pause: () => req('POST', '/api/control', { action: 'pause' }),
  resume: () => req('POST', '/api/control', { action: 'resume' }),
  end: () => req('POST', '/api/control', { action: 'end' }),
  getConfig: () => req('GET', '/api/config'),
  setConfig: (cfg: unknown) => req('POST', '/api/config', cfg),
  state: () => req('GET', '/api/state'),
  simulate: (kind: string, username: string, text: string, value: number) =>
    req('POST', '/api/simulate', { kind, username, text, value }),
}
