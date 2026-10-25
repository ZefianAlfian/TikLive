# OBS Setup — Tiklive Overlay

## 1. Jalankan server

```bat
bin\tiklive.exe
```

Tunggu log `tiklive berjalan di http://127.0.0.1:8080`.

## 2. Tambah Browser Source di OBS

| Field | Nilai |
|---|---|
| URL | `http://127.0.0.1:8080/?mode=overlay` |
| Width | `1080` |
| Height | `1920` |
| FPS | 30 (cukup; animasi ringan) |
| Custom CSS | *(kosongkan)* |
| Shutdown source when not visible | ❌ off (biar animasi tetap jalan) |
| Refresh browser when scene becomes active | bebas |

Kanvas vertikal 1080×1920 sesuai format TikTok LIVE di OBS (Set → Video → Base
1080×1920 bila mau full vertical).

## 3. Overlay 16:9 (alternatif)

Bila kanvas OBS Anda landscape, tetap pakai 1080×1920 source lalu scale
manual (hold Shift + tarik), atau pakai URL yang sama — overlay punya
`transform: scale()` otomatis via CSS container bila ukuran window lebih
kecil dari kanvas.

## 4. Dashboard host

Buka `http://127.0.0.1:8080/?mode=dashboard` di browser biasa (Chrome/Edge).
Dari sini: start/pause/resume/end, atur soal/timer/poin, simulasi event, dan
lihat log event real-time.

## 5. Konek TikTok LIVE asli

1. Edit `data\config.json`:
   ```json
   "provider": { "type": "tiktok", "username": "username_anda", "signer_api_key": "" }
   ```
2. Restart `bin\tiklive.exe`.
3. Status koneksi terlihat di dashboard (chip provider). Bila gagal, error
   tampil di chip yang sama + log console.

Keterbatasan: butuh signing server (fork gotiktoklive); anonymous signer
dibatasi. Belum diuji live — gunakan mock untuk development.

## 6. Troubleshooting

- **Overlay putih / kosong** → cek server jalan; buka `/?mode=overlay` di
  browser biasa; lihat console (F12).
- **Tidak ada event masuk** → pastikan `provider.type` sesuai (`mock` otomatis
  mengirim event tiruan tiap 1.5–5 detik).
- **Leaderboard tidak update** → cek koneksi WS di dashboard (badge `WS ●`).
- **Port 8080 dipakai** → ubah `port` di `data\config.json`.
