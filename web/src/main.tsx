// Route by query/hash: ?mode=overlay (default for OBS) or ?mode=dashboard.
// Also accepts /#mode=... and /?view=...
import React from 'react'
import ReactDOM from 'react-dom/client'
import './index.css'

function mode(): 'overlay' | 'dashboard' {
  const q = new URLSearchParams(window.location.search).get('mode')
  if (q) return (q === 'dashboard' || q === 'overlay') ? q : 'overlay'
  const h = window.location.hash.replace('#', '')
  if (h === 'dashboard' || h === 'overlay') return h as 'overlay' | 'dashboard'
  return 'overlay'
}

async function render() {
  const m = mode()
  let App: React.ComponentType
  if (m === 'dashboard') {
    App = (await import('./pages/Dashboard')).default
  } else {
    App = (await import('./pages/Overlay')).default
  }
  ReactDOM.createRoot(document.getElementById('root')!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  )
}

render()
