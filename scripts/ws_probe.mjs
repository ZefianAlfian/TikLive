// ws_probe.mjs — verify the server pushes a state snapshot immediately on
// WebSocket connect (the reconnect self-heal fix).
const ws = new WebSocket('ws://127.0.0.1:8080/ws?role=overlay')
const t = setTimeout(() => { console.log('TIMEOUT: no message after connect'); process.exit(1) }, 4000)
ws.onopen = () => console.log('ws open')
ws.onmessage = (e) => {
  const m = JSON.parse(e.data)
  console.log('first msg:', m.type, '| game:', m.data && m.data.game, '| round:', m.data && m.data.round)
  clearTimeout(t)
  ws.close()
  process.exit(0)
}
ws.onerror = (e) => { console.log('ws error', e.message); process.exit(1) }
