// Package config loads and persists the host-configurable settings (game
// rules, provider, server port) stored as JSON on disk. Separate from the
// live game state which lives in the engine.
package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
)

type ProviderType string

const (
	ProviderMock   ProviderType = "mock"
	ProviderTikTok ProviderType = "tiktok"
)

type ProviderCfg struct {
	Type     ProviderType `json:"type"`
	Username string       `json:"username"`
	// SignerAPIKey is the optional TikTok WebCast signing-server key
	// (steampoweredtaco fork). Empty means anonymous signing, which is
	// rate-limited. Stored server-side only, never sent to the overlay.
	SignerAPIKey string `json:"signer_api_key,omitempty"`
}

type Config struct {
	Addr       string      `json:"addr"`
	Port       int         `json:"port"`
	Provider   ProviderCfg `json:"provider"`
	AdminToken string      `json:"admin_token,omitempty"`
	AutoStart  bool        `json:"auto_start"` // start a session on boot
	MaxEvents  int         `json:"max_events"` // event log ring size
	MaxQueue   int         `json:"max_queue"`  // provider queue depth
}

func Default() Config {
	return Config{
		Addr:      "127.0.0.1",
		Port:      8080,
		Provider:  ProviderCfg{Type: ProviderMock, Username: ""},
		AutoStart: false,
		MaxEvents: 500,
		MaxQueue:  2000,
	}
}

// Load reads the JSON config file, merging defaults for any missing fields.
func Load(path string) (Config, error) {
	cfg := Default()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return cfg, nil
		}
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Store persists the config atomically (write temp + rename).
func Store(path string, cfg Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Mutex protects concurrent config reads/writes.
var mu sync.Mutex

func Lock()   { mu.Lock() }
func Unlock() { mu.Unlock() }
