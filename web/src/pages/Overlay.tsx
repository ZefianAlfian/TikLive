import { useEffect, useRef, useState } from 'react'
import type {
  AnswerResult, GiftRaised, LeaderRow, Question, RawEvent, State, WSMessage,
} from '../types'
import { connectWS } from '../lib/ws'

// ---------------------------------------------------------------------------
// Overlay: 1080x1920 vertical stage (fit to OBS canvas, transparent bg).
// Comic/playful look: cream stage, red + yellow + mint accents, thick
// outlines, soft shadows, spring pop animations.
// ---------------------------------------------------------------------------

interface Bounce { key: number; user: string; text: string; good: boolean; points: number }

const MAX_LB = 8

export default function Overlay() {
  const [state, setState] = useState<State | null>(null)
  const [leader, setLeader] = useState<LeaderRow[]>([])
  const [bounces, setBounces] = useState<Bounce[]>([])
  const [gift, setGift] = useState<GiftRaised>({ raised: 0, target: 0 })
  const [connected, setConnected] = useState(false)
  const bouncesRef = useRef<Bounce[]>([])

  useEffect(() => {
    const push = (b: Bounce) => {
      const next = [...bouncesRef.current, b].slice(-4)
      bouncesRef.current = next
      setBounces(next)
      setTimeout(() => {
        const after = bouncesRef.current.filter((x) => x.key !== b.key)
        bouncesRef.current = after
        setBounces(after)
      }, 1400)
    }

    // Apply a state snapshot (from WS or poll).
    const applyState = (st: State) => {
      setState(st)
      if (st.scores) setLeader(st.scores)
    }

    // Poll fallback: if the WS link is down or OBS froze timers, fetch the
    // authoritative snapshot every 3s so the overlay never stays frozen.
    // Cheap (local JSON, no payload churn) and only runs while disconnected.
    const poll = setInterval(async () => {
      if (document.visibilityState === 'visible') {
        try {
          const r = await fetch('/api/state')
          if (r.ok) applyState(await r.json())
        } catch { /* server unreachable; retry next tick */ }
      }
    }, 3000)

    const conn = connectWS('overlay', (m: WSMessage) => {
      switch (m.type) {
        case 'state': {
          applyState(m.data as State)
          break
        }
        case 'leaderboard':
          setLeader(m.data as LeaderRow[])
          break
        case 'question':
          setState((s) => (s ? { ...s, question: (m.data as Question) ?? null } : s))
          break
        case 'tick':
          setState((s) => (s ? { ...s, time_left: m.data as number } : s))
          break
        case 'answer': {
          const r = m.data as AnswerResult
          push({ key: Date.now() + Math.random(), user: r.username, text: r.correct ? 'BENAR!' : 'SALAH!', good: r.correct, points: r.points })
          break
        }
        case 'gift.raised':
          setGift(m.data as GiftRaised)
          break
        case 'event': {
          const ev = m.data as RawEvent
          if (ev.kind === 'gift')
            push({ key: Date.now() + Math.random(), user: ev.username, text: `🎁 ${ev.gift_name || 'GIFT'} +${ev.gift_value || 0}`, good: true, points: 0 })
          break
        }
        case 'winner':
          // winner banner comes through state anyway
          break
      }
    }, setConnected)

    // Initial fetch: state snapshot for a fresh load.
    fetch('/api/state').then((r) => r.json()).then(applyState).catch(() => {/* offline; WS catch up */})

    return () => { conn.close(); clearInterval(poll) }
  }, [])

  const phase = state?.game || 'idle'
  const q = state?.question
  const tl = state?.time_left ?? 0
  const pct = q ? Math.max(0, Math.min(1, tl / q.duration_sec)) : 0

  // pull first 3 rows as podium
  const top = leader.slice(0, 3)
  const rest = leader.slice(3, MAX_LB)

  // Auto-scale: fit the fixed 1080x1920 canvas into whatever the OBS Browser
  // Source viewport is (any ratio), preserving proportions and centering.
  const DESIGN_W = 1080
  const DESIGN_H = 1920
  const [scale, setScale] = useState(1)
  useEffect(() => {
    const calc = () => setScale(Math.min(window.innerWidth / DESIGN_W, window.innerHeight / DESIGN_H))
    calc()
    window.addEventListener('resize', calc)
    return () => window.removeEventListener('resize', calc)
  }, [])
  return (
    <div style={{
      position: 'relative', width: '100vw', height: '100vh', overflow: 'hidden',
      background: 'transparent',
      display: 'flex', alignItems: 'center', justifyContent: 'center',
    }}>
      <div style={{
        position: 'relative', width: DESIGN_W, height: DESIGN_H,
        transform: `scale(${scale})`,
        transformOrigin: 'center center',
        flexShrink: 0,
      }}>
      <div style={{
        margin: '40px', height: '100%', borderRadius: 40, background: 'var(--c-cream)',
        border: '8px solid var(--ink)', boxShadow: '0 18px 0 rgba(26,26,26,.9)',
        display: 'flex', flexDirection: 'column', padding: '28px 34px', position: 'relative',
      }}>
        {/* header */}
        <Header phase={phase} connected={connected} viewers={state?.viewers ?? 0} />

        {/* question block */}
        <QuestionCard phase={phase} q={q ?? null} timeLeft={tl} pct={pct} round={state?.round ?? 1} total={state?.round_max ?? 10} />

        {/* bounces (BENAR!/SALAH!/gift) */}
        <BounceLayer bounces={bounces} />

        {/* gift target + leaderboard */}
        <div style={{ flex: 1, display: 'flex', flexDirection: 'column', minHeight: 0 }}>
          <GiftBar raised={gift.raised} target={gift.target} />
          <Leaderboard top={top} rest={rest} bounces={bounces} />
        </div>

        {/* weak footer */}
        <Footer />
      </div>
      </div>
    </div>
  )
}

function Header({ phase, connected, viewers }: { phase: string; connected: boolean; viewers: number }) {
  return (
    <div style={{ display: 'flex', alignItems: 'center', gap: 16, marginBottom: 20 }}>
      <div style={{
        display: 'inline-flex', alignItems: 'center', gap: 10, background: 'var(--c-black)',
        color: '#fff', border: '4px solid var(--ink)', borderRadius: 999, padding: '6px 18px',
        boxShadow: '5px 5px 0 var(--c-red)',
      }}>
        <span style={{ fontFamily: 'var(--font-display)', fontSize: 30, letterSpacing: 1 }}>TIKLIVE</span>
        <span style={{
          background: phase === 'running' ? 'var(--c-red)' : 'var(--c-yellow-dark)',
          color: '#fff', fontWeight: 800, fontSize: 18, borderRadius: 999, padding: '2px 12px',
        }}>● {phase === 'running' ? 'LIVE' : phase === 'paused' ? 'PAUSE' : phase === 'ended' ? 'SELESAI' : 'IDLE'}</span>
      </div>
      <div style={{ marginLeft: 'auto', display: 'flex', alignItems: 'center', gap: 8 }}>
        <span style={{
          width: 14, height: 14, borderRadius: '50%',
          background: connected ? 'var(--c-mint)' : 'var(--c-red)',
          border: '2px solid var(--ink)', boxShadow: connected ? '0 0 10px var(--c-mint)' : 'none',
        }} />
        {viewers > 0 && (
          <span style={{ fontSize: 20, fontWeight: 700, fontVariantNumeric: 'tabular-nums' }}>👁 {viewers}</span>
        )}
      </div>
    </div>
  )
}

function QuestionCard({ phase, q, timeLeft, pct, round, total }: {
  phase: string; q: Question | null; timeLeft: number; pct: number; round: number; total: number;
}) {
  if (!q) {
    return (
      <div style={{
        borderRadius: 30, border: '6px solid var(--ink)', background: 'var(--c-yellow)',
        padding: '40px 30px', textAlign: 'center', boxShadow: '10px 10px 0 var(--ink)', marginBottom: 18,
      }}>
        <div style={{ fontFamily: 'var(--font-display)', fontSize: 44, color: 'var(--ink)' }}>
          {phase === 'ended' ? 'GAME SELESAI!' : phase === 'paused' ? 'DIJEDA' : 'MENUNGGU SOAL…'}
        </div>
        {phase === 'idle' && (
          <div style={{ fontSize: 24, marginTop: 8, color: 'var(--ink-muted)' }}>Buka dashboard untuk mulai</div>
        )}
      </div>
    )
  }

  // timer ring: color shifts red near 0
  const ringColor = pct > 0.5 ? 'var(--c-mint)' : pct > 0.25 ? 'var(--c-yellow)' : 'var(--c-red)'

  return (
    <div style={{ position: 'relative', marginBottom: 18 }}>
      {/* round chip */}
      <div style={{
        position: 'absolute', top: -16, left: '50%', transform: 'translateX(-50%)',
        background: 'var(--c-red)', color: '#fff', fontWeight: 800, fontSize: 22,
        borderRadius: 999, padding: '4px 18px', border: '4px solid var(--ink)', zIndex: 3,
      }}>
        RONDE {round} / {total}
      </div>
      <div style={{
        borderRadius: 34, border: '7px solid var(--ink)', background: 'var(--c-white)',
        boxShadow: '12px 12px 0 var(--c-red)', padding: '48px 30px 34px', textAlign: 'center',
      }}>
        <div className="anim-pop" style={{ fontFamily: 'var(--font-display)', fontSize: 92, lineHeight: 1.15, color: 'var(--ink)', letterSpacing: 1 }}
          key={q.id}>
          {q.text}
        </div>
        <div style={{ marginTop: 24, display: 'flex', alignItems: 'center', justifyContent: 'center', gap: 14 }}>
          <div style={{ fontSize: 26, fontWeight: 700 }}>⏳</div>
          <div style={{ flex: '0 0 220px' }}>
            <div style={{ height: 26, borderRadius: 999, background: 'rgba(0,0,0,.12)', overflow: 'hidden', border: '3px solid var(--ink)' }}>
              <div style={{
                height: '100%', width: `${pct * 100}%`, borderRadius: 999,
                background: ringColor, transition: 'width .9s linear, background .4s',
              }} />
            </div>
          </div>
          <div style={{ fontFamily: 'var(--font-display)', fontSize: 34, color: ringColor }} className="mono">{Math.max(timeLeft, 0)}s</div>
        </div>
      </div>
    </div>
  )
}

function BounceLayer({ bounces }: { bounces: Bounce[] }) {
  return (
    <div style={{ position: 'absolute', inset: 0, pointerEvents: 'none', zIndex: 5 }}>
      {bounces.map((b) => (
        <div
          key={b.key}
          className={b.good ? 'anim-pop-big fx-good' : 'anim-shake fx-bad'}
          style={{
            position: 'absolute', left: '50%', top: '34%', transform: 'translateX(-50%)',
            fontFamily: 'var(--font-display)', fontSize: 74, zIndex: 10, whiteSpace: 'nowrap',
          }}
        >
          {b.text}
        </div>
      ))}
      {/* point + username floats */}
      {bounces.filter((b) => b.good && b.points > 0).map((b) => (
        <div key={`p${b.key}`} className="anim-float"
          style={{
            position: 'absolute', left: '50%', top: '42%', transform: 'translateX(-50%)',
            fontFamily: 'var(--font-display)', fontSize: 40, color: 'var(--c-mint-dark)',
            textShadow: '2px 2px 0 #fff', zIndex: 11,
          }}>
          {b.user} +{b.points} POIN
        </div>
      ))}
    </div>
  )
}

function GiftBar({ raised, target }: { raised: number; target: number }) {
  const show = target > 0
  const pct = target > 0 ? Math.min(1, raised / target) : 0
  return (
    <div style={{
      display: 'flex', alignItems: 'center', gap: 12, marginBottom: 16,
      opacity: show ? 1 : 0.35, transition: 'opacity .3s',
    }}>
      <span style={{ fontSize: 26 }}>🎯</span>
      <div style={{
        flex: 1, height: 26, borderRadius: 999, background: 'rgba(0,0,0,.1)',
        border: '3px solid var(--ink)', overflow: 'hidden',
      }}>
        <div style={{
          height: '100%', width: `${pct * 100}%`, borderRadius: 999,
          background: 'var(--c-yellow)', transition: 'width .4s',
        }} />
      </div>
      <span style={{ fontSize: 20, fontWeight: 800 }} className="mono">{raised}{target > 0 ? `/${target}` : ''}</span>
    </div>
  )
}

function Leaderboard({ top, rest, bounces }: { top: LeaderRow[]; rest: LeaderRow[]; bounces: Bounce[] }) {
  const latest = [...bounces].reverse()[0]
  return (
    <div style={{ flex: 1, minHeight: 0, overflow: 'hidden' }}>
      <div style={{ fontFamily: 'var(--font-display)', fontSize: 34, marginBottom: 10, color: 'var(--ink)' }}>
        🏆 PEMIMPIN
      </div>
      <div style={{ display: 'flex', gap: 12, marginBottom: 10 }}>
        {top.map((p, i) => (
          <div key={p.username} className="comic-border"
            style={{
              flex: 1, background: i === 0 ? 'var(--c-yellow)' : 'var(--c-white)',
              textAlign: 'center', padding: '10px 4px', boxShadow: `6px 6px 0 ${i === 0 ? 'var(--c-mint)' : 'var(--c-red)'}`,
            }}>
            <div style={{ fontSize: 30 }}>{['🥇', '🥈', '🥉'][i]}</div>
            <div style={{ fontSize: 20, fontWeight: 800, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {p.username}
            </div>
            <div style={{ fontSize: 24, fontWeight: 800, color: 'var(--c-red-dark)' }} className="mono">{p.points}</div>
          </div>
        ))}
      </div>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 8 }}>
        {rest.map((p) => (
          <div key={p.username} style={{
            display: 'flex', alignItems: 'center', gap: 10,
            background: 'var(--c-cream)', border: '3px solid var(--ink)', borderRadius: 14, padding: '6px 12px',
          }}>
            <span style={{ fontSize: 18, fontWeight: 800, width: 36, color: 'var(--ink-muted)' }} className="mono">#{p.rank}</span>
            <span style={{ flex: 1, fontSize: 22, fontWeight: 700, overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
              {p.username}
            </span>
            {p.streak >= 2 && <span style={{ fontSize: 18 }}>🔥{p.streak}</span>}
            <span style={{ fontSize: 20, fontWeight: 800, color: 'var(--c-red-dark)' }} className="mono">{p.points}</span>
          </div>
        ))}
      </div>
      {latest && latest.good && (
        <div className="anim-pop" style={{ marginTop: 12, fontSize: 22, fontWeight: 700, color: 'var(--c-mint-dark)' }}>
          ⚡ {latest.user} naik!
        </div>
      )}
    </div>
  )
}

function Footer() {
  return (
    <div style={{ marginTop: 14, textAlign: 'center', fontSize: 18, color: 'var(--ink-muted)', fontWeight: 600 }}>
      ✦ JAWAB DI KOMENTAR ✦
    </div>
  )
}
