import { useEffect, useRef, useState } from 'react'
import type { EngineConfig, ProviderStatus, State, WSMessage } from '../types'
import { connectWS } from '../lib/ws'
import { api, getToken, setToken } from '../lib/api'

export default function Dashboard() {
  const [state, setState] = useState<State | null>(null)
  const [cfg, setCfg] = useState<EngineConfig | null>(null)
  const [status, setStatus] = useState<ProviderStatus | null>(null)
  const [events, setEvents] = useState<WSMessage[]>([])
  const [wsConnected, setWsConnected] = useState(false)
  const [token, setTokenInput] = useState(getToken())
  const [back, setBack] = useState<string | null>(null)
  const sim = { kind: 'comment', username: 'Dummy', text: '', value: 1 }
  const [simK, setSimK] = useState(sim.kind)
  const [simU, setSimU] = useState(sim.username)
  const [simT, setSimT] = useState(sim.text)
  const [simV, setSimV] = useState(sim.value)

  // live ws: dashboards receive state, events, leaderboard, status
  useEffect(() => {
    const conn = connectWS('dashboard', (m) => {
      switch (m.type) {
        case 'state': setState(m.data as State); break
        case 'event': setEvents((e) => [...e, m].slice(-50)); break
        case 'status': setStatus(m.data as ProviderStatus); break
        default: break
      }
    }, setWsConnected)
    setEvents([])
    refresh()
    return () => conn.close()
  }, [])

  async function refresh() {
    try {
      setState(await api.state())
      setCfg(await api.getConfig())
    } catch (e) { setBack((e as Error).message) }
  }

  async function act(action: 'start' | 'pause' | 'resume' | 'end') {
    try {
      await api[action]()
      setBack(action + ' ok')
      refresh()
    } catch (e) { setBack((e as Error).message) }
  }

  async function saveCfg() {
    if (!cfg) return
    try {
      await api.setConfig(cfg)
      setBack('config tersimpan')
    } catch (e) { setBack((e as Error).message) }
  }

  async function saveToken() {
    setToken(token)
    setBack('token disimpan (berlaku session)')
  }

  async function doSim() {
    try {
      await api.simulate(simK, simU, simT, simV)
      setBack('event simulasi terkirim')
    } catch (e) { setBack((e as Error).message) }
  }

  return (
    <div style={{ height: '100vh', overflow: 'auto', padding: 24, fontFamily: 'var(--font-body)', background: 'var(--c-cream)' }}>
      <h1 style={{ fontFamily: 'var(--font-display)', fontSize: 34, display: 'flex', alignItems: 'center', gap: 12 }}>
        🎮 Tiklive Dashboard
        <span style={{
          fontSize: 16, fontWeight: 700, padding: '4px 12px', borderRadius: 999,
          background: wsConnected ? 'var(--c-mint)' : 'var(--c-red)', color: '#fff',
        }}>
          {wsConnected ? 'WS ●' : 'WS ○'}
        </span>
      </h1>
      {back && <div style={{ marginBottom: 12, padding: '8px 14px', borderRadius: 10, background: 'var(--c-yellow)', border: '2px solid var(--ink)' }}>{back}</div>}

      {/* status + copy URL */}
      <section style={{ marginBottom: 24 }}>
        <h2>🔌 Provider & Overlay</h2>
        <div style={{ display: 'flex', gap: 12, flexWrap: 'wrap', alignItems: 'center' }}>
          <StatusChip status={status} />
          <input value={token} onChange={(e) => setTokenInput(e.target.value)}
            placeholder="Admin token (opsional)" style={inp} />
          <button onClick={saveToken} style={btn}>Simpan Token</button>
        </div>
        <div style={{ marginTop: 10, display: 'flex', gap: 8, alignItems: 'center', flexWrap: 'wrap' }}>
          <UrlCopy label="Overlay (OBS)" url={`${location.origin}/?mode=overlay`} size="1080x1920" />
          <UrlCopy label="Overlay 16:9" url={`${location.origin}/?mode=overlay&ratio=16:9`} size="1920x1080" />
          <UrlCopy label="Dashboard" url={`${location.origin}/?mode=dashboard`} size="auto" />
          <span style={{ fontSize: 14, color: 'var(--ink-muted)' }}>Beritahu OBS: Browser Source → custom CSS hilangkan scrollbar.</span>
        </div>
      </section>

      {/* game control */}
      <section style={{ marginBottom: 24 }}>
        <h2>▶️ Kontrol Game</h2>
        <div style={{ display: 'flex', gap: 10, flexWrap: 'wrap' }}>
          <button onClick={() => act('start')} style={{ ...btn, background: 'var(--c-mint)' }}>▶ Mulai</button>
          <button onClick={() => act('pause')} style={btnYellow}>⏸ Jeda</button>
          <button onClick={() => act('resume')} style={btnYellow}>▶ Lanjut</button>
          <button onClick={() => act('end')} style={{ ...btn, background: 'var(--c-red)', color: '#fff' }}>■ Akhiri</button>
          <button onClick={refresh} style={btn}>↻ Refresh</button>
        </div>
        {state && (
          <div style={{ marginTop: 12, display: 'grid', gridTemplateColumns: 'repeat(auto-fit,minmax(150px,1fr))', gap: 10 }}>
            <Stat k="Status" v={state.game} />
            <Stat k="Soal" v={state.question ? state.question.text : '—'} />
            <Stat k="Waktu" v={`${state.time_left}s`} />
            <Stat k="Ronde" v={`${state.round}/${state.round_max}`} />
            <Stat k="Penonton" v={`${state.viewers}`} />
          </div>
        )}
      </section>

      {/* config */}
      <section style={{ marginBottom: 24 }}>
        <h2>⚙️ Aturan Game</h2>
        {!cfg ? <div>Memuat…</div> : (
          <div style={{ display: 'grid', gridTemplateColumns: 'repeat(auto-fill,minmax(220px,1fr))', gap: 12 }}>
            <label style={lbl}>Tingkat
              <select value={cfg.difficulty} onChange={(e) => setCfg({ ...cfg, difficulty: e.target.value as EngineConfig['difficulty'] })} style={inp}>
                <option value="easy">Mudah</option>
                <option value="medium">Sedang</option>
                <option value="hard">Sulit</option>
              </select>
            </label>
            <label style={lbl}>Timer (dtk)
              <input type="number" value={cfg.timer_sec} min={3} onChange={(e) => setCfg({ ...cfg, timer_sec: +e.target.value })} style={inp} />
            </label>
            <label style={lbl}>Poin dasar
              <input type="number" value={cfg.base_points} min={1} onChange={(e) => setCfg({ ...cfg, base_points: +e.target.value })} style={inp} />
            </label>
            <label style={lbl}>Ronde
              <input type="number" value={cfg.round_total} min={1} onChange={(e) => setCfg({ ...cfg, round_total: +e.target.value })} style={inp} />
            </label>
            <label style={lbl}>Target gift (0 = off)
              <input type="number" value={cfg.gift_target} min={0} onChange={(e) => setCfg({ ...cfg, gift_target: +e.target.value })} style={inp} />
            </label>
            <div style={{ display: 'flex', gap: 14, alignItems: 'end' }}>
              <label style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <input type="checkbox" checked={!!cfg.speed_bonus} onChange={(e) => setCfg({ ...cfg, speed_bonus: e.target.checked })} /> Bonus cepat
              </label>
              <label style={{ display: 'flex', gap: 6, alignItems: 'center' }}>
                <input type="checkbox" checked={!!cfg.combo_bonus} onChange={(e) => setCfg({ ...cfg, combo_bonus: e.target.checked })} /> Bonus combo
              </label>
            </div>
          </div>
        )}
        <button onClick={saveCfg} style={{ ...btn, marginTop: 12 }}>💾 Simpan Aturan</button>
      </section>

      {/* simulation */}
      <section style={{ marginBottom: 24 }}>
        <h2>🧪 Simulasi Event</h2>
        <div style={{ display: 'flex', gap: 8, flexWrap: 'wrap', alignItems: 'end' }}>
          <label style={lbl}>Tipe
            <select value={simK} onChange={(e) => setSimK(e.target.value)} style={inp}>
              <option value="comment">Komentar</option>
              <option value="gift">Gift</option>
              <option value="like">Like</option>
              <option value="join">Join</option>
            </select>
          </label>
          <label style={lbl}>Username
            <input value={simU} onChange={(e) => setSimU(e.target.value)} style={inp} />
          </label>
          <label style={lbl}>Teks/Nilai
            <input value={simT} onChange={(e) => setSimT(e.target.value)} style={inp} placeholder="jawaban / nama gift" />
          </label>
          <label style={lbl}>Value
            <input type="number" value={simV} onChange={(e) => setSimV(+e.target.value)} style={inp} />
          </label>
          <button onClick={doSim} style={{ ...btn, background: 'var(--c-yellow)' }}>📤 Kirim</button>
        </div>
      </section>

      {/* event log */}
      <section>
        <h2>📜 Log Event (server)</h2>
        <div style={{ maxHeight: 260, overflow: 'auto', border: '2px solid var(--ink)', borderRadius: 12, background: 'var(--c-white)', padding: 8 }}>
          {events.length === 0 && <div style={{ color: 'var(--ink-muted)', padding: 8 }}>Belum ada event — mulai game & kirim event simulasi.</div>}
          {events.map((e, i) => (
            <div key={i} style={{ fontSize: 13, padding: '3px 6px', borderBottom: '1px solid rgba(0,0,0,.08)', fontFamily: 'monospace' }}>
              <b>{e.type}</b> {JSON.stringify(e.data).slice(0, 120)}
            </div>
          ))}
        </div>
      </section>
    </div>
  )
}

// helpers
function Stat({ k, v }: { k: string; v: string }) {
  return (
    <div className="comic-border" style={{ background: 'var(--c-white)', padding: '10px 14px' }}>
      <div style={{ fontSize: 13, color: 'var(--ink-muted)', textTransform: 'uppercase' }}>{k}</div>
      <div style={{ fontSize: 20, fontWeight: 800 }}>{v}</div>
    </div>
  )
}

function StatusChip({ status }: { status: ProviderStatus | null }) {
  if (!status) return <span style={{ fontSize: 14, color: 'var(--ink-muted)' }}>Status: —</span>
  return (
    <span className="comic-border" style={{ padding: '6px 14px', background: status.connected ? 'var(--c-mint)' : 'var(--c-cream)' }}>
      {status.type} · {status.connected ? '✅ Terhubung' : '⚠️ Terputus'}
      {status.error && <span style={{ marginLeft: 8, color: 'var(--c-red-dark)' }}>{status.error}</span>}
    </span>
  )
}

function UrlCopy({ label, url, size }: { label: string; url: string; size: string }) {
  async function copy() {
    try { await navigator.clipboard.writeText(url) } catch { /* clipboard may be blocked */ }
  }
  return (
    <button onClick={copy} style={{ ...btn, background: 'var(--c-black)', color: '#fff' }} title="klik untuk salin">
      <span style={{ display: 'block', fontSize: 12, opacity: .8 }}>{label} {size}</span>
      <span style={{ fontSize: 13 }}>{url}</span>
    </button>
  )
}

const btn: React.CSSProperties = {
  padding: '8px 14px', borderRadius: 10, border: '3px solid var(--ink)',
  boxShadow: '3px 3px 0 var(--ink)', background: 'var(--c-white)',
  fontWeight: 700, cursor: 'pointer', fontSize: 15,
}
const btnYellow: React.CSSProperties = { ...btn, background: 'var(--c-yellow)' }
const inp: React.CSSProperties = {
  padding: '8px 10px', borderRadius: 8, border: '2px solid var(--ink)', fontSize: 15,
  background: 'var(--c-white)', minWidth: 200,
}
const lbl: React.CSSProperties = {
  display: 'flex', flexDirection: 'column', gap: 4, fontSize: 13,
  fontWeight: 700, color: 'var(--ink-muted)',
}
