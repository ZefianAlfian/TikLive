// tiklive server — interactive TikTok LIVE math game.
//
// Usage:
//
//	tiklive                  # serve with data/config.json (created on first run)
//	tiklive -config path     # explicit config file
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"tiklive/internal/config"
	"tiklive/internal/hub"
	"tiklive/internal/provider"
	"tiklive/internal/server"
)

func contextWithTimeout(d time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), d)
}

func main() {
	var cfgPath string
	flag.StringVar(&cfgPath, "config", filepath.Join("data", "config.json"), "path to config JSON")
	flag.Parse()

	cfg, err := config.Load(cfgPath)
	if err != nil {
		log.Fatalf("config: %v", err)
	}
	// Save a default config on first run so users can edit it.
	if _, err := os.Stat(cfgPath); os.IsNotExist(err) {
		if err := config.Store(cfgPath, cfg); err != nil {
			log.Printf("warn: cannot write default config: %v", err)
		} else {
			log.Printf("config tersimpan di %s", cfgPath)
		}
	}

	// Build hub + engine.
	h := hub.New(cfg.MaxQueue)

	// Start provider adapter per config.
	startProvider(cfg, h)

	// HTTP server (static frontend + REST + WS).
	srv := server.New(cfg, h, webDir())
	mux := srv.Routes()

	httpSrv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", cfg.Addr, cfg.Port),
		Handler: mux,
	}

	// Run the event bus consumer until shutdown.
	stop := make(chan struct{})
	go h.Run(stop)

	// Graceful shutdown.
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		log.Println("shutting down…")
		close(stop)
		h.StopProvider()
		ctx, cancel := contextWithTimeout(3 * time.Second)
		defer cancel()
		_ = httpSrv.Shutdown(ctx)
		os.Exit(0)
	}()

	log.Printf("tiklive berjalan di http://%s:%d  (dashboard: / , overlay: /?mode=overlay)",
		cfg.Addr, cfg.Port)
	if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		log.Fatalf("server: %v", err)
	}
}

// startProvider creates the configured adapter and connects the mock/tiktok.
func startProvider(cfg config.Config, h *hub.Hub) {
	pc := provider.Config{
		Username:     cfg.Provider.Username,
		SignerAPIKey: cfg.Provider.SignerAPIKey,
		QueueSize:    cfg.MaxQueue,
	}
	var a provider.Adapter
	switch cfg.Provider.Type {
	case config.ProviderTikTok:
		a = provider.NewTikTokAdapter(pc)
	default:
		a = provider.NewMock(pc)
	}
	h.SetProvider(a)
	if err := a.Start(); err != nil {
		log.Printf("provider %s gagal start: %v", a.Name(), err)
	}
}

// webDir resolves the built frontend directory relative to the binary.
func webDir() string {
	exe, err := os.Executable()
	if err == nil {
		dir := filepath.Join(filepath.Dir(exe), "web", "dist")
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	// Dev layout: tiklive/web/dist from the repo root.
	return filepath.Join("web", "dist")
}
