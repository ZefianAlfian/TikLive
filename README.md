# Tiklive

Game interaktif untuk TikTok LIVE — kuis matematika hitung cepat dengan overlay
animasi real-time untuk OBS Browser Source. Backend Go ringan, frontend React
comic-style, komunikasi WebSocket, tanpa Docker.

## Fitur

- **Game Matematika Hitung Cepat** — penjumlahan, pengurangan, perkalian,
  pembagian (hasil bulat), operasi campuran ("5 + 11 - 2"); tingkat mudah /
  sedang / sulit.
- **Overlay 1080×1920** (vertikal) comic-style: merah/krem/hitam/mint/kuning,
  outline tebal, offset shadow, animasi pop/float/shake; indikator LIVE, timer
  ring berubah warna, badge ronde, bonus gift target, leaderboard real-time,
  bouncer BENAR!/SALAH!/gift, streak 🔥.
- **Dashboard host** — kontrol start/pause/resume/end, aturan game (tingkat,
  timer, poin dasar, ronde, bonus, target gift), simulasi event, log event
  live, status provider, salin URL Browser Source, admin token opsional.
- **Provider adapter** — `mock` (default, tanpa LIVE asli) dan `tiktok`
  (lewat gotiktoklive fork aktif). Reconnect otomatis + backoff, dedupe gift
  streak, event bus bounded.
- **Ringan** — ~13 MB RSS saat idle, ~14 MB setelah burst 200 event.

## Menjalankan (Windows, tanpa Docker)

### 1. Build

Butuh [Go ≥ 1.23](https://go.dev/dl/) dan Node.js ≥ 18.

```bat
cd web
npm install
npm run build
cd ..
go build -o bin\tiklive.exe cmd\server\main.go
```

Atau jalankan `scripts\build.bat` (mengerjakan keduanya).

### 2. Jalankan

```bat
bin\tiklive.exe
```

Server berjalan di `http://127.0.0.1:8080` (dapat diubah lewat
`data\config.json`):

- `/?mode=dashboard` — dashboard host
- `/?mode=overlay` — overlay OBS (default bila tanpa `?mode=`)
- `data\config.json` — konfigurasi (dibuat otomatis saat pertama kali jalan)

### 3. OBS Browser Source

1. Tambah **Browser** source di OBS.
2. URL: `http://127.0.0.1:8080/?mode=overlay`
3. Width `1080`, Height `1920` (atau 1920×1080 untuk 16:9 — layout otomatis
   scale lewat CSS `transform: scale()` bila layar lebih kecil; lihat
   `docs/obs-setup.md`).
4. ✅ *Shutdown source when not visible* — matikan bila ingin animasi jalan
   terus di background.
5. ✅ *Refresh browser when scene becomes active* — opsional.
6. Custom CSS: kosong (background sudah transparan, tanpa scrollbar).
7. Klik OK — overlay tampil dengan status LIVE ketika game berjalan.

## Konfigurasi (`data/config.json`)

```json
{
  "addr": "127.0.0.1",
  "port": 8080,
  "provider": {
    "type": "mock",
    "username": "",
    "signer_api_key": ""
  },
  "admin_token": "",
  "auto_start": false,
  "max_events": 500,
  "max_queue": 2000
```

- `provider.type`: `mock` untuk demo, `tiktok` untuk LIVE asli.
- `provider.username`: username TikTok yang LIVE-nya mau di-track.
- `provider.signer_api_key`: **opsional** — API key signing server
  (Euler Stream / sesuai fork gotiktoklive). Tanpa ini koneksi masih bisa
  jalan tapi kena rate limit sign request anonim.
- `admin_token`: bila diisi, semua endpoint kontrol (`/api/control`,
  `/api/config`, `/api/strict simulate`) menuntut header `X-Admin-Token`.
  Overlay hanya boleh menerima broadcast; overlay tidak pernah punya akses
  kontrol. Token disimpan di localStorage dashboard (jangan di URL).
- `max_queue`: antrean event (spike buffer).

## Keterbatasan provider TikTok (jujur, belum diuji live)

- Library: fork **`steampoweredtaco/gotiktoklive`** (MIT, aktif per 2025).
  Library asli `Davincible/gotiktoklive` (2022) sudah tidak kompatibel dengan
  endpoint TikTok saat ini.
- Bukan API resmi TikTok. Bisa berubah sewaktu-waktu tanpa pemberitahuan.
- Butuh **signing server**; anonymous signer dibatasi (~5 req/menit). Untuk
  pemakaian serius, siapkan API key Euler Stream (atau signing server lain).
- **Belum ada pengujian live**: integrasi TikTok asli memerlukan akun LIVE
  sungguhan. Yang terverifikasi adalah alur mock → engine → overlay.
- Adaptor TikTok menangani `DisconnectEvent` dengan reconnect + backoff, dan
  hanya menghitung gift streak terakhir (menghindari double count).
- Like/join/share tidak memengaruhi skor (game hanya merespons komentar).

## Struktur

```
tiklive/
├── cmd/server/         entrypoint (main.go)
├── internal/
│   ├── config/         load/store data/config.json
│   ├── engine/         game engine (soal, skor, timer) + unit test
│   ├── expr/           parser aritmatika + unit test
│   ├── hub/            event bus + WebSocket broadcast + provider fwd
│   ├── provider/       adapter mock + tiktok (reconnect, dedupe)
│   ├── server/         HTTP + REST + WS routing, guard admin
│   └── types/          wire types
├── web/                React + Vite frontend
│   ├── src/pages/      Overlay.tsx, Dashboard.tsx
│   ├── src/lib/        ws.ts (reconnect WS), api.ts (admin token header)
│   └── dist/           hasil build (dilayani Go server)
├── scripts/            build.bat, smoke.mjs
├── data/               config.json + log runtime (dibuat saat jalan)
└── docs/               overlay_live.png, obs-setup.md
```

## Pengujian

```bat
go test ./...                    -- unit test engine + expr
scripts\build.bat                -- build web + go
scripts\smoke.mjs                -- smoke test end-to-end (54 cek)
```

## Performa (diukur di mesin Windows 10, mode mock)

- RAM idle (full stack, mock live): **12.9 MB** RSS
- Setelah burst 200 event simulasi: **14.0 MB** (+1.1 MB), CPU total 1.0 s
- Frontend bundle: ~225 KB JS (70 KB gzip) + 2 KB CSS — ringan untuk OBS.
- WebSocket push, bukan polling; overlay hanya re-render bagian yang berubah.

## Lisensi

MIT.
